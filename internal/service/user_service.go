package service

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/myzjc/SuperDisk/internal/model"
	"github.com/myzjc/SuperDisk/internal/pkg/jwt"
	"github.com/myzjc/SuperDisk/internal/pkg/password"
)

var (
	ErrUserAlreadyExists  = errors.New("username already exists")
	ErrInvalidCredentials = errors.New("invalid username or password")
)

type UserService struct {
	db         *gorm.DB
	jwtManager *jwt.JWTManager
}

// NewUserService 构造函数
func NewUserService(db *gorm.DB, jwtManager *jwt.JWTManager) *UserService {
	return &UserService{
		db:         db,
		jwtManager: jwtManager,
	}
}

// Register 处理用户注册业务
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
	}
	if err := s.db.Create(&newUser).Error; err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return &newUser, nil
}

// Login 处理用户登录业务
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

	token, err := s.jwtManager.GenerateToken(user.ID, user.Username)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return token, &user, nil
}
