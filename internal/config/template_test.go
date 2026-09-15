package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// TestTemplateMatchesConfigSchema 断言模板与 Config 结构一一对应。
//
// 少一个键不会报错，只会让用户拿到的配置里少一行、静默回落到内置默认值；多一个键会被
// 悄悄忽略。以后新增配置项最容易忘的就是同步模板，所以在这里卡住。
func TestTemplateMatchesConfigSchema(t *testing.T) {
	schema := schemaKeys(reflect.TypeOf(Config{}), "", map[string]bool{})
	template := templateKeys(t)

	for key := range schema {
		if !template[key] {
			t.Errorf("config.template.yaml is missing %q: 新增配置项后忘了同步模板", key)
		}
	}
	for key := range template {
		if !schema[key] {
			t.Errorf("config.template.yaml has %q, which Config does not declare: 写错了会被静默忽略", key)
		}
	}
}

// TestTemplateIsLoadable 验证模板本身就是一份能被正常解析的完整配置。
func TestTemplateIsLoadable(t *testing.T) {
	cfg, err := Load(writeTempFile(t, configName, configTemplate))
	if err != nil {
		t.Fatalf("load template: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("server.port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Article.RevisionKeep != 20 {
		t.Errorf("article.revision_keep = %d, want 20", cfg.Article.RevisionKeep)
	}
	if cfg.Upload.Path == "" || cfg.Site.Title == "" {
		t.Errorf("upload.path = %q, site.title = %q, want both non-empty", cfg.Upload.Path, cfg.Site.Title)
	}
}

// TestEnsureDefaultsGeneratesEnvWithoutTouchingConfig 验证首次启动会补出 .env 与随机密钥，
// 同时已存在的 config.yaml 一个字节都不改、重复调用也不会重写 .env。
func TestEnsureDefaultsGeneratesEnvWithoutTouchingConfig(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	configPath := filepath.Join(dir, configName)
	if err := os.WriteFile(configPath, []byte(minimalConfig), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := EnsureDefaults(); err != nil {
		t.Fatalf("ensure defaults: %v", err)
	}

	envPath := filepath.Join(dir, dotEnvName)
	content := readFile(t, envPath)
	if got := len(envValue(t, content, "BLOG_JWT_SECRET")); got != jwtSecretBytes*2 {
		t.Errorf("generated jwt secret length = %d, want %d", got, jwtSecretBytes*2)
	}
	if got := len(envValue(t, content, "BLOG_BOOTSTRAP_ADMIN_PASSWORD")); got != adminPasswordLength {
		t.Errorf("generated admin password length = %d, want %d", got, adminPasswordLength)
	}

	// 再次调用必须原样保留：升级重启把用户改过的配置重置回默认值，才是真的「配置丢失」。
	if err := EnsureDefaults(); err != nil {
		t.Fatalf("second ensure defaults: %v", err)
	}
	if got := readFile(t, envPath); got != content {
		t.Error(".env was rewritten on the second call, want it left untouched")
	}
	if got := readFile(t, configPath); got != minimalConfig {
		t.Error("config.yaml was modified, want it left untouched")
	}
}

// TestRandomPasswordCoversAlphabet 验证口令只由字母表内字符构成，且每个字符都真的会出现——
// 取模实现会系统性漏掉字母表尾部的字符，靠这条断言卡住。
func TestRandomPasswordCoversAlphabet(t *testing.T) {
	const draws = 200
	seen := make(map[rune]bool, len(passwordAlphabet))
	for range draws {
		got, err := randomPassword(adminPasswordLength)
		if err != nil {
			t.Fatalf("random password: %v", err)
		}
		if len(got) != adminPasswordLength {
			t.Fatalf("password length = %d, want %d", len(got), adminPasswordLength)
		}
		for _, r := range got {
			if !strings.ContainsRune(passwordAlphabet, r) {
				t.Fatalf("unexpected character %q in password", r)
			}
			seen[r] = true
		}
	}
	if len(seen) != len(passwordAlphabet) {
		t.Errorf("only %d of %d alphabet characters appeared in %d draws", len(seen), len(passwordAlphabet), draws)
	}
}

// schemaKeys 反射配置类型，收集全部叶子键路径（按 mapstructure tag 逐层拼接）。
func schemaKeys(typ reflect.Type, prefix string, out map[string]bool) map[string]bool {
	for i := range typ.NumField() {
		field := typ.Field(i)
		name, _, _ := strings.Cut(field.Tag.Get("mapstructure"), ",")
		if name == "" {
			continue
		}
		key := name
		if prefix != "" {
			key = prefix + "." + name
		}
		if field.Type.Kind() == reflect.Struct {
			schemaKeys(field.Type, key, out)
			continue
		}
		out[key] = true
	}
	return out
}

// templateKeys 展开模板里的叶子键路径。
func templateKeys(t *testing.T) map[string]bool {
	t.Helper()
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(strings.NewReader(configTemplate)); err != nil {
		t.Fatalf("parse config template: %v", err)
	}
	return flattenKeys(v.AllSettings(), "", map[string]bool{})
}

// flattenKeys 把嵌套配置展开成「a.b.c」形式的叶子键集合。
func flattenKeys(settings map[string]any, prefix string, out map[string]bool) map[string]bool {
	for key, value := range settings {
		full := key
		if prefix != "" {
			full = prefix + "." + key
		}
		nested, ok := value.(map[string]any)
		if !ok {
			out[full] = true
			continue
		}
		flattenKeys(nested, full, out)
	}
	return out
}

// readFile 读取文本文件内容。
func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

// envValue 取出 .env 内容里某个键的值，复用实现自身的解析规则，避免两边规则各写一套。
func envValue(t *testing.T, content, key string) string {
	t.Helper()
	for _, line := range strings.Split(content, "\n") {
		if k, v, ok := parseEnvLine(line); ok && k == key {
			return v
		}
	}
	t.Fatalf("%s not found in generated .env:\n%s", key, content)
	return ""
}
