package model

import "time"

// UploadSession 代表大文件分片上传的会话元数据
type UploadSession struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UploadID    string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"upload_id"`
	UserID      uint      `gorm:"not null;index" json:"user_id"`
	Filename    string    `gorm:"not null" json:"filename"`
	FolderID    uint      `gorm:"not null;default:0" json:"folder_id"`
	TotalSize   int64     `gorm:"not null" json:"total_size"`   // 文件总字节数
	ChunkSize   int64     `gorm:"not null" json:"chunk_size"`   // 单个切片字节数
	TotalChunks int       `gorm:"not null" json:"total_chunks"` // 切片总数
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ChunkUploadStatusResponse 返回给客户端的断点续传状态响应
type ChunkUploadStatusResponse struct {
	UploadID       string `json:"upload_id"`
	Filename       string `json:"filename"`
	TotalSize      int64  `json:"total_size"`
	ChunkSize      int64  `json:"chunk_size"`
	TotalChunks    int    `json:"total_chunks"`
	UploadedChunks []int  `json:"uploaded_chunks"` // 已成功上传的分片索引数组
}
