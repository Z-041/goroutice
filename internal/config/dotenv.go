package config

import (
	"bufio"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// dotEnvName 是环境变量文件名。
const dotEnvName = ".env"

// loadDotEnv 把 .env 中的键值注入进程环境变量，供 viper 的 BLOG_ 前缀覆盖使用。
//
// 按 searchDirs 的顺序取第一个存在的文件（见该函数的说明）。
// 已存在的环境变量优先，不会被文件覆盖，因此「系统环境变量 > .env > config.yaml > 内置默认」。
func loadDotEnv() {
	for _, path := range dotEnvPaths() {
		loaded, err := loadEnvFile(path)
		if err != nil {
			slog.Warn("read dotenv file", "path", path, "err", err)
			return
		}
		if loaded {
			slog.Info("loaded dotenv file", "path", path)
			return
		}
	}
}

// searchDirs 返回查找配置文件与 .env 的候选目录。
//
// 顺序为「可执行文件所在目录 → 当前工作目录」：
//  1. 可执行文件所在目录在前：自更新会替换二进制但不改变其位置，生产部署把配置放在 exe 旁最稳；
//  2. 当前工作目录作为回退：`go run` 的二进制在临时目录里，此时配置只在仓库根目录。
func searchDirs() []string {
	dirs := make([]string, 0, 2)
	if exePath, err := os.Executable(); err == nil {
		if resolved, resolveErr := filepath.EvalSymlinks(exePath); resolveErr == nil {
			exePath = resolved
		}
		dirs = append(dirs, filepath.Dir(exePath))
	}
	if cwd, err := os.Getwd(); err == nil {
		dirs = append(dirs, cwd)
	}
	return dirs
}

// dotEnvPaths 返回候选 .env 路径。
func dotEnvPaths() []string {
	dirs := searchDirs()
	paths := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		paths = append(paths, filepath.Join(dir, dotEnvName))
	}
	return paths
}

// loadEnvFile 读取一个 .env 文件并注入环境变量，返回该文件是否存在且被读取。
// 格式为每行一个 `KEY=VALUE`：忽略空行与以 # 开头的行，值两侧的引号会被去掉。
func loadEnvFile(path string) (bool, error) {
	f, err := os.Open(path) //nolint:gosec // 路径来自可执行文件/工作目录，非外部输入
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		key, value, ok := parseEnvLine(scanner.Text())
		if !ok {
			continue
		}
		// 进程内已存在的变量优先：显式导出的值不应被文件悄悄覆盖。
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return false, err
		}
	}
	if err := scanner.Err(); err != nil {
		return true, err
	}
	return true, nil
}

// parseEnvLine 解析一行 .env 内容，返回键值；注释、空行与非法行返回 ok=false。
func parseEnvLine(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	// 兼容 `export KEY=VALUE` 写法。
	line = strings.TrimPrefix(line, "export ")
	key, value, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", false
	}
	return key, unquote(strings.TrimSpace(value)), true
}

// unquote 去掉值两侧成对的单/双引号。
func unquote(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}
