package service

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/myzjc/SuperDisk/internal/model"
	"github.com/myzjc/SuperDisk/internal/pkg/jwt"
	"github.com/myzjc/SuperDisk/internal/pkg/password"
	"github.com/myzjc/SuperDisk/internal/storage"
)

var (
	ErrUserAlreadyExists  = errors.New("username already exists")
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUserNotFound       = errors.New("user not found")
)

type UserService struct {
	db         *gorm.DB
	jwtManager *jwt.JWTManager
	storage    storage.Storage
}

func NewUserService(db *gorm.DB, jwtManager *jwt.JWTManager, st storage.Storage) *UserService {
	return &UserService{
		db:         db,
		jwtManager: jwtManager,
		storage:    st,
	}
}

// Register 注册用户
func (s *UserService) Register(username, plainPassword string) (*model.User, error) {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(username) > 32 {
		return nil, errors.New("username length must be between 3 and 32 characters")
	}
	if len(plainPassword) < 6 {
		return nil, errors.New("password must be at least 6 characters")
	}

	var existing model.User
	err := s.db.Where("username = ?", username).First(&existing).Error
	if err == nil {
		return nil, ErrUserAlreadyExists
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("database query error: %w", err)
	}

	hashedPassword, err := password.HashPassword(plainPassword)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	newUser := model.User{
		Username:     username,
		PasswordHash: hashedPassword,
		Nickname:     username, // 默认昵称为用户名
		TokenVersion: 1,
	}
	if err := s.db.Create(&newUser).Error; err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return &newUser, nil
}

// Login 用户登录（传入当前 TokenVersion 生成 Token）
func (s *UserService) Login(username, plainPassword string) (string, *model.User, error) {
	username = strings.TrimSpace(username)

	var user model.User
	if err := s.db.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil, ErrInvalidCredentials
		}
		return "", nil, fmt.Errorf("database query error: %w", err)
	}

	if err := password.CheckPassword(plainPassword, user.PasswordHash); err != nil {
		return "", nil, ErrInvalidCredentials
	}

	token, err := s.jwtManager.GenerateToken(user.ID, user.Username, user.TokenVersion)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return token, &user, nil
}

// GetProfile 获取用户个人信息
func (s *UserService) GetProfile(userID uint) (*model.UserProfileResponse, error) {
	var user model.User
	if err := s.db.First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return s.toProfileResponse(&user), nil
}

// UpdateProfile 修改个人信息
func (s *UserService) UpdateProfile(userID uint, nickname, email, bio string) (*model.UserProfileResponse, error) {
	var user model.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return nil, ErrUserNotFound
	}

	user.Nickname = strings.TrimSpace(nickname)
	user.Email = strings.TrimSpace(email)
	user.Bio = strings.TrimSpace(bio)

	if err := s.db.Save(&user).Error; err != nil {
		return nil, err
	}
	return s.toProfileResponse(&user), nil
}

// ChangePassword 修改密码（递增 TokenVersion 强制作废所有已登录的旧 Token）
func (s *UserService) ChangePassword(userID uint, oldPassword, newPassword string) error {
	if len(newPassword) < 6 {
		return errors.New("new password must be at least 6 characters")
	}

	var user model.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return ErrUserNotFound
	}

	if err := password.CheckPassword(oldPassword, user.PasswordHash); err != nil {
		return errors.New("incorrect old password")
	}

	newHash, err := password.HashPassword(newPassword)
	if err != nil {
		return err
	}

	user.PasswordHash = newHash
	user.TokenVersion++ // 密码修改，全端旧 Token 立即失效！
	return s.db.Save(&user).Error
}

// Logout 用户登出（递增 TokenVersion 使服务端记录的当前 Token 立即失效）
func (s *UserService) Logout(userID uint) error {
	return s.db.Model(&model.User{}).Where("id = ?", userID).Update("token_version", gorm.Expr("token_version + 1")).Error
}

// DeleteAccount 注销账户（级联清理该用户的所有文件、文件夹、分享，并根据引用计数清理物理存储）
func (s *UserService) DeleteAccount(userID uint) error {
	var user model.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return ErrUserNotFound
	}

	// 1. 查找用户的所有文件及其关联 Blob
	var userFiles []model.File
	if err := s.db.Preload("Blob").Where("user_id = ?", userID).Find(&userFiles).Error; err != nil {
		return err
	}

	var blobsToPhysicalDelete []model.FileBlob

	err := s.db.Transaction(func(tx *gorm.DB) error {
		// 删除用户分享
		if err := tx.Where("user_id = ?", userID).Delete(&model.Share{}).Error; err != nil {
			return err
		}
		// 删除用户文件夹
		if err := tx.Where("user_id = ?", userID).Delete(&model.Folder{}).Error; err != nil {
			return err
		}
		// 删除用户文件并维护 Blob 引用计数
		for _, f := range userFiles {
			if err := tx.Delete(&f).Error; err != nil {
				return err
			}
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
		// 删除用户自身
		return tx.Delete(&user).Error
	})
	if err != nil {
		return fmt.Errorf("failed to delete account: %w", err)
	}

	// 物理删除无任何引用的文件
	for _, b := range blobsToPhysicalDelete {
		_ = s.storage.Delete(b.StorageName)
	}

	return nil
}

func (s *UserService) toProfileResponse(u *model.User) *model.UserProfileResponse {
	return &model.UserProfileResponse{
		ID:        u.ID,
		Username:  u.Username,
		Nickname:  u.Nickname,
		Email:     u.Email,
		Bio:       u.Bio,
		CreatedAt: u.CreatedAt,
	}
}
