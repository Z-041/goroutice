package dto

import (
	"time"

	"goroutice/internal/updater"
)

// UpdaterStatus 自动更新状态响应（仅管理员可见）。
type UpdaterStatus struct {
	Enabled         bool       `json:"enabled"`
	CurrentVersion  string     `json:"current_version"`
	UpdateAvailable bool       `json:"update_available"`
	LatestVersion   string     `json:"latest_version,omitempty"`
	ReleaseURL      string     `json:"release_url,omitempty"`
	Source          string     `json:"source,omitempty"`
	LastCheckedAt   *time.Time `json:"last_checked_at,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
}

// ToUpdaterStatus 将更新器状态快照转换为响应结构。
func ToUpdaterStatus(st updater.Status) *UpdaterStatus {
	resp := &UpdaterStatus{
		Enabled:         st.Enabled,
		CurrentVersion:  st.CurrentVersion,
		UpdateAvailable: st.UpdateAvailable,
		LatestVersion:   st.LatestVersion,
		ReleaseURL:      st.ReleaseURL,
		Source:          string(st.Source),
		LastError:       st.LastError,
	}
	if !st.LastCheckedAt.IsZero() {
		checkedAt := st.LastCheckedAt
		resp.LastCheckedAt = &checkedAt
	}
	return resp
}
