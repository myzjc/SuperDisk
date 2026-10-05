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

// Upload 流式上传+去重
func (s *FileService) Upload(userID uint, originalFilename string, folderID uint, src io.Reader) (*model.FileResponse, error) {
	if strings.TrimSpace(originalFilename) == "" {
		return nil, errors.New("filename cannot be empty")
	}

	if folderID != 0 {
		var folder model.Folder
		if err := s.db.Where("id = ? AND user_id = ?", folderID, userID).First(&folder).Error; err != nil {
			return nil, errors.New("target folder not found")
		}
	}

	var count int64
	if err := s.db.Model(&model.File{}).Where("user_id = ? AND folder_id = ? AND filename = ?", userID, folderID, originalFilename).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, errors.New("file with same name already exists in this directory")
	}

	relPath, writtenSize, fileHash, err := s.storage.Save(src)
	if err != nil {
		return nil, fmt.Errorf("storage save failed: %w", err)
	}

	ext := filepath.Ext(originalFilename)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	var targetBlob model.FileBlob

	err = s.db.Where("file_hash = ?", fileHash).First(&targetBlob).Error
	if err == nil {
		_ = s.storage.Delete(relPath)
		targetBlob.RefCount++
		if err := s.db.Save(&targetBlob).Error; err != nil {
			return nil, err
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		targetBlob = model.FileBlob{
			FileHash:    fileHash,
			StorageName: relPath,
			FileSize:    writtenSize,
			ContentType: contentType,
			RefCount:    1,
		}
		if err := s.db.Create(&targetBlob).Error; err != nil {
			_ = s.storage.Delete(relPath)
			return nil, fmt.Errorf("failed to create file blob: %w", err)
		}
	} else {
		_ = s.storage.Delete(relPath)
		return nil, err
	}

	fileRecord := model.File{
		Filename: originalFilename,
		FolderID: folderID,
		UserID:   userID,
		BlobID:   targetBlob.ID,
		Blob:     targetBlob,
	}

	if err := s.db.Create(&fileRecord).Error; err != nil {
		return nil, fmt.Errorf("failed to save metadata: %w", err)
	}

	return s.toResponse(&fileRecord), nil
}

// GetFileForDownload 下载当前用户拥有的文件
func (s *FileService) GetFileForDownload(userID uint, id uint) (*model.File, io.ReadCloser, error) {
	var fileRecord model.File
	if err := s.db.Preload("Blob").Where("id = ? AND user_id = ?", id, userID).First(&fileRecord).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, errors.New("file record not found")
		}
		return nil, nil, err
	}

	stream, err := s.storage.Open(fileRecord.Blob.StorageName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open physical file: %w", err)
	}

	return &fileRecord, stream, nil
}

// ListFiles 获取用户特定目录下的文件
func (s *FileService) ListFiles(userID uint, folderID uint) ([]*model.FileResponse, error) {
	var files []model.File
	if err := s.db.Preload("Blob").Order("created_at desc").Where("user_id = ? AND folder_id = ?", userID, folderID).Find(&files).Error; err != nil {
		return nil, err
	}

	responses := make([]*model.FileResponse, len(files))
	for i, f := range files {
		responses[i] = s.toResponse(&f)
	}
	return responses, nil
}

// Rename 重命名文件
func (s *FileService) Rename(userID uint, id uint, newName string) (*model.FileResponse, error) {
	trimmed := strings.TrimSpace(newName)
	if trimmed == "" {
		return nil, errors.New("new filename cannot be empty")
	}

	var fileRecord model.File
	if err := s.db.Preload("Blob").Where("id = ? AND user_id = ?", id, userID).First(&fileRecord).Error; err != nil {
		return nil, errors.New("file not found")
	}

	var count int64
	if err := s.db.Model(&model.File{}).Where("user_id = ? AND folder_id = ? AND filename = ? AND id != ?", userID, fileRecord.FolderID, trimmed, id).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, errors.New("file with same name already exists in this directory")
	}

	fileRecord.Filename = trimmed
	if err := s.db.Save(&fileRecord).Error; err != nil {
		return nil, err
	}

	return s.toResponse(&fileRecord), nil
}

// MoveFile 移动文件
func (s *FileService) MoveFile(userID uint, id uint, targetFolderID uint) (*model.FileResponse, error) {
	var fileRecord model.File
	if err := s.db.Preload("Blob").Where("id = ? AND user_id = ?", id, userID).First(&fileRecord).Error; err != nil {
		return nil, errors.New("file not found")
	}

	if targetFolderID != 0 {
		var targetFolder model.Folder
		if err := s.db.Where("id = ? AND user_id = ?", targetFolderID, userID).First(&targetFolder).Error; err != nil {
			return nil, errors.New("target folder not found")
		}
	}

	fileRecord.FolderID = targetFolderID
	if err := s.db.Save(&fileRecord).Error; err != nil {
		return nil, err
	}
	return s.toResponse(&fileRecord), nil
}

// Delete 删除文件
func (s *FileService) Delete(userID uint, id uint) error {
	var fileRecord model.File
	if err := s.db.Preload("Blob").Where("id = ? AND user_id = ?", id, userID).First(&fileRecord).Error; err != nil {
		return errors.New("file not found")
	}

	var shouldPhysicalDelete bool
	var storageNameToDelete string

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&fileRecord).Error; err != nil {
			return err
		}
		if err := tx.Where("target_type = ? AND target_id = ?", model.ShareTypeFile, id).Delete(&model.Share{}).Error; err != nil {
			return err
		}

		fileRecord.Blob.RefCount--
		if fileRecord.Blob.RefCount <= 0 {
			shouldPhysicalDelete = true
			storageNameToDelete = fileRecord.Blob.StorageName
			if err := tx.Delete(&fileRecord.Blob).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Save(&fileRecord.Blob).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to delete database record: %w", err)
	}

	if shouldPhysicalDelete {
		_ = s.storage.Delete(storageNameToDelete)
	}

	return nil
}

func (s *FileService) toResponse(f *model.File) *model.FileResponse {
	return &model.FileResponse{
		ID:          f.ID,
		Filename:    f.Filename,
		FolderID:    f.FolderID,
		UserID:      f.UserID,
		FileSize:    f.Blob.FileSize,
		ContentType: f.Blob.ContentType,
		FileHash:    f.Blob.FileHash,
		DownloadURL: fmt.Sprintf("/api/v1/files/%d/content", f.ID),
		CreatedAt:   f.CreatedAt,
		UpdatedAt:   f.UpdatedAt,
	}
}
