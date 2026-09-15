package config

import (
	"os"
	"path/filepath"
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
