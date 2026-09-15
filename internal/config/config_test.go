package config

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// minimalConfig 是「密钥留空」形态的最小配置，用于验证环境变量能把这些空值补上。
const minimalConfig = `
server:
  port: 8080
database:
  host: 127.0.0.1
  password: ""
jwt:
  secret: ""
`

// writeTempFile 在临时目录写入文件并返回路径。
func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// TestLoadEnvFillsEmptySecrets 验证配置文件里留空的密钥可由环境变量补齐，
// 这是「密钥不写进配置文件」得以成立的前提。
func TestLoadEnvFillsEmptySecrets(t *testing.T) {
	t.Setenv("BLOG_DATABASE_PASSWORD", "from-env")
	t.Setenv("BLOG_JWT_SECRET", "env-secret-123456")

	cfg, err := Load(writeTempFile(t, "config.yaml", minimalConfig))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Database.Password != "from-env" {
		t.Errorf("database.password = %q, want from-env", cfg.Database.Password)
	}
	if cfg.JWT.Secret != "env-secret-123456" {
		t.Errorf("jwt.secret = %q, want env value", cfg.JWT.Secret)
	}
	// 未被覆盖的项仍来自配置文件。
	if cfg.Database.Host != "127.0.0.1" {
		t.Errorf("database.host = %q, want 127.0.0.1", cfg.Database.Host)
	}
}

// TestLoadEnvBindsSecretWithoutConfigKey 验证配置文件里完全没有该键时环境变量依然生效，
// 即 Load 中显式 BindEnv 的作用。
func TestLoadEnvBindsSecretWithoutConfigKey(t *testing.T) {
	t.Setenv("BLOG_JWT_SECRET", "env-secret-123456")

	cfg, err := Load(writeTempFile(t, "config.yaml", "server:\n  port: 8080\n"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.JWT.Secret != "env-secret-123456" {
		t.Errorf("jwt.secret = %q, want env value", cfg.JWT.Secret)
	}
}

// TestLoadEnvCoercesTypes 验证环境变量是字符串也能落到 int/bool/[]string 字段上。
func TestLoadEnvCoercesTypes(t *testing.T) {
	t.Setenv("BLOG_SERVER_PORT", "9999")
	t.Setenv("BLOG_SECURITY_EMAIL_VERIFICATION_ENABLED", "false")
	t.Setenv("BLOG_CORS_ALLOWED_ORIGINS", "https://a.com,https://b.com")

	cfg, err := Load(writeTempFile(t, "config.yaml", minimalConfig))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Server.Port != 9999 {
		t.Errorf("server.port = %d, want 9999", cfg.Server.Port)
	}
	if cfg.Security.EmailVerificationEnabled {
		t.Error("security.email_verification_enabled = true, want false")
	}
	if len(cfg.CORS.AllowedOrigins) != 2 || cfg.CORS.AllowedOrigins[1] != "https://b.com" {
		t.Errorf("cors.allowed_origins = %v, want 2 items with https://b.com last", cfg.CORS.AllowedOrigins)
	}
}

// TestLoadEnvFile 验证 .env 的解析规则（注释、空行、引号、export 前缀）以及
// 「进程内已有的环境变量优先」。
func TestLoadEnvFile(t *testing.T) {
	path := writeTempFile(t, ".env", `# 注释行

BLOG_TEST_PLAIN=plain
BLOG_TEST_DOUBLE="double quoted"
BLOG_TEST_SINGLE='single quoted'
export BLOG_TEST_EXPORTED=exported
BLOG_TEST_KEEP=from-file
这一行没有等号
`)

	keys := []string{"BLOG_TEST_PLAIN", "BLOG_TEST_DOUBLE", "BLOG_TEST_SINGLE", "BLOG_TEST_EXPORTED"}
	for _, key := range keys {
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
		t.Cleanup(func() { _ = os.Unsetenv(key) })
	}
	t.Setenv("BLOG_TEST_KEEP", "from-process")

	loaded, err := loadEnvFile(path)
	if err != nil {
		t.Fatalf("load env file: %v", err)
	}
	if !loaded {
		t.Fatal("expected .env to be reported as loaded")
	}

	want := map[string]string{
		"BLOG_TEST_PLAIN":    "plain",
		"BLOG_TEST_DOUBLE":   "double quoted",
		"BLOG_TEST_SINGLE":   "single quoted",
		"BLOG_TEST_EXPORTED": "exported",
	}
	for key, wantValue := range want {
		if got := os.Getenv(key); got != wantValue {
			t.Errorf("%s = %q, want %q", key, got, wantValue)
		}
	}
	if got := os.Getenv("BLOG_TEST_KEEP"); got != "from-process" {
		t.Errorf("BLOG_TEST_KEEP = %q, want from-process", got)
	}
}

// TestLoadEnvFileMissing 验证文件不存在时视为「未加载」而不是错误。
func TestLoadEnvFileMissing(t *testing.T) {
	loaded, err := loadEnvFile(filepath.Join(t.TempDir(), ".env"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded {
		t.Error("expected loaded=false for a missing file")
	}
}

// TestResolvePathFindsWorkingDirectory 验证工作目录下的 config.yaml 能被找到。
// 可执行文件目录优先，而测试二进制所在的目录里不会有 config.yaml，因此这里必然命中回退分支。
func TestResolvePathFindsWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, configName)
	if err := os.WriteFile(want, []byte(minimalConfig), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Chdir(dir)

	// 用 SameFile 比对而非字符串：Windows 上 Getwd 与 TempDir 的路径大小写/短名形式可能不同。
	wantInfo, err := os.Stat(want)
	if err != nil {
		t.Fatalf("stat written config: %v", err)
	}
	got := ResolvePath()
	gotInfo, err := os.Stat(got)
	if err != nil {
		t.Fatalf("ResolvePath() = %q, stat: %v", got, err)
	}
	if !os.SameFile(wantInfo, gotInfo) {
		t.Errorf("ResolvePath() = %q, want the config.yaml in the working directory", got)
	}
}

// TestResolvePathMissingReturnsAbsoluteCandidate 验证两处都找不到配置时返回绝对路径，
// 让报错信息直接指出该把文件放到哪，而不是一句 `open config.yaml` 让人无从下手。
func TestResolvePathMissingReturnsAbsoluteCandidate(t *testing.T) {
	t.Chdir(t.TempDir())

	if got := ResolvePath(); !filepath.IsAbs(got) {
		t.Errorf("ResolvePath() = %q, want an absolute path", got)
	}
}

// TestLoadResolvesUploadPathAgainstConfigDir 验证相对的 upload.path 以配置文件所在目录为基准，
// 而不是工作目录：否则从快捷方式或计划任务启动时，上传的文件会落到工作目录（可能是 C:\Windows\System32）。
func TestLoadResolvesUploadPathAgainstConfigDir(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, configName)
	if err := os.WriteFile(configPath, []byte(minimalConfig), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	// 换一个工作目录，模拟快捷方式/计划任务启动。
	t.Chdir(t.TempDir())

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := filepath.Join(dir, "uploads")
	if cfg.Upload.Path != want {
		t.Errorf("upload.path = %q, want %q", cfg.Upload.Path, want)
	}
}

// TestLoadKeepsAbsoluteUploadPath 验证写死的绝对路径原样保留，不因所在目录而被改写。
func TestLoadKeepsAbsoluteUploadPath(t *testing.T) {
	uploadDir := t.TempDir()
	content := minimalConfig + "upload:\n  path: " + strconv.Quote(uploadDir) + "\n"

	cfg, err := Load(writeTempFile(t, configName, content))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Upload.Path != uploadDir {
		t.Errorf("upload.path = %q, want %q", cfg.Upload.Path, uploadDir)
	}
}
