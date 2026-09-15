package updater

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
)

// prepare 解析当前可执行文件路径，并把校验通过的产物写入同目录临时文件。
// 临时文件与目标同目录，保证后续 os.Rename 不跨文件系统。
func (u *Updater) prepare(ctx context.Context, rel *Release) (exePath, tmpPath string, err error) {
	exePath, err = os.Executable()
	if err != nil {
		return "", "", fmt.Errorf("locate current executable: %w", err)
	}
	// 解析符号链接，确保替换的是真实文件而非链接本身。
	if resolved, resolveErr := filepath.EvalSymlinks(exePath); resolveErr == nil {
		exePath = resolved
	}

	// 临时文件必须与目标同目录，保证下面的 os.Rename 不跨文件系统。
	tmpFile, err := os.CreateTemp(filepath.Dir(exePath), filepath.Base(exePath)+".new-*")
	if err != nil {
		return "", "", fmt.Errorf("create temp file next to executable: %w", err)
	}
	tmpPath = tmpFile.Name()
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", "", fmt.Errorf("close temp file %s: %w", tmpPath, err)
	}

	if err := u.downloadVerified(ctx, rel, tmpPath); err != nil {
		return "", "", err
	}
	if err := os.Chmod(tmpPath, executableMode); err != nil {
		_ = os.Remove(tmpPath)
		return "", "", fmt.Errorf("chmod temp file %s: %w", tmpPath, err)
	}
	return exePath, tmpPath, nil
}

// swapExecutable 用 newPath 原子替换 exePath：先把当前二进制改名为 .old 备份
// （Windows 允许重命名正在运行的可执行文件），再把新文件改名到位；
// 第二步失败会立即回滚，返回备份文件路径供回滚或清理使用。
func swapExecutable(exePath, newPath string) (string, error) {
	backupPath := exePath + ".old"
	if err := os.Remove(backupPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("remove stale backup %s: %w", backupPath, err)
	}
	if err := os.Rename(exePath, backupPath); err != nil {
		return "", fmt.Errorf("backup current executable to %s: %w", backupPath, err)
	}
	if err := os.Rename(newPath, exePath); err != nil {
		if rbErr := rollbackExecutable(exePath, backupPath); rbErr != nil {
			return backupPath, fmt.Errorf("replace executable: %w (rollback failed: %w, backup kept at %s)", err, rbErr, backupPath)
		}
		return "", fmt.Errorf("replace executable: %w", err)
	}
	return backupPath, nil
}

// rollbackExecutable 把备份的旧二进制恢复回原路径，覆盖启动失败的新二进制。
func rollbackExecutable(exePath, backupPath string) error {
	if err := os.Rename(backupPath, exePath); err != nil {
		return fmt.Errorf("restore backup %s: %w", backupPath, err)
	}
	return nil
}

// restart 以分离（detached）方式启动新二进制并透传原启动参数与环境变量，
// 启动成功后旧进程即可安全退出。
func (u *Updater) restart(exePath string) error {
	// 必须用 exec.Command 而非 exec.CommandContext：新进程要在旧进程退出后继续运行，
	// 绑定到会随优雅关闭一起取消的 ctx 会把它一起杀掉。
	//nolint:gosec,noctx // 执行的是替换后的自身二进制，路径来自 os.Executable
	cmd := exec.Command(exePath, u.cfg.RestartArgs...)
	// 不设置 cmd.Dir：新进程沿用当前工作目录，与本次启动时解析 config.yaml、
	// uploads 等相对路径的基准保持一致；改成二进制所在目录会让新进程找不到配置而启动失败。
	cmd.Env = os.Environ()
	cmd.Stdin = nil
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = detachSysProcAttr()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start new process %s: %w", exePath, err)
	}
	u.log.Info("new process started", "pid", cmd.Process.Pid, "exe", exePath)
	return nil
}
