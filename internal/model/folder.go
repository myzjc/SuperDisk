package model

import "time"

// Folder 代表存储在数据库中的文件夹元数据
type Folder struct {
	ID        uint      `gorm:"primaryKey" json:"id"`                   // 文件夹唯一标识
	Name      string    `gorm:"type:varchar(255);not null" json:"name"` // 文件夹名称
	ParentID  uint      `gorm:"index" json:"parent_id"`                 // 父文件夹 ID，为 0 表示根目录
	CreatedAt time.Time `json:"created_at"`                             // 创建时间
	UpdatedAt time.Time `json:"updated_at"`                             // 更新时间
}

// FolderResponse 代表返回给前端/客户端的文件夹视图
type FolderResponse struct {
	ID        uint      `json:"id"`
	Name      string    `json:"name"`
	ParentID  uint      `json:"parent_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FolderContentResponse 用于获取某个目录下的全部内容
type FolderContentResponse struct {
	CurrentFolder *FolderResponse   `json:"current_folder"` // 当前目录信息（根目录时为 nil）
	Folders       []*FolderResponse `json:"folders"`        // 子文件夹列表
	Files         []*FileResponse   `json:"files"`          // 文件列表
}
