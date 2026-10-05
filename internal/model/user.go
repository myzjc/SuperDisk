package model

import "time"

// User 代表系统用户
type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"type:varchar(128);not null" json:"-"`
	Nickname     string    `gorm:"type:varchar(64)" json:"nickname"`
	Email        string    `gorm:"type:varchar(128)" json:"email"`
	Bio          string    `gorm:"type:text" json:"bio"`
	TokenVersion uint      `gorm:"not null;default:1" json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// UserProfileResponse 返回给前端的用户个人信息视图
type UserProfileResponse struct {
	ID        uint      `json:"id"`
	Username  string    `json:"username"`
	Nickname  string    `json:"nickname"`
	Email     string    `json:"email"`
	Bio       string    `json:"bio"`
	CreatedAt time.Time `json:"created_at"`
}
