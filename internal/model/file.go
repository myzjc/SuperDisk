// Package model 定义应用使用的数据模型。
package model

import "time"

// File 代表存储在数据库中的文件元数据模型
type File struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Filename    string    `gorm:"not null" json:"filename"`              // 用户看到的文件名
	StorageName string    `gorm:"uniqueIndex;not null" json:"-"`         // 磁盘上的真实物理文件名（唯一索引）
	FileSize    int64     `gorm:"not null" json:"file_size"`             // 文件字节大小
	ContentType string    `gorm:"type:varchar(128)" json:"content_type"` // 文件 MIME 类型
	CreatedAt   time.Time `json:"created_at"`                            // 上传时间
	UpdatedAt   time.Time `json:"updated_at"`                            // 修改时间
}

// FileResponse 代表返回给前端/客户端的 JSON 视图
type FileResponse struct {
	ID          uint      `json:"id"`
	Filename    string    `json:"filename"`
	FileSize    int64     `json:"file_size"`
	ContentType string    `json:"content_type"`
	DownloadURL string    `json:"download_url"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
