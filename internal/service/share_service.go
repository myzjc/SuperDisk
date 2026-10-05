package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/myzjc/SuperDisk/internal/model"
	"github.com/myzjc/SuperDisk/internal/storage"
)

var (
	ErrShareNotFound       = errors.New("share link not found")
	ErrShareTargetNotFound = errors.New("shared resource no longer exists or has been deleted")
	ErrInvalidShareTarget  = errors.New("invalid share target type, must be 'file' or 'folder'")
)

type ShareService struct {
	db            *gorm.DB
	storage       storage.Storage
	folderService *FolderService
	s3Storage     *storage.S3Storage
}

func NewShareService(db *gorm.DB, st storage.Storage, s3 *storage.S3Storage, fs *FolderService) *ShareService {
	return &ShareService{
		db:            db,
		storage:       st,
		s3Storage:     s3,
		folderService: fs,
	}
}

// CreateShare 创建分享链接
func (s *ShareService) CreateShare(userID uint, targetType string, targetID uint) (*model.ShareResponse, error) {
	if targetType != model.ShareTypeFile && targetType != model.ShareTypeFolder {
		return nil, ErrInvalidShareTarget
	}

	var targetName string

	if targetType == model.ShareTypeFile {
		var f model.File
		if err := s.db.Where("id = ? AND user_id = ?", targetID, userID).First(&f).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.New("file not found or access denied")
			}
			return nil, err
		}
		targetName = f.Filename
	} else {
		var f model.Folder
		if err := s.db.Where("id = ? AND user_id = ?", targetID, userID).First(&f).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.New("folder not found or access denied")
			}
			return nil, err
		}
		targetName = f.Name
	}

	shareCode, err := generateShareCode(5)
	if err != nil {
		return nil, fmt.Errorf("failed to generate share code: %w", err)
	}

	share := model.Share{
		ShareCode:  shareCode,
		TargetType: targetType,
		TargetID:   targetID,
		UserID:     userID,
	}

	if err := s.db.Create(&share).Error; err != nil {
		return nil, err
	}

	return &model.ShareResponse{
		ID:         share.ID,
		ShareCode:  share.ShareCode,
		ShareURL:   fmt.Sprintf("/api/v1/public/shares/%s", share.ShareCode),
		TargetType: share.TargetType,
		TargetID:   share.TargetID,
		TargetName: targetName,
		CreatedAt:  share.CreatedAt,
	}, nil
}

// ListShares 仅列出当前登录用户的分享链接
func (s *ShareService) ListShares(userID uint) ([]*model.ShareResponse, error) {
	var shares []model.Share
	if err := s.db.Where("user_id = ?", userID).Order("created_at desc").Find(&shares).Error; err != nil {
		return nil, err
	}

	responses := make([]*model.ShareResponse, len(shares))
	for i, share := range shares {
		targetName := "[已失效或已被删除]"
		if share.TargetType == model.ShareTypeFile {
			var f model.File
			if err := s.db.Select("filename").Where("id = ? AND user_id = ?", share.TargetID, userID).First(&f).Error; err == nil {
				targetName = f.Filename
			}
		} else {
			var f model.Folder
			if err := s.db.Select("name").Where("id = ? AND user_id = ?", share.TargetID, userID).First(&f).Error; err == nil {
				targetName = f.Name
			}
		}

		responses[i] = &model.ShareResponse{
			ID:         share.ID,
			ShareCode:  share.ShareCode,
			ShareURL:   fmt.Sprintf("/api/v1/public/shares/%s", share.ShareCode),
			TargetType: share.TargetType,
			TargetID:   share.TargetID,
			TargetName: targetName,
			CreatedAt:  share.CreatedAt,
		}
	}

	return responses, nil
}

// DeleteShare 删除指定的分享链接
func (s *ShareService) DeleteShare(userID uint, id uint) error {
	var share model.Share
	if err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&share).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrShareNotFound
		}
		return err
	}

	return s.db.Delete(&share).Error
}

// GetPublicShareDetail 公开提取分享内容
func (s *ShareService) GetPublicShareDetail(shareCode string) (*model.PublicShareDetail, error) {
	var share model.Share
	if err := s.db.Where("share_code = ?", shareCode).First(&share).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrShareNotFound
		}
		return nil, err
	}

	detail := &model.PublicShareDetail{
		ShareCode:  share.ShareCode,
		TargetType: share.TargetType,
	}

	if share.TargetType == model.ShareTypeFile {
		var f model.File
		if err := s.db.Preload("Blob").First(&f, share.TargetID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrShareTargetNotFound
			}
			return nil, err
		}
		detail.FileInfo = &model.FileResponse{
			ID:          f.ID,
			Filename:    f.Filename,
			FolderID:    f.FolderID,
			UserID:      f.UserID,
			FileSize:    f.Blob.FileSize,
			ContentType: f.Blob.ContentType,
			FileHash:    f.Blob.FileHash,
			DownloadURL: fmt.Sprintf("/api/v1/public/shares/%s/download", share.ShareCode),
			CreatedAt:   f.CreatedAt,
			UpdatedAt:   f.UpdatedAt,
		}
	} else {
		contents, err := s.folderService.GetFolderContents(share.UserID, share.TargetID)
		if err != nil {
			if errors.Is(err, ErrFolderNotFound) {
				return nil, ErrShareTargetNotFound
			}
			return nil, err
		}
		detail.FolderInfo = contents
	}

	return detail, nil
}

// DownloadSharedFile 流式下载分享的文件
func (s *ShareService) DownloadSharedFile(ctx context.Context, shareCode string) (*FileDownloadResult, error) {
	var share model.Share
	if err := s.db.Where("share_code = ?", shareCode).First(&share).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrShareNotFound
		}
		return nil, err
	}
	if share.TargetType != model.ShareTypeFile {
		return nil, errors.New("cannot download a folder directly as a single file")
	}
	var f model.File
	if err := s.db.Preload("Blob").First(&f, share.TargetID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrShareTargetNotFound
		}
		return nil, err
	}
	if f.Blob.StorageType == model.StorageTypeS3 && s.s3Storage != nil {
		presignedURL, err := s.s3Storage.GetPresignedURL(ctx, f.Blob.StorageName, f.Filename, 15*time.Minute)
		if err != nil {
			return nil, fmt.Errorf("failed to generate presigned download link: %w", err)
		}
		return &FileDownloadResult{
			File:         &f,
			PresignedURL: presignedURL,
			IsRemote:     true,
		}, nil
	}
	stream, err := s.storage.Open(f.Blob.StorageName)
	if err != nil {
		return nil, fmt.Errorf("failed to open physical storage: %w", err)
	}
	return &FileDownloadResult{
		File:     &f,
		Stream:   stream,
		IsRemote: false,
	}, nil
}

func generateShareCode(byteLen int) (string, error) {
	bytes := make([]byte, byteLen)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
