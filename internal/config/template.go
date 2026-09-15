package config

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// configTemplate 是首次启动时生成的 config.yaml 内容。
//
// 模板漏掉某个键不会报错，只会让用户拿到的配置里少一行、静默回落到内置默认值，
// 因此用 TestTemplateMatchesConfigSchema 把「模板」与「Config 结构」钉在一起。
//
//go:embed config.template.yaml
var configTemplate string

// envTemplate 是首次启动时生成的 .env 内容，两个 %s 依次为 JWT 密钥与初始管理员口令。
// 内容是纯文本，除占位符外不含 % 字符。
const envTemplate = `# 由 server 首次启动自动生成。本文件含密钥，请勿提交到版本库或外发。
# 取值优先级：进程环境变量 > 本文件 > config.yaml > 程序内置默认值。
# 本文件缺失时，下次启动会重新生成一份新的随机密钥，已签发的令牌会随之全部失效。

# JWT 签名密钥，至少 16 字符。改动它会让所有已签发的令牌立即失效。
BLOG_JWT_SECRET=%s

# 初始管理员口令，仅在数据库中还没有该管理员时用于创建；首次登录后请立即修改。
BLOG_BOOTSTRAP_ADMIN_PASSWORD=%s

# 数据库口令按需填写（对应 config.yaml 里的 database.password）：
# BLOG_DATABASE_PASSWORD=
`

// jwtSecretBytes 是随机 JWT 密钥的字节数；hex 编码后 64 字符，远超 16 字符下限。
const jwtSecretBytes = 32

// adminPasswordLength 是随机初始管理员口令的长度。
const adminPasswordLength = 20

// passwordAlphabet 刻意剔除了 l/I/O/o/0/1：用户得照着 .env 手打口令登录，
// 形近字符看错一个，只会表现为「密码错误」，很难自己发现。
const passwordAlphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// 生成文件的权限：配置文件要能被用户编辑，.env 含密钥，只给属主。
const (
	configPerm = 0o644
	envPerm    = 0o600
)

// EnsureDefaults 在 config.yaml 与 .env 都不存在时按内置模板生成，并把结果写进日志。
//
// 只补缺失、不覆盖：自更新只替换二进制、不改变其位置，用户改过的端口、数据库连接、
// 站点域名必须原样留着，否则升级一次配置就被重置回默认值——那才是真的把配置弄丢了。
func EnsureDefaults() error {
	dir := generateDir()
	created := make([]string, 0, 2)

	configPath := filepath.Join(dir, configName)
	if !existsAny(configPaths()) {
		if err := writeFile(configPath, []byte(configTemplate), configPerm); err != nil {
			return err
		}
		created = append(created, configPath)
	}

	envCreated := false
	envPath := filepath.Join(dir, dotEnvName)
	if !existsAny(dotEnvPaths()) {
		content, err := envContent()
		if err != nil {
			return err
		}
		if err := writeFile(envPath, []byte(content), envPerm); err != nil {
			return err
		}
		created = append(created, envPath)
		envCreated = true
	}

	for _, path := range created {
		slog.Info("generated file", "path", path)
	}
	if envCreated {
		// 只提示位置，不打印口令本身：日志会被收集、转发、留存，口令不该在里面。
		slog.Info("initial admin password is in the generated .env; change it after the first login",
			"file", envPath, "key", "BLOG_BOOTSTRAP_ADMIN_PASSWORD")
	}
	return nil
}

// generateDir 返回生成配置文件的目录：已有 config.yaml 就生成在它旁边，否则生成在可执行文件所在目录。
//
// 与查找顺序（见 searchDirs）保持一致，生成的文件下次启动就能被找到。
// 已有配置时贴着它放，是为了让 `go run` 也把 .env 生成到仓库根目录——那种场景下可执行文件
// 在临时目录里，按可执行文件目录生成只会把 .env 丢进一次性的临时目录。
func generateDir() string {
	for _, path := range configPaths() {
		if _, err := os.Stat(path); err == nil {
			return filepath.Dir(path)
		}
	}
	dirs := searchDirs()
	if len(dirs) > 0 {
		return dirs[0]
	}
	return "."
}

// existsAny 报告候选路径中是否已有文件存在。
func existsAny(paths []string) bool {
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

// writeFile 写入文件，父目录不存在时创建。错误里带上路径：写不进去多半是权限问题，
// 而用户需要知道到底该给哪个目录提权。
func writeFile(path string, content []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, content, perm); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// envContent 渲染首次启动的 .env：随机 JWT 密钥与随机初始管理员口令。
func envContent() (string, error) {
	secret, err := randomHex(jwtSecretBytes)
	if err != nil {
		return "", err
	}
	password, err := randomPassword(adminPasswordLength)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(envTemplate, secret, password), nil
}

// randomHex 返回 n 字节密码学随机数据的 hex 编码。
func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// randomPassword 返回 length 个字符的随机口令。
//
// 用拒绝采样而非直接取模：256 不能被字母表长度整除，取模会让字母表前段字符出现得更频繁，
// 白白削掉一部分熵。落在不完整区间（>= limit）的字节直接丢弃重采样。
func randomPassword(length int) (string, error) {
	limit := byte(256 / len(passwordAlphabet) * len(passwordAlphabet))
	buf := make([]byte, length)
	out := make([]byte, 0, length)
	for len(out) < length {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("read random bytes: %w", err)
		}
		for _, b := range buf {
			if b >= limit {
				continue
			}
			out = append(out, passwordAlphabet[int(b)%len(passwordAlphabet)])
			if len(out) == length {
				break
			}
		}
	}
	return string(out), nil
}
