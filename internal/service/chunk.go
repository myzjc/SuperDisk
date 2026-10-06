package service

import (
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/myzjc/SuperDisk/internal/model"
)

// InitChunkUpload 初始化大文件分片上传
func (s *FileService) InitChunkUpload(userID uint, filename string, folderID uint, totalSize int64, chunkSize int64) (*model.UploadSession, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return nil, errors.New("filename cannot be empty")
	}
	if totalSize <= 0 {
		return nil, errors.New("total_size must be greater than 0")
	}

	if chunkSize <= 0 {
		chunkSize = 5 * 1024 * 1024
	} else if chunkSize < 1024*1024 {
		return nil, errors.New("chunk_size must be at least 1MB")
	}

	if folderID != 0 {
		var folder model.Folder
		if err := s.db.Where("id = ? AND user_id = ?", folderID, userID).First(&folder).Error; err != nil {
			return nil, errors.New("target folder not found")
		}
	}

	var count int64
	if err := s.db.Model(&model.File{}).Where("user_id = ? AND folder_id = ? AND filename = ?", userID, folderID, filename).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, errors.New("file with same name already exists in this directory")
	}

	totalChunks := int(math.Ceil(float64(totalSize) / float64(chunkSize)))
	uploadID := uuid.New().String()

	session := model.UploadSession{
		UploadID:    uploadID,
		UserID:      userID,
		Filename:    filename,
		FolderID:    folderID,
		TotalSize:   totalSize,
		ChunkSize:   chunkSize,
		TotalChunks: totalChunks,
	}

	if err := s.db.Create(&session).Error; err != nil {
		return nil, fmt.Errorf("failed to create upload session: %w", err)
	}

	return &session, nil
}

// UploadChunk 上传单个分片
func (s *FileService) UploadChunk(userID uint, uploadID string, chunkIndex int, src io.Reader) error {
	var session model.UploadSession
	if err := s.db.Where("upload_id = ? AND user_id = ?", uploadID, userID).First(&session).Error; err != nil {
		return errors.New("upload session not found")
	}

	if chunkIndex < 0 || chunkIndex >= session.TotalChunks {
		return fmt.Errorf("invalid chunk_index %d: must be between 0 and %d", chunkIndex, session.TotalChunks-1)
	}

	return s.storage.SaveChunk(uploadID, chunkIndex, src)
}

// GetChunkUploadStatus 获取上传进度与已完成切片列表
func (s *FileService) GetChunkUploadStatus(userID uint, uploadID string) (*model.ChunkUploadStatusResponse, error) {
	var session model.UploadSession
	if err := s.db.Where("upload_id = ? AND user_id = ?", uploadID, userID).First(&session).Error; err != nil {
		return nil, errors.New("upload session not found")
	}

	uploaded, err := s.storage.GetUploadedChunks(uploadID)
	if err != nil {
		return nil, fmt.Errorf("failed to get uploaded chunks: %w", err)
	}

	return &model.ChunkUploadStatusResponse{
		UploadID:       session.UploadID,
		Filename:       session.Filename,
		TotalSize:      session.TotalSize,
		ChunkSize:      session.ChunkSize,
		TotalChunks:    session.TotalChunks,
		UploadedChunks: uploaded,
	}, nil
}

// CompleteChunkUpload 合并切片，计算哈希，触发秒传去重与入库
func (s *FileService) CompleteChunkUpload(userID uint, uploadID string) (*model.FileResponse, error) {
	var session model.UploadSession
	if err := s.db.Where("upload_id = ? AND user_id = ?", uploadID, userID).First(&session).Error; err != nil {
		return nil, errors.New("upload session not found")
	}

	uploaded, err := s.storage.GetUploadedChunks(uploadID)
	if err != nil {
		return nil, err
	}
	if len(uploaded) != session.TotalChunks {
		return nil, fmt.Errorf("cannot complete upload: missing chunks (got %d of %d)", len(uploaded), session.TotalChunks)
	}

	relPath, writtenSize, fileHash, err := s.storage.MergeChunks(uploadID, session.TotalChunks)
	if err != nil {
		return nil, fmt.Errorf("failed to merge chunks: %w", err)
	}

	ext := filepath.Ext(session.Filename)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	var targetBlob model.FileBlob
	err = s.db.Where("file_hash = ?", fileHash).First(&targetBlob).Error
	if err == nil {
		_ = s.storage.Delete(relPath)
		targetBlob.RefCount++
		_ = s.db.Save(&targetBlob)
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		targetBlob = model.FileBlob{
			FileHash:    fileHash,
			StorageType: model.StorageTypeLocal,
			StorageName: relPath,
			FileSize:    writtenSize,
			ContentType: contentType,
			RefCount:    1,
		}
		if err := s.db.Create(&targetBlob).Error; err != nil {
			_ = s.storage.Delete(relPath)
			return nil, err
		}
	} else {
		_ = s.storage.Delete(relPath)
		return nil, err
	}

	fileRecord := model.File{
		Filename: session.Filename,
		FolderID: session.FolderID,
		UserID:   userID,
		BlobID:   targetBlob.ID,
		Blob:     targetBlob,
	}

	if err := s.db.Create(&fileRecord).Error; err != nil {
		return nil, err
	}

	_ = s.db.Delete(&session)

	return s.toResponse(&fileRecord), nil
}
