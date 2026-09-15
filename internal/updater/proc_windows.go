//go:build windows

package updater

import "syscall"

// detachedProcess 让新进程不继承父进程控制台（Windows 常量 DETACHED_PROCESS）。
const detachedProcess = 0x00000008

// detachSysProcAttr 让新进程以独立进程组、脱离控制台的方式启动，父进程退出后继续存活。
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess,
	}
}
