package model

import "time"

// Folder 代表存储在数据库中的文件夹元数据
type Folder struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"type:varchar(255);not null" json:"name"`
	ParentID  uint      `gorm:"index" json:"parent_id"`
	UserID    uint      `gorm:"not null;default:0;index" json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FolderResponse 代表返回给前端/客户端的文件夹视图
type FolderResponse struct {
	ID        uint      `json:"id"`
	Name      string    `json:"name"`
	ParentID  uint      `json:"parent_id"`
	UserID    uint      `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FolderContentResponse 用于获取某个目录下的全部内容
type FolderContentResponse struct {
	CurrentFolder *FolderResponse   `json:"current_folder"` // 当前目录信息（根目录时为 nil）
	Folders       []*FolderResponse `json:"folders"`        // 子文件夹列表
	Files         []*FileResponse   `json:"files"`          // 文件列表
}
