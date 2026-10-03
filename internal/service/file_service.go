// Package service 提供文件业务逻辑
package service

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"

	"github.com/myzjc/SuperDisk/internal/model"
	"github.com/myzjc/SuperDisk/internal/storage"

	"gorm.io/gorm"
)

type FileService struct {
	db      *gorm.DB
	storage storage.Storage
}

func NewFileService(db *gorm.DB, st storage.Storage) *FileService {
	return &FileService{
		db:      db,
		storage: st,
	}
}

// Upload 协调流式存盘与元数据入库
func (s *FileService) Upload(originalFilename string, folderID uint, src io.Reader) (*model.FileResponse, error) {
	if strings.TrimSpace(originalFilename) == "" {
		return nil, errors.New("filename cannot be empty")
	}

	if folderID != 0 {
		var folder model.Folder
		if err := s.db.First(&folder, folderID).Error; err != nil {
			return nil, errors.New("target folder not found")
		}
	}

	relPath, writtenSize, err := s.storage.Save(src)
	if err != nil {
		return nil, fmt.Errorf("storage save failed: %w", err)
	}

	ext := filepath.Ext(originalFilename)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	fileRecord := model.File{
		Filename:    originalFilename,
		FolderID:    folderID,
		StorageName: relPath,
		FileSize:    writtenSize,
		ContentType: contentType,
	}

	if err := s.db.Create(&fileRecord).Error; err != nil {
		_ = s.storage.Delete(relPath)
		return nil, fmt.Errorf("failed to save metadata to database: %w", err)
	}

	return s.toResponse(&fileRecord), nil
}

// GetFileForDownload 获取文件元数据和读取流供下载
func (s *FileService) GetFileForDownload(id uint) (*model.File, io.ReadCloser, error) {
	var fileRecord model.File
	if err := s.db.First(&fileRecord, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, errors.New("file record not found")
		}
		return nil, nil, err
	}

	stream, err := s.storage.Open(fileRecord.StorageName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open physical file: %w", err)
	}

	return &fileRecord, stream, nil
}

// ListFiles 获取所有文件并动态填充下载链接
func (s *FileService) ListFiles() ([]*model.FileResponse, error) {
	var files []model.File
	if err := s.db.Order("created_at desc").Find(&files).Error; err != nil {
		return nil, err
	}

	responses := make([]*model.FileResponse, len(files))
	for i, f := range files {
		responses[i] = s.toResponse(&f)
	}
	return responses, nil
}

// Rename 修改文件逻辑展示名（零磁盘操作）
func (s *FileService) Rename(id uint, newName string) (*model.FileResponse, error) {
	trimmed := strings.TrimSpace(newName)
	if trimmed == "" {
		return nil, errors.New("new filename cannot be empty")
	}

	var fileRecord model.File
	if err := s.db.First(&fileRecord, id).Error; err != nil {
		return nil, errors.New("file not found")
	}

	fileRecord.Filename = trimmed
	if err := s.db.Save(&fileRecord).Error; err != nil {
		return nil, err
	}

	return s.toResponse(&fileRecord), nil
}

// Delete 删除元数据与磁盘物理文件
func (s *FileService) Delete(id uint) error {
	var fileRecord model.File
	if err := s.db.First(&fileRecord, id).Error; err != nil {
		return errors.New("file not found")
	}

	if err := s.db.Delete(&fileRecord).Error; err != nil {
		return fmt.Errorf("failed to delete database record: %w", err)
	}

	if err := s.storage.Delete(fileRecord.StorageName); err != nil {
		fmt.Printf("warning: failed to delete physical file %s: %v\n", fileRecord.StorageName, err)
	}

	return nil
}

// MoveFile 移动文件到指定文件夹
func (s *FileService) MoveFile(id uint, targetFolderID uint) (*model.FileResponse, error) {
	var fileRecord model.File
	if err := s.db.First(&fileRecord, id).Error; err != nil {
		return nil, errors.New("file not found")
	}
	if targetFolderID != 0 {
		var targetFolder model.Folder
		if err := s.db.First(&targetFolder, targetFolderID).Error; err != nil {
			return nil, errors.New("target folder not found")
		}
	}
	fileRecord.FolderID = targetFolderID
	if err := s.db.Save(&fileRecord).Error; err != nil {
		return nil, err
	}
	return s.toResponse(&fileRecord), nil
}

// toResponse 将数据库 Model 转换为对外的 FileResponse，并动态生成下载相对路径
func (s *FileService) toResponse(f *model.File) *model.FileResponse {
	return &model.FileResponse{
		ID:          f.ID,
		Filename:    f.Filename,
		FolderID:    f.FolderID,
		FileSize:    f.FileSize,
		ContentType: f.ContentType,
		DownloadURL: fmt.Sprintf("/api/v1/files/%d/content", f.ID),
		CreatedAt:   f.CreatedAt,
		UpdatedAt:   f.UpdatedAt,
	}
}
