package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/myzjc/SuperDisk/internal/handler"
	"github.com/myzjc/SuperDisk/internal/pkg/jwt"
	"github.com/myzjc/SuperDisk/internal/repository"
	"github.com/myzjc/SuperDisk/internal/service"
	"github.com/myzjc/SuperDisk/internal/storage"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	customMiddleware "github.com/myzjc/SuperDisk/internal/middleware"
)

const (
	defaultPort       = ":8080"
	defaultDBPath     = "netdisk.db"
	defaultStorageDir = "./data/uploads"
	jwtSecretKey      = "netdisk-secret-key-change-me-in-production"
	jwtDuration       = 24 * time.Hour
)

func main() {
	log.Println("[Init] Connecting to database...")
	db, err := repository.InitDB(defaultDBPath)
	if err != nil {
		log.Fatalf("Fatal: Failed to initialize database: %v", err)
	}

	log.Println("[Init] Initializing local disk storage...")
	diskStorage, err := storage.NewDiskStorage(defaultStorageDir)
	if err != nil {
		log.Fatalf("Fatal: Failed to initialize disk storage: %v", err)
	}

	jwtManager := jwt.NewJWTManager(jwtSecretKey, jwtDuration)

	fileService := service.NewFileService(db, diskStorage)
	fileHandler := handler.NewFileHandler(fileService)
	folderService := service.NewFolderService(db, diskStorage)
	folderHandler := handler.NewFolderHandler(folderService)
	shareService := service.NewShareService(db, diskStorage, folderService)
	shareHandler := handler.NewShareHandler(shareService)
	userService := service.NewUserService(db, jwtManager)
	authHandler := handler.NewAuthHandler(userService)

	e := echo.New()
	e.HideBanner = true

	e.Use(middleware.Logger())  // 记录请求日志
	e.Use(middleware.Recover()) // 防止 panic 导致进程崩溃
	e.Use(middleware.CORS())    // 支持跨域请求

	e.GET("/ping", func(c echo.Context) error {
		return c.String(http.StatusOK, "pong")
	})

	api := e.Group("/api/v1")
	{

		authGroup := api.Group("/auth")
		{
			authGroup.POST("/register", authHandler.Register)
			authGroup.POST("/login", authHandler.Login)
		}

		filesGroup := api.Group("/files")
		filesGroup.Use(customMiddleware.JWTMiddleware(jwtManager))
		{
			filesGroup.POST("", fileHandler.Upload)              // 上传文件
			filesGroup.GET("", fileHandler.List)                 // 查看已上传列表
			filesGroup.GET("/:id/content", fileHandler.Download) // 流式下载文件
			filesGroup.PATCH("/:id", fileHandler.Rename)         // 重命名文件
			filesGroup.DELETE("/:id", fileHandler.Delete)        // 删除文件
			filesGroup.PATCH("/:id/move", fileHandler.Move)      // 移动文件到目标文件夹
		}
		foldersGroup := api.Group("/folders")
		foldersGroup.Use(customMiddleware.JWTMiddleware(jwtManager))
		{
			foldersGroup.POST("", folderHandler.Create)                        // 新建文件夹
			foldersGroup.PATCH("/:id", folderHandler.Rename)                   // 重命名文件夹
			foldersGroup.PATCH("/:id/move", folderHandler.Move)                // 移动文件夹到目标文件夹
			foldersGroup.DELETE("/:id", folderHandler.Delete)                  // 删除文件夹
			foldersGroup.GET("/contents", folderHandler.GetRootContents)       // 获取根目录内容
			foldersGroup.GET("/:id/contents", folderHandler.GetFolderContents) // 获取指定目录内容
		}
		sharesGroup := api.Group("/shares")
		sharesGroup.Use(customMiddleware.JWTMiddleware(jwtManager))
		{
			sharesGroup.POST("", shareHandler.Create)               // 创建分享
			sharesGroup.GET("/:code", shareHandler.GetPublicDetail) // 获取分享详情
			sharesGroup.DELETE("/:code", shareHandler.Delete)       // 删除分享
			sharesGroup.GET("", shareHandler.List)                  // 查看所有分享
		}
		publicSharesGroup := api.Group("/public/shares")
		{
			publicSharesGroup.GET("/:code", shareHandler.GetPublicDetail)             // 公开查看分享详情
			publicSharesGroup.GET("/:code/download", shareHandler.DownloadPublicFile) // 流式下载分享的文件
		}
	}

	go func() {
		log.Printf("[Server] NetDisk running at http://localhost%s\n", defaultPort)
		if err := e.Start(defaultPort); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server startup failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Println("[Shutdown] Signal received, shutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := e.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("[Shutdown] Server exited cleanly.")
}
