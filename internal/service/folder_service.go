package service

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/myzjc/SuperDisk/internal/model"
	"github.com/myzjc/SuperDisk/internal/storage"
)

var (
	ErrFolderNotFound      = errors.New("folder not found")
	ErrFolderAlreadyExists = errors.New("folder with same name already exists in this directory")
	ErrInvalidFolderName   = errors.New("invalid folder name")
	ErrCannotMoveToChild   = errors.New("cannot move folder into itself or its subfolder")
)

type FolderService struct {
	db      *gorm.DB
	storage storage.Storage
}

func NewFolderService(db *gorm.DB, st storage.Storage) *FolderService {
	return &FolderService{
		db:      db,
		storage: st,
	}
}

func (s *FolderService) CreateFolder(userID uint, name string, parentID uint) (*model.FolderResponse, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, `/\:*?"<>|`) {
		return nil, ErrInvalidFolderName
	}

	if parentID != 0 {
		var parent model.Folder
		if err := s.db.Where("id = ? AND user_id = ?", parentID, userID).First(&parent).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.New("parent folder not found")
			}
			return nil, err
		}
	}

	var count int64
	if err := s.db.Model(&model.Folder{}).Where("user_id = ? AND name = ? AND parent_id = ?", userID, name, parentID).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrFolderAlreadyExists
	}

	folder := model.Folder{
		Name:     name,
		ParentID: parentID,
		UserID:   userID,
	}
	if err := s.db.Create(&folder).Error; err != nil {
		return nil, err
	}

	return s.toResponse(&folder), nil
}

func (s *FolderService) RenameFolder(userID uint, id uint, newName string) (*model.FolderResponse, error) {
	newName = strings.TrimSpace(newName)
	if newName == "" || strings.ContainsAny(newName, `/\:*?"<>|`) {
		return nil, ErrInvalidFolderName
	}

	var folder model.Folder
	if err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&folder).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrFolderNotFound
		}
		return nil, err
	}

	var count int64
	if err := s.db.Model(&model.Folder{}).Where("user_id = ? AND name = ? AND parent_id = ? AND id != ?", userID, newName, folder.ParentID, id).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrFolderAlreadyExists
	}

	folder.Name = newName
	if err := s.db.Save(&folder).Error; err != nil {
		return nil, err
	}

	return s.toResponse(&folder), nil
}

func (s *FolderService) MoveFolder(userID uint, id uint, targetParentID uint) (*model.FolderResponse, error) {
	var folder model.Folder
	if err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&folder).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrFolderNotFound
		}
		return nil, err
	}

	if targetParentID == id {
		return nil, ErrCannotMoveToChild
	}

	if targetParentID != 0 {
		var targetFolder model.Folder
		if err := s.db.Where("id = ? AND user_id = ?", targetParentID, userID).First(&targetFolder).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.New("target folder not found")
			}
			return nil, err
		}

		currParentID := targetFolder.ParentID
		for currParentID != 0 {
			if currParentID == id {
				return nil, ErrCannotMoveToChild
			}
			var ancestor model.Folder
			if err := s.db.Where("id = ? AND user_id = ?", currParentID, userID).First(&ancestor).Error; err != nil {
				break
			}
			currParentID = ancestor.ParentID
		}
	}

	var count int64
	if err := s.db.Model(&model.Folder{}).Where("user_id = ? AND name = ? AND parent_id = ? AND id != ?", userID, folder.Name, targetParentID, id).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrFolderAlreadyExists
	}

	folder.ParentID = targetParentID
	if err := s.db.Save(&folder).Error; err != nil {
		return nil, err
	}

	return s.toResponse(&folder), nil
}

func (s *FolderService) DeleteFolder(userID uint, id uint) error {
	var rootFolder model.Folder
	if err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&rootFolder).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrFolderNotFound
		}
		return err
	}

	allFolderIDs := []uint{id}
	queue := []uint{id}

	for len(queue) > 0 {
		currID := queue[0]
		queue = queue[1:]

		var subIDs []uint
		if err := s.db.Model(&model.Folder{}).Where("user_id = ? AND parent_id = ?", userID, currID).Pluck("id", &subIDs).Error; err != nil {
			return err
		}
		for _, sid := range subIDs {
			allFolderIDs = append(allFolderIDs, sid)
			queue = append(queue, sid)
		}
	}

	var filesToDelete []model.File
	if err := s.db.Preload("Blob").Where("user_id = ? AND folder_id IN ?", userID, allFolderIDs).Find(&filesToDelete).Error; err != nil {
		return err
	}

	var blobsToPhysicalDelete []model.FileBlob

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if len(filesToDelete) > 0 {
			var fileIDs []uint
			for _, f := range filesToDelete {
				fileIDs = append(fileIDs, f.ID)
			}
			_ = tx.Where("target_type = ? AND target_id IN ?", model.ShareTypeFile, fileIDs).Delete(&model.Share{}).Error

			if err := tx.Where("user_id = ? AND folder_id IN ?", userID, allFolderIDs).Delete(&model.File{}).Error; err != nil {
				return err
			}

			for _, f := range filesToDelete {
				f.Blob.RefCount--
				if f.Blob.RefCount <= 0 {
					blobsToPhysicalDelete = append(blobsToPhysicalDelete, f.Blob)
					if err := tx.Delete(&f.Blob).Error; err != nil {
						return err
					}
				} else {
					if err := tx.Save(&f.Blob).Error; err != nil {
						return err
					}
				}
			}
		}

		_ = tx.Where("target_type = ? AND target_id IN ?", model.ShareTypeFolder, allFolderIDs).Delete(&model.Share{}).Error

		return tx.Where("user_id = ? AND id IN ?", userID, allFolderIDs).Delete(&model.Folder{}).Error
	})
	if err != nil {
		return fmt.Errorf("failed to delete database records: %w", err)
	}

	for _, b := range blobsToPhysicalDelete {
		_ = s.storage.Delete(b.StorageName)
	}

	return nil
}

func (s *FolderService) GetFolderContents(userID uint, folderID uint) (*model.FolderContentResponse, error) {
	var currentFolder *model.FolderResponse

	if folderID != 0 {
		var f model.Folder
		if err := s.db.Where("id = ? AND user_id = ?", folderID, userID).First(&f).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrFolderNotFound
			}
			return nil, err
		}
		currentFolder = s.toResponse(&f)
	}

	var folders []model.Folder
	if err := s.db.Order("name asc").Where("user_id = ? AND parent_id = ?", userID, folderID).Find(&folders).Error; err != nil {
		return nil, err
	}

	var files []model.File
	if err := s.db.Preload("Blob").Order("created_at desc").Where("user_id = ? AND folder_id = ?", userID, folderID).Find(&files).Error; err != nil {
		return nil, err
	}

	folderResps := make([]*model.FolderResponse, len(folders))
	for i, f := range folders {
		folderResps[i] = s.toResponse(&f)
	}

	fileResps := make([]*model.FileResponse, len(files))
	for i, file := range files {
		fileResps[i] = &model.FileResponse{
			ID:          file.ID,
			Filename:    file.Filename,
			FolderID:    file.FolderID,
			UserID:      file.UserID,
			FileSize:    file.Blob.FileSize,
			ContentType: file.Blob.ContentType,
			FileHash:    file.Blob.FileHash,
			DownloadURL: fmt.Sprintf("/api/v1/files/%d/content", file.ID),
			CreatedAt:   file.CreatedAt,
			UpdatedAt:   file.UpdatedAt,
		}
	}

	return &model.FolderContentResponse{
		CurrentFolder: currentFolder,
		Folders:       folderResps,
		Files:         fileResps,
	}, nil
}

func (s *FolderService) toResponse(f *model.Folder) *model.FolderResponse {
	return &model.FolderResponse{
		ID:        f.ID,
		Name:      f.Name,
		ParentID:  f.ParentID,
		UserID:    f.UserID,
		CreatedAt: f.CreatedAt,
		UpdatedAt: f.UpdatedAt,
	}
}
