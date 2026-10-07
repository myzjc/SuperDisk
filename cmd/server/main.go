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

	s3Endpoint := os.Getenv("S3_ENDPOINT")
	if s3Endpoint == "" {
		s3Endpoint = "127.0.0.1:9000"
	}
	s3Bucket := os.Getenv("S3_BUCKET")
	if s3Bucket == "" {
		s3Bucket = "superdisk"
	}
	s3AK := os.Getenv("S3_ACCESS_KEY")
	if s3AK == "" {
		s3AK = "minioadmin"
	}
	s3SK := os.Getenv("S3_SECRET_KEY")
	if s3SK == "" {
		s3SK = "minioadmin"
	}
	s3Storage, err := storage.NewS3Storage(storage.S3Config{
		Endpoint:  s3Endpoint,
		Bucket:    s3Bucket,
		AccessKey: s3AK,
		SecretKey: s3SK,
		UseSSL:    false,
	})
	if err != nil {
		log.Printf("[Init] Warning: S3 storage initialization: %v", err)
	}
	jwtManager := jwt.NewJWTManager(jwtSecretKey, jwtDuration)
	folderService := service.NewFolderService(db, diskStorage, s3Storage)
	folderHandler := handler.NewFolderHandler(folderService)
	fileService := service.NewFileService(db, diskStorage, s3Storage)
	fileHandler := handler.NewFileHandler(fileService)
	shareService := service.NewShareService(db, diskStorage, s3Storage, folderService)
	shareHandler := handler.NewShareHandler(shareService)
	userService := service.NewUserService(db, jwtManager, diskStorage)
	authHandler := handler.NewAuthHandler(userService)

	e := echo.New()
	e.HideBanner = true

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

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
		usersGroup := api.Group("/users")
		usersGroup.Use(customMiddleware.JWTMiddleware(jwtManager, db))
		{
			usersGroup.GET("/me", authHandler.GetProfile)
			usersGroup.PATCH("/me", authHandler.UpdateProfile)
			usersGroup.PUT("/me/password", authHandler.ChangePassword)
			usersGroup.POST("/logout", authHandler.Logout)
			usersGroup.DELETE("/me", authHandler.DeleteAccount)
		}
		filesGroup := api.Group("/files")
		filesGroup.Use(customMiddleware.JWTMiddleware(jwtManager, db))

		filesGroup.POST("", fileHandler.Upload)
		filesGroup.GET("", fileHandler.List)
		filesGroup.GET("/:id/content", fileHandler.Download)
		filesGroup.PATCH("/:id", fileHandler.Rename)
		filesGroup.PATCH("/:id/move", fileHandler.Move)
		filesGroup.DELETE("/:id", fileHandler.Delete)
		filesGroup.POST("/:id/migrate", fileHandler.Migrate)

		filesGroup.POST("/upload/init", fileHandler.InitChunkUpload)
		filesGroup.POST("/upload/chunk", fileHandler.UploadChunk)
		filesGroup.GET("/upload/:upload_id", fileHandler.GetChunkUploadStatus)
		filesGroup.POST("/upload/complete", fileHandler.CompleteChunkUpload)
		filesGroup.GET("/:id/download", folderHandler.Download)
	}
	foldersGroup := api.Group("/folders")
	foldersGroup.Use(customMiddleware.JWTMiddleware(jwtManager, db))
	{
		foldersGroup.POST("", folderHandler.Create)
		foldersGroup.GET("/contents", folderHandler.GetRootContents)
		foldersGroup.GET("/:id/contents", folderHandler.GetFolderContents)
		foldersGroup.PATCH("/:id", folderHandler.Rename)
		foldersGroup.PATCH("/:id/move", folderHandler.Move)
		foldersGroup.DELETE("/:id", folderHandler.Delete)
	}
	publicSharesGroup := api.Group("/public/shares")
	{
		publicSharesGroup.GET("/:code", shareHandler.GetPublicDetail)
		publicSharesGroup.GET("/:code/download", shareHandler.DownloadPublicFile)

		sharesGroup := api.Group("/shares")
		sharesGroup.Use(customMiddleware.JWTMiddleware(jwtManager, db))
		{
			sharesGroup.POST("", shareHandler.Create)
			sharesGroup.GET("", shareHandler.List)
			sharesGroup.DELETE("/:id", shareHandler.Delete)
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
