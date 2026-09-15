package model

// File 上传文件元数据模型。
type File struct {
	Base
	Name       string `gorm:"size:255;not null" json:"name"`
	Path       string `gorm:"size:255;not null" json:"path"`
	URL        string `gorm:"size:255;not null" json:"url"`
	MimeType   string `gorm:"size:128" json:"mime_type"`
	Size       int64  `json:"size"`
	UploaderID string `gorm:"size:36;not null;index" json:"uploader_id"`
}
