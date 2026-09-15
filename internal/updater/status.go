package updater

import "time"

// Status 是 Updater 的运行状态快照，用于向管理端展示更新提示。
type Status struct {
	// Enabled 表示自动更新是否处于启用状态。
	// 配置开启但版本不可比较（非语义化，如本机构建的 dev）时同样为 false：这种版本下
	// 更新永远不会生效，如实报 false 才不会让管理端把它显示成「已是最新」。
	Enabled bool
	// CurrentVersion 是当前二进制版本。
	CurrentVersion string
	// UpdateAvailable 表示最近一次检查是否发现了更高版本。
	UpdateAvailable bool
	// LatestVersion 是可用新版本号，无可用更新时为空。
	LatestVersion string
	// ReleaseURL 是可用新版本的 Release 页面地址，无可用更新时为空。
	ReleaseURL string
	// Source 是命中更新的来源平台，无可用更新时为空。
	Source SourceKind
	// LastCheckedAt 是最近一次检查的时间，尚未检查过时为零值。
	LastCheckedAt time.Time
	// LastError 是最近一次检查或升级的错误信息，成功时为空；
	// 未启用（Enabled=false）时承载停用原因，便于管理端说明为什么没有更新。
	LastError string
}

// Status 返回当前运行状态快照，可与其他方法并发调用。
func (u *Updater) Status() Status {
	u.statusMu.RLock()
	defer u.statusMu.RUnlock()

	return u.status
}

// markChecked 记录一次版本检查的结果：成功时刷新可用更新信息，失败时记录错误原因。
func (u *Updater) markChecked(candidates []*Release, err error) {
	u.statusMu.Lock()
	defer u.statusMu.Unlock()

	u.status.LastCheckedAt = time.Now()
	if err != nil {
		u.status.LastError = err.Error()
		return
	}

	u.status.LastError = ""
	if len(candidates) == 0 {
		u.status.UpdateAvailable = false
		u.status.LatestVersion = ""
		u.status.ReleaseURL = ""
		u.status.Source = ""
		return
	}

	top := candidates[0]
	u.status.UpdateAvailable = true
	u.status.LatestVersion = top.Version
	u.status.ReleaseURL = top.PageURL
	u.status.Source = top.Source.Kind
}

// markFailed 记录一次升级失败的原因，不影响已发现的可用更新信息。
func (u *Updater) markFailed(err error) {
	u.statusMu.Lock()
	defer u.statusMu.Unlock()

	u.status.LastError = err.Error()
}
