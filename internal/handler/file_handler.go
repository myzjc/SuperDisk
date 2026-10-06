// Package handler 提供 HTTP 处理程序，用于处理文件相关的请求。
package handler

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/myzjc/SuperDisk/internal/middleware"
	"github.com/myzjc/SuperDisk/internal/service"
)

type FileHandler struct {
	fileService *service.FileService
}

func NewFileHandler(fs *service.FileService) *FileHandler {
	return &FileHandler{
		fileService: fs,
	}
}

// Upload 流式上传文件
func (h *FileHandler) Upload(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	var folderID uint
	if fidStr := c.QueryParam("folder_id"); fidStr != "" {
		parsed, err := strconv.ParseUint(fidStr, 10, 32)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid folder_id parameter")
		}
		folderID = uint(parsed)
	}

	req := c.Request()
	reader, err := req.MultipartReader()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid multipart/form-data request")
	}

	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Failed to read form part: %v", err))
		}

		if part.FormName() == "file" {
			defer part.Close()

			originalFilename := part.FileName()
			if originalFilename == "" {
				return echo.NewHTTPError(http.StatusBadRequest, "File name must not be empty")
			}

			// 传入 userID
			resp, err := h.fileService.Upload(userID, originalFilename, folderID, part)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Upload failed: %v", err))
			}

			return c.JSON(http.StatusCreated, resp)
		}
	}

	return echo.NewHTTPError(http.StatusBadRequest, "Missing 'file' field in form")
}

// List 获取用户特定文件夹下的文件列表
func (h *FileHandler) List(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	var folderID uint
	if fidStr := c.QueryParam("folder_id"); fidStr != "" {
		parsed, err := strconv.ParseUint(fidStr, 10, 32)
		if err == nil {
			folderID = uint(parsed)
		}
	}

	files, err := h.fileService.ListFiles(userID, folderID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Failed to list files: %v", err))
	}
	return c.JSON(http.StatusOK, files)
}

// Download 下载文件内容
func (h *FileHandler) Download(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid file ID")
	}
	result, err := h.fileService.GetFileForDownload(c.Request().Context(), userID, uint(id))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if result.IsRemote {
		return c.Redirect(http.StatusFound, result.PresignedURL)
	}
	defer result.Stream.Close()
	encodedFilename := url.PathEscape(result.File.Filename)
	contentDisposition := fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s", encodedFilename, encodedFilename)
	c.Response().Header().Set(echo.HeaderContentDisposition, contentDisposition)
	c.Response().Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(c.Response(), c.Request(), result.File.Filename, result.File.UpdatedAt, result.Stream)
	return nil
}

type RenameReq struct {
	NewFilename string `json:"filename"`
}

// Rename 重命名文件
func (h *FileHandler) Rename(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid file ID")
	}

	var req RenameReq
	if err := c.Bind(&req); err != nil || req.NewFilename == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body: 'filename' is required")
	}

	updated, err := h.fileService.Rename(userID, uint(id), req.NewFilename)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, updated)
}

type MoveFileReq struct {
	TargetFolderID uint `json:"target_folder_id"`
}

// Move 移动文件
func (h *FileHandler) Move(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid file ID")
	}

	var req MoveFileReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	updated, err := h.fileService.MoveFile(userID, uint(id), req.TargetFolderID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, updated)
}

// Delete 删除文件
func (h *FileHandler) Delete(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid file ID")
	}

	if err := h.fileService.Delete(userID, uint(id)); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

// Migrate 将文件挪动到对象存储中
// POST /api/v1/files/:id/migrate
func (h *FileHandler) Migrate(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid file ID")
	}

	if err := h.fileService.MigrateFile(c.Request().Context(), userID, uint(id)); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, echo.Map{"message": "file migrated to object storage successfully"})
}
