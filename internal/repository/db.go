// Package repository 提供数据库访问相关的功能。
package repository

import (
	"log"

	"github.com/glebarez/sqlite"
	"github.com/myzjc/SuperDisk/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// InitDB 用于初始化数据库
func InitDB(dbPath string) (*gorm.DB, error) {
	// 打开数据库连接
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		// SQL 打印日志(发布时删除)
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, err
	}

	err = db.AutoMigrate(&model.File{})
	if err != nil {
		return nil, err
	}

	log.Println("Database initialized and migrated successfully!")
	return db, nil
}
