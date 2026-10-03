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

	"github.com/myzjc/SuperDisk/internal/service"
)

// FileHandler 处理文件相关的 HTTP 请求
type FileHandler struct {
	fileService *service.FileService
}

// NewFileHandler 构造函数
func NewFileHandler(fs *service.FileService) *FileHandler {
	return &FileHandler{
		fileService: fs,
	}
}

// Upload 流式上传文件
// POST /api/v1/files
func (h *FileHandler) Upload(c echo.Context) error {
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

			resp, err := h.fileService.Upload(originalFilename, folderID, part)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Upload failed: %v", err))
			}

			return c.JSON(http.StatusCreated, resp)
		}
	}

	return echo.NewHTTPError(http.StatusBadRequest, "Missing 'file' field in form")
}

// List 获取已上传的文件列表
// GET /api/v1/files
func (h *FileHandler) List(c echo.Context) error {
	files, err := h.fileService.ListFiles()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Failed to list files: %v", err))
	}
	return c.JSON(http.StatusOK, files)
}

// Download 下载文件内容
// GET /api/v1/files/:id/content
func (h *FileHandler) Download(c echo.Context) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid file ID")
	}

	fileRecord, stream, err := h.fileService.GetFileForDownload(uint(id))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	defer stream.Close()

	encodedFilename := url.PathEscape(fileRecord.Filename)
	contentDisposition := fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s", encodedFilename, encodedFilename)
	c.Response().Header().Set(echo.HeaderContentDisposition, contentDisposition)

	if fileRecord.FileSize > 0 {
		c.Response().Header().Set(echo.HeaderContentLength, strconv.FormatInt(fileRecord.FileSize, 10))
	}

	return c.Stream(http.StatusOK, fileRecord.ContentType, stream)
}

// RenameReq 重命名请求体
type RenameReq struct {
	NewFilename string `json:"filename"`
}

// Rename 重命名文件
// PATCH /api/v1/files/:id
func (h *FileHandler) Rename(c echo.Context) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid file ID")
	}

	var req RenameReq
	if err := c.Bind(&req); err != nil || req.NewFilename == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body: 'filename' is required")
	}

	updated, err := h.fileService.Rename(uint(id), req.NewFilename)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, updated)
}

// Delete 删除文件
// DELETE /api/v1/files/:id
func (h *FileHandler) Delete(c echo.Context) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid file ID")
	}

	if err := h.fileService.Delete(uint(id)); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

// MoveFileReq 文件移动请求体
type MoveFileReq struct {
	TargetFolderID uint `json:"target_folder_id"` // 目标文件夹 ID
}

// Move 移动文件到目标文件夹
// PATCH /api/v1/files/:id/move
func (h *FileHandler) Move(c echo.Context) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid file ID")
	}
	var req MoveFileReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}
	updated, err := h.fileService.MoveFile(uint(id), req.TargetFolderID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, updated)
}
