package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/hashicorp/go-version"
)

// maxReleaseBody 限制 Release 接口响应体读取上限，避免异常响应耗尽内存。
const maxReleaseBody = 1 << 20 // 1MB

// releaseAsset 对应 Release 接口中的单个资产。
type releaseAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
}

// releasePayload 是 GitHub 与 Gitee latest release 接口的公共响应结构。
type releasePayload struct {
	TagName string         `json:"tag_name"`
	PageURL string         `json:"html_url"`
	Assets  []releaseAsset `json:"assets"`
}

// apiURL 返回来源对应的版本发现接口地址，token 仅通过查询参数或请求头注入。
func (s Source) apiURL() string {
	owner := url.PathEscape(s.Owner)
	repo := url.PathEscape(s.Repo)
	if s.Kind == SourceGitee {
		endpoint := fmt.Sprintf("https://gitee.com/api/v5/repos/%s/%s/releases/latest", owner, repo)
		if s.Token == "" {
			return endpoint
		}
		return endpoint + "?access_token=" + url.QueryEscape(s.Token)
	}
	return fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
}

// fetchAll 并发拉取所有来源并匹配当前平台产物，返回按配置顺序（主源在前）排列的结果；
// 单个来源失败不影响其他来源，全部失败时返回聚合错误。
func (u *Updater) fetchAll(ctx context.Context) ([]*Release, error) {
	results := make([]*Release, len(u.cfg.Sources))
	errs := make([]error, len(u.cfg.Sources))

	var wg sync.WaitGroup
	for i, src := range u.cfg.Sources {
		wg.Add(1)
		go func(idx int, src Source) {
			defer wg.Done()
			rel, err := u.fetchOne(ctx, src)
			if err != nil {
				errs[idx] = err
				return
			}
			results[idx] = rel
		}(i, src)
	}
	wg.Wait()

	releases := make([]*Release, 0, len(results))
	for i, rel := range results {
		if rel != nil {
			releases = append(releases, rel)
			continue
		}
		u.log.Warn("release source unavailable", "source", u.cfg.Sources[i].Kind, "err", errs[i])
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("all release sources failed: %w", errors.Join(errs...))
	}
	return releases, nil
}

// fetchOne 拉取单个来源的最新 Release 并匹配当前平台产物。
func (u *Updater) fetchOne(ctx context.Context, src Source) (*Release, error) {
	payload, err := u.fetchLatest(ctx, src)
	if err != nil {
		return nil, err
	}
	return u.matchRelease(src, payload)
}

// fetchLatest 请求单个来源的最新 Release；任何异常均返回带上下文的错误。
func (u *Updater) fetchLatest(ctx context.Context, src Source) (*releasePayload, error) {
	hdr := map[string]string{"Accept": "application/vnd.github+json"}
	if src.Kind == SourceGitHub && src.Token != "" {
		hdr["Authorization"] = "Bearer " + src.Token
	}

	resp, cancel, err := u.do(ctx, http.MethodGet, src.apiURL(), requestTimeout, hdr)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s latest release: unexpected status %s", src.Kind, resp.Status)
	}
	var payload releasePayload
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxReleaseBody)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode %s release body: %w", src.Kind, err)
	}
	if payload.TagName == "" {
		return nil, fmt.Errorf("%s release has empty tag_name", src.Kind)
	}
	return &payload, nil
}

// matchRelease 在 Release 中匹配当前平台产物与校验文件。
func (u *Updater) matchRelease(src Source, payload *releasePayload) (*Release, error) {
	rel := &Release{Version: payload.TagName, PageURL: payload.PageURL, Source: src}
	for _, asset := range payload.Assets {
		switch {
		case rel.ChecksumsURL == "" && strings.EqualFold(asset.Name, u.cfg.ChecksumsName):
			rel.ChecksumsURL = asset.DownloadURL
		case rel.AssetURL == "" && u.isPlatformAsset(asset.Name):
			rel.AssetURL = asset.DownloadURL
			rel.AssetName = asset.Name
		}
	}
	if rel.AssetURL == "" {
		return nil, fmt.Errorf("%s %s: no asset matching %q", src.Kind, payload.TagName, u.platformSuffix())
	}
	if rel.ChecksumsURL == "" {
		return nil, fmt.Errorf("%s %s: no %s asset", src.Kind, payload.TagName, u.cfg.ChecksumsName)
	}
	return rel, nil
}

// highestGroup 返回版本最高的候选集合；同一版本存在多个来源（主备）时全部保留，顺序与入参一致。
func highestGroup(releases []*Release) []*Release {
	if len(releases) == 0 {
		return nil
	}
	best := releases[0]
	for _, rel := range releases[1:] {
		if compareVersions(rel.Version, best.Version) > 0 {
			best = rel
		}
	}
	group := make([]*Release, 0, len(releases))
	for _, rel := range releases {
		if compareVersions(rel.Version, best.Version) == 0 {
			group = append(group, rel)
		}
	}
	return group
}

// isNewer 判断 remote 是否严格高于 current；任一侧不是语义化版本时返回错误。
func isNewer(current, remote string) (bool, error) {
	cur, err := parseVersion(current)
	if err != nil {
		return false, fmt.Errorf("parse current version: %w", err)
	}
	rem, err := parseVersion(remote)
	if err != nil {
		return false, fmt.Errorf("parse remote version: %w", err)
	}
	return rem.GreaterThan(cur), nil
}

// compareVersions 比较两个版本号：a 大于 b 返回 1，小于返回 -1，相等或均无法解析返回 0。
func compareVersions(a, b string) int {
	va, errA := parseVersion(a)
	vb, errB := parseVersion(b)
	switch {
	case errA != nil && errB != nil:
		return 0
	case errA != nil:
		return -1
	case errB != nil:
		return 1
	}
	return va.Compare(vb)
}

// parseVersion 解析版本号，兼容 v 前缀；空串或非法格式返回错误。
func parseVersion(raw string) (*version.Version, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, errors.New("empty version")
	}
	parsed, err := version.NewVersion(strings.TrimPrefix(trimmed, "v"))
	if err != nil {
		return nil, fmt.Errorf("parse version %q: %w", raw, err)
	}
	return parsed, nil
}
