package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

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
}

func NewShareService(db *gorm.DB, st storage.Storage, fs *FolderService) *ShareService {
	return &ShareService{
		db:            db,
		storage:       st,
		folderService: fs,
	}
}

// CreateShare 创建分享链接
func (s *ShareService) CreateShare(targetType string, targetID uint) (*model.ShareResponse, error) {
	if targetType != model.ShareTypeFile && targetType != model.ShareTypeFolder {
		return nil, ErrInvalidShareTarget
	}

	var targetName string

	if targetType == model.ShareTypeFile {
		var f model.File
		if err := s.db.First(&f, targetID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.New("file not found")
			}
			return nil, err
		}
		targetName = f.Filename
	} else {
		var f model.Folder
		if err := s.db.First(&f, targetID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.New("folder not found")
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

// ListShares 列出所有分享链接
func (s *ShareService) ListShares() ([]*model.ShareResponse, error) {
	var shares []model.Share
	if err := s.db.Order("created_at desc").Find(&shares).Error; err != nil {
		return nil, err
	}

	responses := make([]*model.ShareResponse, len(shares))
	for i, share := range shares {
		targetName := "[已失效或已被删除]"
		if share.TargetType == model.ShareTypeFile {
			var f model.File
			if err := s.db.Select("filename").First(&f, share.TargetID).Error; err == nil {
				targetName = f.Filename
			}
		} else {
			var f model.Folder
			if err := s.db.Select("name").First(&f, share.TargetID).Error; err == nil {
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
func (s *ShareService) DeleteShare(id uint) error {
	var share model.Share
	if err := s.db.First(&share, id).Error; err != nil {
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
		if err := s.db.First(&f, share.TargetID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrShareTargetNotFound
			}
			return nil, err
		}
		detail.FileInfo = &model.FileResponse{
			ID:          f.ID,
			Filename:    f.Filename,
			FolderID:    f.FolderID,
			FileSize:    f.FileSize,
			ContentType: f.ContentType,
			DownloadURL: fmt.Sprintf("/api/v1/public/shares/%s/download", share.ShareCode),
			CreatedAt:   f.CreatedAt,
			UpdatedAt:   f.UpdatedAt,
		}
	} else {
		contents, err := s.folderService.GetFolderContents(share.TargetID)
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
func (s *ShareService) DownloadSharedFile(shareCode string) (*model.File, io.ReadCloser, error) {
	var share model.Share
	if err := s.db.Where("share_code = ?", shareCode).First(&share).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrShareNotFound
		}
		return nil, nil, err
	}

	if share.TargetType != model.ShareTypeFile {
		return nil, nil, errors.New("cannot download a folder directly as a single file")
	}

	var f model.File
	if err := s.db.First(&f, share.TargetID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrShareTargetNotFound
		}
		return nil, nil, err
	}

	stream, err := s.storage.Open(f.StorageName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open physical storage: %w", err)
	}

	return &f, stream, nil
}

// generateShareCode 生成随机字符串
func generateShareCode(byteLen int) (string, error) {
	bytes := make([]byte, byteLen)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
