package main

import (
	"log"

	"github.com/myzjc/SuperDisk/internal/repository"
)

func main() {
	// 建立数据库文件
	db, err := repository.InitDB("netdisk.db")
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	_ = db

	log.Println("Server is ready...")
}
