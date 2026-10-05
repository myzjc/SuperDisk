package model

import "time"

const (
	StorageTypeLocal = "local"
	StorageTypeS3    = "s3"
)

// FileBlob 代表物理磁盘或对象存储上的真实文件数据块
type FileBlob struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	FileHash    string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"file_hash"`        // 文件 SHA-256 哈希
	StorageType string    `gorm:"type:varchar(32);not null;default:'local'" json:"storage_type"` //"local" 或 "s3"
	StorageName string    `gorm:"uniqueIndex;not null" json:"-"`                                 // 本地相对路径 或 对象存储的 Object Key
	FileSize    int64     `gorm:"not null" json:"file_size"`                                     // 文件字节大小
	ContentType string    `gorm:"type:varchar(128)" json:"content_type"`                         // MIME 类型
	RefCount    int64     `gorm:"not null;default:1" json:"ref_count"`                           // 引用计数
	CreatedAt   time.Time `json:"created_at"`
}
