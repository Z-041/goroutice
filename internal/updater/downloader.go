package updater

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// 下载相关限制。
const (
	// probeTimeout 是主备测速单次 HEAD 请求的超时。
	probeTimeout = 5 * time.Second
	// maxChecksumsBody 限制 checksums.txt 响应体读取上限。
	maxChecksumsBody = 1 << 20 // 1MB
	// sha256HexLen 是 SHA256 十六进制摘要长度。
	sha256HexLen = sha256.Size * 2
	// maxArtifactSize 限制单个产物的落盘上限，避免来源返回超大响应把磁盘写满。
	maxArtifactSize = 200 << 20 // 200MB
	// executableMode 是替换后二进制应具备的权限位。
	executableMode = 0o755
)

// do 发起一次带统一 User-Agent 的 HTTP 请求，超时由 timeout 控制，hdr 为附加请求头。
// 返回的 cancel 必须在响应体读取完毕后调用，否则请求上下文会被提前释放。
func (u *Updater) do(ctx context.Context, method, rawURL string, timeout time.Duration, hdr map[string]string) (*http.Response, context.CancelFunc, error) {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)

	req, err := http.NewRequestWithContext(reqCtx, method, rawURL, nil)
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("build %s request: %w", method, redactError(err))
	}
	req.Header.Set("User-Agent", userAgent)
	for key, value := range hdr {
		req.Header.Set(key, value)
	}

	resp, err := u.client.Do(req)
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("%s %s: %w", method, redactURL(rawURL), redactError(err))
	}
	return resp, cancel, nil
}

// pickRelease 对候选来源的产物地址并发测速，返回响应最快的 Release；
// 全部不可达时返回聚合错误。
func (u *Updater) pickRelease(ctx context.Context, candidates []*Release) (*Release, error) {
	type probeResult struct {
		rel     *Release
		latency time.Duration
		err     error
	}
	results := make(chan probeResult, len(candidates))
	for _, rel := range candidates {
		go func(rel *Release) {
			latency, err := u.probe(ctx, rel.AssetURL)
			results <- probeResult{rel: rel, latency: latency, err: err}
		}(rel)
	}

	var (
		best        *Release
		bestLatency time.Duration
		errs        []error
	)
	for range candidates {
		res := <-results
		if res.err != nil {
			errs = append(errs, res.err)
			continue
		}
		if best == nil || res.latency < bestLatency {
			best, bestLatency = res.rel, res.latency
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no reachable download source: %w", errors.Join(errs...))
	}
	u.log.Info("download source selected", "source", best.Source.Kind, "latency", bestLatency.String())
	return best, nil
}

// probe 通过 HEAD 请求测量单个下载地址的响应耗时。
func (u *Updater) probe(ctx context.Context, rawURL string) (time.Duration, error) {
	start := time.Now()
	resp, cancel, err := u.do(ctx, http.MethodHead, rawURL, probeTimeout, nil)
	if err != nil {
		return 0, err
	}
	defer cancel()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		return 0, fmt.Errorf("probe %s: unexpected status %s", redactURL(rawURL), resp.Status)
	}
	return time.Since(start), nil
}

// downloadVerified 下载并校验产物：先取 checksums.txt 得到期望摘要，再下载产物做流式 SHA256 比对；
// 任一步失败都会立即删除 dst（临时文件）并返回错误。
func (u *Updater) downloadVerified(ctx context.Context, rel *Release, dst string) error {
	if err := u.fetchAndVerify(ctx, rel, dst); err != nil {
		if rmErr := os.Remove(dst); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
			u.log.Warn("discard invalid artifact failed", "path", dst, "err", rmErr)
		}
		return err
	}
	return nil
}

// fetchAndVerify 完成“下载校验文件 → 解析期望摘要 → 下载产物 → 摘要比对”。
func (u *Updater) fetchAndVerify(ctx context.Context, rel *Release, dst string) error {
	sums, err := u.fetchBody(ctx, rel.ChecksumsURL, maxChecksumsBody)
	if err != nil {
		return fmt.Errorf("download checksums %s: %w", redactURL(rel.ChecksumsURL), err)
	}
	expected, err := checksumFor(sums, rel.AssetName)
	if err != nil {
		return fmt.Errorf("parse %s: %w", u.cfg.ChecksumsName, err)
	}
	if err := u.downloadToFile(ctx, rel.AssetURL, dst); err != nil {
		return err
	}
	actual, err := fileSHA256(dst)
	if err != nil {
		return err
	}
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", rel.AssetName, expected, actual)
	}
	u.log.Info("artifact verified", "asset", rel.AssetName, "sha256", actual)
	return nil
}

// fetchBody 拉取小体积文本资源（如校验文件），返回响应体内容。
func (u *Updater) fetchBody(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	resp, cancel, err := u.do(ctx, http.MethodGet, rawURL, requestTimeout, nil)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: unexpected status %s", redactURL(rawURL), resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", redactURL(rawURL), err)
	}
	return body, nil
}

// downloadToFile 下载产物到 dst（覆盖写入），失败时返回带上下文的错误。
func (u *Updater) downloadToFile(ctx context.Context, rawURL, dst string) error {
	resp, cancel, err := u.do(ctx, http.MethodGet, rawURL, downloadTimeout, nil)
	if err != nil {
		return err
	}
	defer cancel()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: unexpected status %s", redactURL(rawURL), resp.Status)
	}

	//nolint:gosec // dst 为本地同目录临时文件路径，需保留可执行权限
	file, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, executableMode)
	if err != nil {
		return fmt.Errorf("open %s: %w", dst, err)
	}
	// 多读 1 字节用于判定是否超限，避免把整个超大响应写入磁盘后再报错。
	written, copyErr := io.Copy(file, io.LimitReader(resp.Body, maxArtifactSize+1))
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("write %s (%d bytes): %w", dst, written, copyErr)
	}
	if written > maxArtifactSize {
		return fmt.Errorf("download %s: artifact exceeds %d bytes", redactURL(rawURL), maxArtifactSize)
	}
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", dst, closeErr)
	}
	return nil
}

// checksumFor 从 checksums.txt 内容中解析 assetName 对应的 SHA256 摘要（十六进制小写）。
// 兼容 GNU coreutils（"<hash>  <file>"、"<hash> *<file>"）与 BSD（"SHA256 (file) = <hash>"）两种格式。
func checksumFor(data []byte, assetName string) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), maxChecksumsBody)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, sum, ok := parseChecksumLine(line)
		if !ok || !strings.EqualFold(name, assetName) {
			continue
		}
		return sum, nil
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan checksums: %w", err)
	}
	return "", fmt.Errorf("checksum entry for %q not found", assetName)
}

// parseChecksumLine 解析单行摘要记录，返回文件名与十六进制摘要；格式非法时 ok 为 false。
func parseChecksumLine(line string) (name, sum string, ok bool) {
	const bsdPrefix = "SHA256 ("
	const bsdSep = ") = "
	if strings.HasPrefix(line, bsdPrefix) {
		rest := line[len(bsdPrefix):]
		end := strings.Index(rest, bsdSep)
		if end <= 0 {
			return "", "", false
		}
		name = strings.TrimPrefix(rest[:end], "*")
		sum = strings.TrimSpace(rest[end+len(bsdSep):])
		return name, strings.ToLower(sum), isHexDigest(sum)
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", "", false
	}
	name = strings.TrimPrefix(fields[1], "*")
	sum = strings.ToLower(fields[0])
	return name, sum, name != "" && isHexDigest(sum)
}

// isHexDigest 判断字符串是否为长度为 SHA256 十六进制摘要的合法十六进制串。
func isHexDigest(s string) bool {
	if len(s) != sha256HexLen {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// fileSHA256 流式计算文件的 SHA256 摘要（十六进制小写）。
func fileSHA256(path string) (string, error) {
	//nolint:gosec // path 为本地临时文件路径，非外部输入
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// redactURL 去掉 URL 查询串，避免 access_token 等敏感参数写入日志。
func redactURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "<invalid-url>"
	}
	parsed.RawQuery = ""
	return parsed.String()
}

// redactError 返回脱敏后的错误副本：net/http 与 url 包的错误会携带完整 URL，
// 其中的 access_token 等查询参数不得进入日志或状态接口返回。
func redactError(err error) error {
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		return err
	}
	sanitized := *urlErr
	sanitized.URL = redactURL(urlErr.URL)
	return &sanitized
}
