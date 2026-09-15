package service

import (
	"errors"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"goroutice/internal/config"
	"goroutice/internal/dto"
	"goroutice/internal/model"
	"goroutice/internal/pkg/apperror"
	"goroutice/internal/repository"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// FileService 文件上传与管理服务。
type FileService struct {
	repo *repository.FileRepository
	cfg  *config.UploadConfig
}

// 元数据列宽，与 model.File 的字段定义一致；用户可控的原始文件名与 Content-Type
// 都可能超长，直接落库会在 MySQL 报错并把上传整体变成 500。
const (
	fileNameMaxLen = 255
	fileMimeMaxLen = 128
)

// NewFileService 构造 FileService。
func NewFileService(repo *repository.FileRepository, cfg *config.UploadConfig) *FileService {
	return &FileService{repo: repo, cfg: cfg}
}

// Upload 保存上传文件并记录元数据。
func (s *FileService) Upload(uploaderID string, header *multipart.FileHeader) (*dto.FileInfo, error) {
	if header.Size > s.cfg.MaxSize {
		return nil, apperror.BadRequest("file size exceeds limit")
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !containsExt(s.cfg.AllowedExts, ext) {
		return nil, apperror.BadRequest("file type not allowed")
	}

	src, err := header.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = src.Close() }()

	if err := os.MkdirAll(s.cfg.Path, 0o750); err != nil {
		return nil, err
	}

	filename := uuid.NewString() + ext
	//nolint:gosec // 文件名由 UUID 生成，扩展名经 AllowedExts 白名单校验，不存在路径穿越风险。
	dst, err := os.Create(filepath.Join(s.cfg.Path, filename))
	if err != nil {
		return nil, err
	}
	defer func() { _ = dst.Close() }()

	if _, err := io.Copy(dst, src); err != nil {
		return nil, err
	}

	f := &model.File{
		Name:       truncateRunes(header.Filename, fileNameMaxLen),
		Path:       filename,
		URL:        "/uploads/" + filename,
		MimeType:   truncateRunes(header.Header.Get("Content-Type"), fileMimeMaxLen),
		Size:       header.Size,
		UploaderID: uploaderID,
	}
	if err := s.repo.Create(f); err != nil {
		// 元数据落库失败时删除已写入的物理文件，否则磁盘上会留下无记录的孤儿文件。
		_ = os.Remove(filepath.Join(s.cfg.Path, filename))
		return nil, err
	}
	return dto.ToFileInfo(f), nil
}

// List 分页查询文件，非管理员仅能查看自己上传的文件。
func (s *FileService) List(userID string, roles []string, page, size int) ([]dto.FileInfo, int64, error) {
	var uploaderID string
	if !slices.Contains(roles, model.RoleAdmin) {
		if userID == "" {
			// 非管理员必须有确定的身份：uploader_id 为空时仓储层会跳过过滤条件，
			// 那样「只看自己的文件」就退化成「看所有人的文件」。
			return nil, 0, apperror.Unauthorized("authentication required")
		}
		uploaderID = userID
	}

	files, total, err := s.repo.List(page, size, uploaderID)
	if err != nil {
		return nil, 0, err
	}

	infos := make([]dto.FileInfo, 0, len(files))
	for i := range files {
		infos = append(infos, *dto.ToFileInfo(&files[i]))
	}
	return infos, total, nil
}

// Delete 删除文件，仅上传者本人或管理员可操作。
func (s *FileService) Delete(userID string, roles []string, id string) error {
	f, err := s.repo.GetByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperror.NotFound("file not found")
		}
		return err
	}
	if !slices.Contains(roles, model.RoleAdmin) && f.UploaderID != userID {
		return apperror.Forbidden("you can only delete your own files")
	}

	if err := s.repo.Delete(id); err != nil {
		return err
	}
	// 尽力删除物理文件，失败不影响结果。
	_ = os.Remove(filepath.Join(s.cfg.Path, f.Path))
	return nil
}

func containsExt(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
