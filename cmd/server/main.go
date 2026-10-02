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

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/myzjc/SuperDisk/internal/handler"
	"github.com/myzjc/SuperDisk/internal/repository"
	"github.com/myzjc/SuperDisk/internal/service"
	"github.com/myzjc/SuperDisk/internal/storage"
)

const (
	defaultPort       = ":8080"
	defaultDBPath     = "netdisk.db"
	defaultStorageDir = "./data/uploads"
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

	fileService := service.NewFileService(db, diskStorage)
	fileHandler := handler.NewFileHandler(fileService)

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
		files := api.Group("/files")
		files.POST("", fileHandler.Upload)
		files.GET("", fileHandler.List)
		files.GET("/:id/content", fileHandler.Download)
		files.PATCH("/:id", fileHandler.Rename)
		files.DELETE("/:id", fileHandler.Delete)
	}

	go func() {
		log.Printf("[Server] NetDisk is running at http://localhost%s\n", defaultPort)
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
