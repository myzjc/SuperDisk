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

// CreateFolder 创建文件夹
func (s *FolderService) CreateFolder(name string, parentID uint) (*model.FolderResponse, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, `/\:*?"<>|`) {
		return nil, ErrInvalidFolderName
	}

	if parentID != 0 {
		var parent model.Folder
		if err := s.db.First(&parent, parentID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.New("parent folder not found")
			}
			return nil, err
		}
	}

	var count int64
	if err := s.db.Model(&model.Folder{}).Where("name = ? AND parent_id = ?", name, parentID).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrFolderAlreadyExists
	}

	folder := model.Folder{
		Name:     name,
		ParentID: parentID,
	}
	if err := s.db.Create(&folder).Error; err != nil {
		return nil, err
	}

	return s.toResponse(&folder), nil
}

// RenameFolder 重命名文件夹
func (s *FolderService) RenameFolder(id uint, newName string) (*model.FolderResponse, error) {
	newName = strings.TrimSpace(newName)
	if newName == "" || strings.ContainsAny(newName, `/\:*?"<>|`) {
		return nil, ErrInvalidFolderName
	}

	var folder model.Folder
	if err := s.db.First(&folder, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrFolderNotFound
		}
		return nil, err
	}

	var count int64
	if err := s.db.Model(&model.Folder{}).Where("name = ? AND parent_id = ? AND id != ?", newName, folder.ParentID, id).Count(&count).Error; err != nil {
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

// MoveFolder 移动文件夹
func (s *FolderService) MoveFolder(id uint, targetParentID uint) (*model.FolderResponse, error) {
	var folder model.Folder
	if err := s.db.First(&folder, id).Error; err != nil {
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
		if err := s.db.First(&targetFolder, targetParentID).Error; err != nil {
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
			if err := s.db.First(&ancestor, currParentID).Error; err != nil {
				break
			}
			currParentID = ancestor.ParentID
		}
	}

	var count int64
	if err := s.db.Model(&model.Folder{}).Where("name = ? AND parent_id = ? AND id != ?", folder.Name, targetParentID, id).Count(&count).Error; err != nil {
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

func (s *FolderService) DeleteFolder(id uint) error {
	var rootFolder model.Folder
	if err := s.db.First(&rootFolder, id).Error; err != nil {
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
		if err := s.db.Model(&model.Folder{}).Where("parent_id = ?", currID).Pluck("id", &subIDs).Error; err != nil {
			return err
		}
		for _, sid := range subIDs {
			allFolderIDs = append(allFolderIDs, sid)
			queue = append(queue, sid)
		}
	}

	var filesToDelete []model.File
	if err := s.db.Where("folder_id IN ?", allFolderIDs).Find(&filesToDelete).Error; err != nil {
		return err
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if len(filesToDelete) > 0 {
			if err := tx.Where("folder_id IN ?", allFolderIDs).Delete(&model.File{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("id IN ?", allFolderIDs).Delete(&model.Folder{}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to delete database records: %w", err)
	}

	for _, f := range filesToDelete {
		if err := s.storage.Delete(f.StorageName); err != nil {
			fmt.Printf("warning: failed to delete physical file %s: %v\n", f.StorageName, err)
		}
	}

	return nil
}

// GetFolderContents 获取指定目录下的内容
func (s *FolderService) GetFolderContents(folderID uint) (*model.FolderContentResponse, error) {
	var currentFolder *model.FolderResponse

	if folderID != 0 {
		var f model.Folder
		if err := s.db.First(&f, folderID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrFolderNotFound
			}
			return nil, err
		}
		currentFolder = s.toResponse(&f)
	}

	var folders []model.Folder
	if err := s.db.Order("name asc").Where("parent_id = ?", folderID).Find(&folders).Error; err != nil {
		return nil, err
	}

	var files []model.File
	if err := s.db.Order("created_at desc").Where("folder_id = ?", folderID).Find(&files).Error; err != nil {
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
			FileSize:    file.FileSize,
			ContentType: file.ContentType,
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
		CreatedAt: f.CreatedAt,
		UpdatedAt: f.UpdatedAt,
	}
}
