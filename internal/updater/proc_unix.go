//go:build !windows

package updater

import "syscall"

// detachSysProcAttr 让新进程成为独立会话组长，脱离父进程控制终端。
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
