package model

import "time"

// FileBlob 代表物理磁盘上的真实文件数据块
type FileBlob struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	FileHash    string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"file_hash"` // 文件 SHA-256 哈希
	StorageName string    `gorm:"uniqueIndex;not null" json:"-"`                          // 物理存储相对路径
	FileSize    int64     `gorm:"not null" json:"file_size"`                              // 物理文件大小
	ContentType string    `gorm:"type:varchar(128)" json:"content_type"`                  // MIME 类型
	RefCount    int64     `gorm:"not null;default:1" json:"ref_count"`                    // 引用计数
	CreatedAt   time.Time `json:"created_at"`
}
