package dto

import (
	"time"

	"goroutice/internal/model"
)

// FileInfo 文件响应。
type FileInfo struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	URL        string    `json:"url"`
	MimeType   string    `json:"mime_type"`
	Size       int64     `json:"size"`
	UploaderID string    `json:"uploader_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// ToFileInfo 将文件模型转换为响应结构。
func ToFileInfo(f *model.File) *FileInfo {
	if f == nil {
		return nil
	}
	return &FileInfo{
		ID:         f.ID,
		Name:       f.Name,
		URL:        f.URL,
		MimeType:   f.MimeType,
		Size:       f.Size,
		UploaderID: f.UploaderID,
		CreatedAt:  f.CreatedAt,
	}
}
