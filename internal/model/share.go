package model

import "time"

const (
	ShareTypeFile   = "file"
	ShareTypeFolder = "folder"
)

// Share 代表数据库中的分享链接记录
type Share struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ShareCode  string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"share_code"` // 访问分享链接的唯一提取码
	TargetType string    `gorm:"type:varchar(32);not null;index" json:"target_type"`      // "file" 或 "folder"
	TargetID   uint      `gorm:"not null;index" json:"target_id"`                         // 指向的文件或文件夹 ID
	UserID     uint      `gorm:"not null;default:0;index" json:"user_id"`                 // 分享创建者 ID
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ShareResponse 代表给分享管理者展示的列表信息
type ShareResponse struct {
	ID         uint      `json:"id"`
	ShareCode  string    `json:"share_code"`
	ShareURL   string    `json:"share_url"`   // 完整的分享访问相对路径
	TargetType string    `json:"target_type"` // "file" 或 "folder"
	TargetID   uint      `json:"target_id"`
	TargetName string    `json:"target_name"` // 展示目标文件或文件夹的当前名字
	CreatedAt  time.Time `json:"created_at"`
}

// PublicShareDetail 代表访问分享链接时获取到的详情
type PublicShareDetail struct {
	ShareCode  string                 `json:"share_code"`
	TargetType string                 `json:"target_type"`           // "file" 或 "folder"
	FileInfo   *FileResponse          `json:"file_info,omitempty"`   // 文件，返回文件元数据与下载链接
	FolderInfo *FolderContentResponse `json:"folder_info,omitempty"` // 文件夹，返回目录内容
}
