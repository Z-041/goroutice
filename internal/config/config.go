package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config 是应用的整体配置。
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	JWT       JWTConfig       `mapstructure:"jwt"`
	Upload    UploadConfig    `mapstructure:"upload"`
	RateLimit RateLimitConfig `mapstructure:"ratelimit"`
	Security  SecurityConfig  `mapstructure:"security"`
	SMTP      SMTPConfig      `mapstructure:"smtp"`
	CORS      CORSConfig      `mapstructure:"cors"`
	Bootstrap BootstrapConfig `mapstructure:"bootstrap"`
	Updater   UpdaterConfig   `mapstructure:"updater"`
	Site      SiteConfig      `mapstructure:"site"`
	Article   ArticleConfig   `mapstructure:"article"`
}

// ServerConfig HTTP 服务配置。
type ServerConfig struct {
	Port         int    `mapstructure:"port"`
	Mode         string `mapstructure:"mode"`
	ReadTimeout  int    `mapstructure:"read_timeout"`
	WriteTimeout int    `mapstructure:"write_timeout"`
	IdleTimeout  int    `mapstructure:"idle_timeout"`
	MaxBodySize  int64  `mapstructure:"max_body_size"`
	// TrustedProxies 是可信反向代理的 IP/CIDR 列表；为空表示不信任任何代理头。
	TrustedProxies []string `mapstructure:"trusted_proxies"`
}

// DatabaseConfig MySQL 连接配置。
type DatabaseConfig struct {
	Host                   string `mapstructure:"host"`
	Port                   int    `mapstructure:"port"`
	Username               string `mapstructure:"username"`
	Password               string `mapstructure:"password"`
	DBName                 string `mapstructure:"dbname"`
	Charset                string `mapstructure:"charset"`
	ParseTime              bool   `mapstructure:"parse_time"`
	Loc                    string `mapstructure:"loc"`
	MaxIdleConns           int    `mapstructure:"max_idle_conns"`
	MaxOpenConns           int    `mapstructure:"max_open_conns"`
	ConnMaxLifetimeMinutes int    `mapstructure:"conn_max_lifetime_minutes"`
}

// JWTConfig 鉴权配置。
type JWTConfig struct {
	Secret              string `mapstructure:"secret"`
	AccessExpireMinutes int    `mapstructure:"access_expire_minutes"`
	RefreshExpireHours  int    `mapstructure:"refresh_expire_hours"`
	Issuer              string `mapstructure:"issuer"`
}

// UploadConfig 文件上传配置。
type UploadConfig struct {
	Path        string   `mapstructure:"path"`
	MaxSize     int64    `mapstructure:"max_size"`
	AllowedExts []string `mapstructure:"allowed_exts"`
}

// RateLimitConfig 接口限流配置（令牌桶，分档）。
type RateLimitConfig struct {
	RPS        float64 `mapstructure:"rps"`
	Burst      int     `mapstructure:"burst"`
	WriteRPS   float64 `mapstructure:"write_rps"`
	WriteBurst int     `mapstructure:"write_burst"`
}

// SecurityConfig 安全加固配置。
type SecurityConfig struct {
	MaxLoginAttempts         int  `mapstructure:"max_login_attempts"`
	LoginLockMinutes         int  `mapstructure:"login_lock_minutes"`
	EmailVerificationEnabled bool `mapstructure:"email_verification_enabled"`
	PasswordMinLength        int  `mapstructure:"password_min_length"`
	PasswordRequireUppercase bool `mapstructure:"password_require_uppercase"`
	PasswordRequireSpecial   bool `mapstructure:"password_require_special"`
}

// SMTPConfig SMTP 邮件发送配置。
type SMTPConfig struct {
	Enabled            bool   `mapstructure:"enabled"`
	Host               string `mapstructure:"host"`
	Port               int    `mapstructure:"port"`
	Username           string `mapstructure:"username"`
	Password           string `mapstructure:"password"`
	From               string `mapstructure:"from"`
	FromName           string `mapstructure:"from_name"`
	UseTLS             bool   `mapstructure:"use_tls"`
	InsecureSkipVerify bool   `mapstructure:"insecure_skip_verify"`
}

// CORSConfig 跨域配置。
type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

// BootstrapConfig 初始化管理员种子配置。
type BootstrapConfig struct {
	AdminUsername string `mapstructure:"admin_username"`
	AdminPassword string `mapstructure:"admin_password"`
	AdminEmail    string `mapstructure:"admin_email"`
}

// UpdaterConfig 自动更新配置（GitHub 主源 + Gitee 备用源）。
type UpdaterConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	IntervalMinutes int    `mapstructure:"interval_minutes"`
	GitHubOwner     string `mapstructure:"github_owner"`
	GitHubRepo      string `mapstructure:"github_repo"`
	GitHubToken     string `mapstructure:"github_token"`
	GiteeOwner      string `mapstructure:"gitee_owner"`
	GiteeRepo       string `mapstructure:"gitee_repo"`
	GiteeToken      string `mapstructure:"gitee_token"`
}

// SiteConfig 站点元信息。
//
// RSS 与站点地图里的链接必须是绝对地址，而服务端只知道自己的监听端口，
// 不知道对外域名，因此站点域名必须显式配置，否则订阅源里会写出一堆
// 读者点不开的 localhost 链接。
type SiteConfig struct {
	BaseURL string `mapstructure:"base_url"`
	Title   string `mapstructure:"title"`
	// Description 用于 RSS channel 描述与站点地图的辅助信息。
	Description string `mapstructure:"description"`
	// Language 是 RSS 的 language 字段（如 zh-CN、en-US）。
	Language string `mapstructure:"language"`
}

// ArticleConfig 文章内容策略。
type ArticleConfig struct {
	// RevisionKeep 是每篇文章保留的历史修订条数，超出后从最旧的开始删除。
	// 设为 0 表示不记录修订历史。
	RevisionKeep int `mapstructure:"revision_keep"`
	// ViewDedupMinutes 是浏览量去重窗口（分钟）：同一来源在窗口内重复访问同一篇文章只计一次。
	// 设为 0 使用内置默认值（30 分钟）。
	ViewDedupMinutes int `mapstructure:"view_dedup_minutes"`
}

// configName 是配置文件名。
const configName = "config.yaml"

// configPaths 返回候选配置文件路径，目录顺序与 .env 一致（见 searchDirs）。
func configPaths() []string {
	dirs := searchDirs()
	paths := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		paths = append(paths, filepath.Join(dir, configName))
	}
	return paths
}

// ResolvePath 返回实际要加载的配置文件路径：取第一个存在的候选文件，返回绝对路径。
//
// 一个都不存在时返回最后一个候选（当前工作目录），这样报错信息会直接指出用户该把文件放到哪，
// 而不是抛出一句相对路径的 `open config.yaml: ...`。
//
// 之所以要按目录查找而不是直接用相对路径：Windows 上双击运行时工作目录恰好是 exe 所在目录，
// 但通过快捷方式、计划任务或服务启动时工作目录可能是别处，只认工作目录会让「配置就在 exe 旁」
// 也起不来，而失败信息一闪而过，用户根本不知道发生了什么。
func ResolvePath() string {
	candidates := configPaths()
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	if len(candidates) > 0 {
		return candidates[len(candidates)-1]
	}
	return configName
}

// Load 从指定配置文件加载配置。
//
// 取值优先级：进程环境变量 > .env 文件 > 配置文件 > setDefaults 内置默认值。
// 环境变量前缀为 BLOG_，层级用下划线连接（例如 BLOG_DATABASE_PASSWORD、BLOG_JWT_SECRET）；
// 列表项用英文逗号分隔（例如 BLOG_SERVER_TRUSTED_PROXIES=10.0.0.1,10.0.0.2）。
// 密钥类配置（数据库口令、JWT 密钥、SMTP 口令、初始管理员口令、更新令牌）不再写入配置文件，
// 统一由环境变量或 .env 注入。
func Load(path string) (*Config, error) {
	loadDotEnv()

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	v.SetEnvPrefix("BLOG")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	// AutomaticEnv 只对「已注册的键」生效（注册来源：默认值 / 配置文件 / BindEnv），
	// 而 Unmarshal 又是按已注册键逐个取值的，因此密钥类键必须显式绑定：
	// 否则有人把 config.yaml 里的密钥行整行删掉后，环境变量会静默失效。
	for _, key := range secretKeys {
		if err := v.BindEnv(key); err != nil {
			return nil, err
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	cfg.Upload.Path = resolveUploadPath(path, cfg.Upload.Path)
	return &cfg, nil
}

// resolveUploadPath 把相对的 upload.path 解析成基于配置文件所在目录的路径。
//
// 不拿工作目录当基准：Windows 上从快捷方式、计划任务或服务启动时工作目录可能是任何地方，
// 上传的文件会散落到意想不到的位置，甚至因无权限而直接失败。以配置文件为基准则
// 「配置在哪、数据就在哪」，与 logs 目录的落点一致。
func resolveUploadPath(configPath, uploadPath string) string {
	if uploadPath == "" || filepath.IsAbs(uploadPath) {
		return uploadPath
	}
	return filepath.Join(filepath.Dir(configPath), uploadPath)
}

// secretKeys 是不写进配置文件、必须由环境变量（或 .env）注入的密钥类配置。
var secretKeys = []string{
	"database.password",
	"jwt.secret",
	"smtp.username",
	"smtp.password",
	"bootstrap.admin_password",
	"updater.github_token",
	"updater.gitee_token",
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.mode", "debug")
	v.SetDefault("server.read_timeout", 30)
	v.SetDefault("server.write_timeout", 30)
	v.SetDefault("server.idle_timeout", 60)
	v.SetDefault("server.max_body_size", 16777216)

	v.SetDefault("database.host", "127.0.0.1")
	v.SetDefault("database.port", 3306)
	v.SetDefault("database.username", "root")
	v.SetDefault("database.password", "")
	v.SetDefault("database.dbname", "blog")
	v.SetDefault("database.charset", "utf8mb4")
	v.SetDefault("database.parse_time", true)
	v.SetDefault("database.loc", "Local")
	// 连接池参数默认 0 表示自动推导（按 CPU 核数），无需设置默认值。

	// jwt.secret 不设默认值：默认密钥会让「忘记配置」变成线上可预测的签名密钥，
	// 缺失时由启动流程直接拒绝启动。
	v.SetDefault("jwt.access_expire_minutes", 15)
	v.SetDefault("jwt.refresh_expire_hours", 720)
	v.SetDefault("jwt.issuer", "goroutice-blog")

	v.SetDefault("upload.path", "uploads")
	v.SetDefault("upload.max_size", 10485760)
	v.SetDefault("upload.allowed_exts", []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg", ".pdf", ".txt", ".md", ".zip"})

	v.SetDefault("ratelimit.rps", 5.0)
	v.SetDefault("ratelimit.burst", 10)
	v.SetDefault("ratelimit.write_rps", 20.0)
	v.SetDefault("ratelimit.write_burst", 40)

	v.SetDefault("security.max_login_attempts", 5)
	v.SetDefault("security.login_lock_minutes", 15)
	v.SetDefault("security.email_verification_enabled", true)
	v.SetDefault("security.password_min_length", 8)
	v.SetDefault("security.password_require_uppercase", false)
	v.SetDefault("security.password_require_special", false)

	v.SetDefault("smtp.enabled", false)
	v.SetDefault("smtp.host", "localhost")
	v.SetDefault("smtp.port", 25)
	v.SetDefault("smtp.username", "")
	v.SetDefault("smtp.password", "")
	v.SetDefault("smtp.from", "no-reply@example.com")
	v.SetDefault("smtp.from_name", "Goroutice Blog")
	v.SetDefault("smtp.use_tls", false)
	v.SetDefault("smtp.insecure_skip_verify", false)

	v.SetDefault("cors.allowed_origins", []string{"*"})

	v.SetDefault("bootstrap.admin_username", "admin")
	// admin_password 不设默认值：默认口令等于「任何知道源码的人都能登录线上后台」，
	// 缺失时由 SeedAdmin 在「尚无管理员」这一前提下拒绝初始化。
	v.SetDefault("bootstrap.admin_password", "")
	v.SetDefault("bootstrap.admin_email", "admin@example.com")

	v.SetDefault("updater.enabled", false)
	v.SetDefault("updater.interval_minutes", 5)
	v.SetDefault("updater.github_owner", "")
	v.SetDefault("updater.github_repo", "")
	v.SetDefault("updater.github_token", "")
	v.SetDefault("updater.gitee_owner", "")
	v.SetDefault("updater.gitee_repo", "")
	v.SetDefault("updater.gitee_token", "")

	// 默认值只保证本机开发可用；对外部署必须改成真实域名，否则 RSS/sitemap 里的链接不可用。
	v.SetDefault("site.base_url", "http://localhost:8080")
	v.SetDefault("site.title", "Goroutice Blog")
	v.SetDefault("site.description", "Goroutice Blog")
	v.SetDefault("site.language", "zh-CN")

	v.SetDefault("article.revision_keep", 20)
	v.SetDefault("article.view_dedup_minutes", 30)
}
