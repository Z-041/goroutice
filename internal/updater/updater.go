// Package updater 实现基于 GitHub / Gitee Release 的自更新链路：
// 定时轮询版本发现接口 → 匹配当前平台产物 → 下载并 SHA256 校验 → 原子替换并重启自身。
//
// 并发模型：
//   - Run 在单个 goroutine 中串行轮询，不使用后台常驻 worker；
//   - 多个 Release 来源的探测并发执行（见 fetchAll），并发 HEAD 测速选优（见 pickRelease）；
//   - Apply 由内部互斥锁串行化，避免重复触发替换与重启；
//   - 运行状态通过 Status 快照对外暴露，由读写锁保证并发安全。
package updater

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// 默认参数。
const (
	// defaultInterval 是默认轮询间隔。
	defaultInterval = 5 * time.Minute
	// defaultHTTPTimeout 是默认 HTTP 客户端整体超时，需覆盖较大的二进制下载。
	defaultHTTPTimeout = 10 * time.Minute
	// requestTimeout 是单次版本发现接口请求超时。
	requestTimeout = 30 * time.Second
	// downloadTimeout 是单次产物/校验文件下载超时。
	downloadTimeout = 10 * time.Minute
	// defaultChecksumsName 是默认的校验文件名。
	defaultChecksumsName = "checksums.txt"
	// userAgent 是访问 Release 接口时使用的 User-Agent（GitHub API 强制要求）。
	userAgent = "goroutice-updater"
)

// SourceKind 标识 Release 来源平台。
type SourceKind string

const (
	// SourceGitHub 表示 GitHub Releases。
	SourceGitHub SourceKind = "github"
	// SourceGitee 表示 Gitee Releases。
	SourceGitee SourceKind = "gitee"
)

// Source 描述一个 Release 来源（主源或备用源）。
type Source struct {
	Kind  SourceKind
	Owner string
	Repo  string
	// Token 通过配置注入，禁止硬编码；公开仓库可留空。
	Token string
}

// GitHubSource 构造 GitHub 来源。
func GitHubSource(owner, repo, token string) Source {
	return Source{Kind: SourceGitHub, Owner: owner, Repo: repo, Token: token}
}

// GiteeSource 构造 Gitee 来源。
func GiteeSource(owner, repo, token string) Source {
	return Source{Kind: SourceGitee, Owner: owner, Repo: repo, Token: token}
}

// Release 描述某个来源上匹配当前平台的一次候选升级。
type Release struct {
	// Version 是远端 tag，例如 v1.2.3。
	Version string
	// AssetName 是匹配到的产物文件名，例如 myapp-1.2.3-windows-amd64.exe。
	AssetName string
	// AssetURL 是产物下载地址。
	AssetURL string
	// ChecksumsURL 是同一 Release 下 checksums.txt 的下载地址。
	ChecksumsURL string
	// PageURL 是 Release 页面地址，用于日志排查。
	PageURL string
	// Source 是命中的来源。
	Source Source
}

// Config 是 Updater 的构造配置，零值字段自动填充默认值。
type Config struct {
	// CurrentVersion 是当前二进制版本，来自构建时注入的 main.version。
	CurrentVersion string
	// Sources 是 Release 来源列表，按优先级排列（主源在前，备用源在后）。
	Sources []Source
	// Interval 是轮询间隔，默认 5 分钟。
	Interval time.Duration
	// Platform 是资产匹配串，默认 runtime.GOOS-runtime.GOARCH（如 windows-amd64）。
	Platform string
	// AssetExt 是资产扩展名，默认 Windows 下为 .exe，其他平台为空。
	AssetExt string
	// ChecksumsName 是校验文件名，默认 checksums.txt。
	ChecksumsName string
	// HTTPClient 是外部注入的 HTTP 客户端，默认带 10 分钟超时。
	HTTPClient *http.Client
	// Logger 是外部注入的日志器，默认 slog.Default()。
	Logger *slog.Logger
	// RestartArgs 是重启新进程时透传的参数，默认 os.Args[1:]。
	RestartArgs []string
	// exit 是交接完成后退出旧进程的方式，默认 os.Exit；仅测试可覆盖。
	exit func(code int)
}

// Updater 负责版本发现、下载校验与零停机交接升级。
type Updater struct {
	cfg    Config
	log    *slog.Logger
	client *http.Client
	// mu 串行化 Apply，避免并发触发替换与重启。
	mu sync.Mutex
	// statusMu 保护 status，供 Status 与后台轮询并发读写。
	statusMu sync.RWMutex
	// status 是最近一次检查/升级结果的快照，用于向管理端展示更新提示。
	status Status
}

// New 校验配置并构造 Updater；未显式提供的字段使用默认值。
func New(cfg Config) (*Updater, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	cfg = withDefaults(cfg)

	u := &Updater{
		cfg:    cfg,
		log:    cfg.Logger,
		client: cfg.HTTPClient,
		status: Status{Enabled: true, CurrentVersion: cfg.CurrentVersion},
	}
	if _, err := parseVersion(cfg.CurrentVersion); err != nil {
		// 版本不可比较时更新永远不会发生（见 Check），因此必须如实报「未启用」：
		// 否则管理端会把它渲染成「已是最新」，让人以为刚检查过且确实没有新版本。
		u.status.Enabled = false
		u.status.LastError = fmt.Sprintf("current version %q is not semver; auto update is disabled", cfg.CurrentVersion)
		u.log.Warn("current version is not semver; auto update is disabled", "version", cfg.CurrentVersion)
	}
	return u, nil
}

// validateConfig 检查必填项与来源合法性。
func validateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.CurrentVersion) == "" {
		return errors.New("updater: CurrentVersion is required")
	}
	if len(cfg.Sources) == 0 {
		return errors.New("updater: at least one release source is required")
	}
	for _, src := range cfg.Sources {
		if src.Kind != SourceGitHub && src.Kind != SourceGitee {
			return fmt.Errorf("updater: unsupported source kind %q", src.Kind)
		}
		if src.Owner == "" || src.Repo == "" {
			return fmt.Errorf("updater: %s source requires owner and repo", src.Kind)
		}
	}
	return nil
}

// withDefaults 为未显式提供的配置项填充默认值。
func withDefaults(cfg Config) Config {
	if cfg.Interval <= 0 {
		cfg.Interval = defaultInterval
	}
	if cfg.Platform == "" {
		cfg.Platform = runtime.GOOS + "-" + runtime.GOARCH
	}
	if cfg.AssetExt == "" && runtime.GOOS == "windows" {
		cfg.AssetExt = ".exe"
	}
	if cfg.ChecksumsName == "" {
		cfg.ChecksumsName = defaultChecksumsName
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	if cfg.RestartArgs == nil {
		cfg.RestartArgs = os.Args[1:]
	}
	if cfg.exit == nil {
		cfg.exit = os.Exit
	}
	return cfg
}

// Run 阻塞轮询：立即检查一次，随后按 Interval 周期检查，直到 ctx 结束或完成一次交接升级。
// 单次失败只记录日志，不会中断轮询。
func (u *Updater) Run(ctx context.Context) {
	if !u.Status().Enabled {
		// 状态已标记为未启用（版本不可比较，见 New）：不轮询，否则每轮都会白拉一次
		// Release 接口再因版本比较失败而放弃，与管理端显示的「未启用」也对不上。
		return
	}
	u.runOnce(ctx)
	ticker := time.NewTicker(u.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			u.runOnce(ctx)
		}
	}
}

// Check 并发查询所有来源，返回仍高于当前版本的最高版本对应的全部主备候选（按来源优先级排列）。
// 无可用更新时返回空切片与 nil。
func (u *Updater) Check(ctx context.Context) ([]*Release, error) {
	releases, err := u.fetchAll(ctx)
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, nil
	}

	top := highestGroup(releases)
	newer, err := isNewer(u.cfg.CurrentVersion, top[0].Version)
	if err != nil {
		u.log.Warn("skip update: version compare failed", "current", u.cfg.CurrentVersion, "remote", top[0].Version, "err", err)
		return nil, nil
	}
	if !newer {
		return nil, nil
	}
	return top, nil
}

// Apply 在同一版本的多个候选来源中自动选择响应最快的服务，下载并校验产物，
// 随后原子替换当前可执行文件并启动新进程；成功时旧进程退出，函数不会返回。
// 替换或重启失败时会自动回滚到旧二进制，并返回带上下文与回滚结果的错误。
func (u *Updater) Apply(ctx context.Context, candidates []*Release) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	top := highestGroup(candidates)
	if len(top) == 0 {
		return errors.New("updater: empty candidate list")
	}
	best, err := u.pickRelease(ctx, top)
	if err != nil {
		return err
	}
	u.log.Info("upgrading", "from", u.cfg.CurrentVersion, "to", best.Version, "source", best.Source.Kind, "asset", best.AssetName)

	exePath, tmpPath, err := u.prepare(ctx, best)
	if err != nil {
		return err
	}
	backupPath, err := swapExecutable(exePath, tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := u.restart(exePath); err != nil {
		return u.rollback(exePath, backupPath, err)
	}
	u.log.Info("handed over to new process, exiting", "version", best.Version, "backup", backupPath)
	u.cfg.exit(0)
	return nil
}

// Refresh 立即执行一次版本检查并刷新状态快照，不触发下载与升级，供管理端手动检查使用。
// 检查失败时返回错误，同时错误原因会记录在 Status 中。
func (u *Updater) Refresh(ctx context.Context) error {
	_, err := u.refresh(ctx)
	return err
}

// refresh 执行一次“检查 → 记录状态”，返回仍高于当前版本的候选更新（无可用更新时为空切片）。
// 未启用时直接返回：此时 LastError 里存的是停用原因，不能被一次空检查覆盖掉。
func (u *Updater) refresh(ctx context.Context) ([]*Release, error) {
	if !u.Status().Enabled {
		return nil, nil
	}
	candidates, err := u.Check(ctx)
	u.markChecked(candidates, err)
	return candidates, err
}

// runOnce 执行一次“发现 → 升级”，失败仅记录日志，保证轮询循环不中断。
func (u *Updater) runOnce(ctx context.Context) {
	candidates, err := u.refresh(ctx)
	if err != nil {
		if ctx.Err() == nil {
			u.log.Error("check update failed", "err", err)
		}
		return
	}
	if len(candidates) == 0 {
		u.log.Debug("already up to date", "version", u.cfg.CurrentVersion)
		return
	}
	u.log.Info("new version available", "current", u.cfg.CurrentVersion, "remote", candidates[0].Version, "candidates", len(candidates))
	if err := u.Apply(ctx, candidates); err != nil {
		u.log.Error("apply update failed", "version", candidates[0].Version, "err", err)
		u.markFailed(err)
	}
}

// rollback 在重启失败时恢复备份的旧二进制，返回同时包含失败原因与回滚结果的错误。
func (u *Updater) rollback(exePath, backupPath string, cause error) error {
	if err := rollbackExecutable(exePath, backupPath); err != nil {
		return fmt.Errorf("restart new version: %w (rollback failed: %w, old binary kept at %s)", cause, err, backupPath)
	}
	return fmt.Errorf("restart new version: %w (rolled back to previous binary)", cause)
}

// platformSuffix 返回资产文件名中代表当前平台的匹配后缀，例如 -windows-amd64.exe。
func (u *Updater) platformSuffix() string {
	return "-" + u.cfg.Platform + u.cfg.AssetExt
}

// isPlatformAsset 判断资产文件名是否匹配当前平台（如 myapp-1.2.3-windows-amd64.exe）。
func (u *Updater) isPlatformAsset(name string) bool {
	return strings.HasSuffix(name, u.platformSuffix())
}
