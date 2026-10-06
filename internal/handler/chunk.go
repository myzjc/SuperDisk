package handler

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/myzjc/SuperDisk/internal/middleware"
)

// InitChunkUploadReq 初始化分片上传请求体
type InitChunkUploadReq struct {
	Filename  string `json:"filename"`
	FolderID  uint   `json:"folder_id"`
	TotalSize int64  `json:"total_size"`
	ChunkSize int64  `json:"chunk_size"`
}

// CompleteChunkUploadReq 完成分片合并请求体
type CompleteChunkUploadReq struct {
	UploadID string `json:"upload_id"`
}

// InitChunkUpload 初始化大文件分片上传会话
// POST /api/v1/files/upload/init
func (h *FileHandler) InitChunkUpload(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	var req InitChunkUploadReq
	if err := c.Bind(&req); err != nil || req.Filename == "" || req.TotalSize <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body: 'filename' and valid 'total_size' required")
	}

	session, err := h.fileService.InitChunkUpload(userID, req.Filename, req.FolderID, req.TotalSize, req.ChunkSize)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusCreated, session)
}

// UploadChunk 上传单个分片
// POST /api/v1/files/upload/chunk
func (h *FileHandler) UploadChunk(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	req := c.Request()
	reader, err := req.MultipartReader()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid multipart/form-data request")
	}

	var uploadID string
	var chunkIndex int = -1
	var chunkUploaded bool

	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Failed to read part: %v", err))
		}

		formName := part.FormName()
		if formName == "upload_id" {
			data, _ := io.ReadAll(part)
			uploadID = string(data)
			_ = part.Close()
		} else if formName == "chunk_index" {
			data, _ := io.ReadAll(part)
			idx, err := strconv.Atoi(string(data))
			if err == nil {
				chunkIndex = idx
			}
			_ = part.Close()
		} else if formName == "file" {
			defer part.Close()

			if uploadID == "" {
				uploadID = c.QueryParam("upload_id")
			}
			if chunkIndex < 0 {
				if idx, err := strconv.Atoi(c.QueryParam("chunk_index")); err == nil {
					chunkIndex = idx
				}
			}

			if uploadID == "" || chunkIndex < 0 {
				return echo.NewHTTPError(http.StatusBadRequest, "Missing upload_id or chunk_index before file part (can pass via query params)")
			}

			if err := h.fileService.UploadChunk(userID, uploadID, chunkIndex, part); err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			chunkUploaded = true
			break
		}
	}

	if !chunkUploaded {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing 'file' binary part in form")
	}

	return c.JSON(http.StatusOK, echo.Map{
		"upload_id":   uploadID,
		"chunk_index": chunkIndex,
		"status":      "uploaded",
	})
}

// GetChunkUploadStatus 查询已上传的分片索引
// GET /api/v1/files/upload/:upload_id
func (h *FileHandler) GetChunkUploadStatus(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	uploadID := c.Param("upload_id")
	if uploadID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing upload_id")
	}

	status, err := h.fileService.GetChunkUploadStatus(userID, uploadID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	return c.JSON(http.StatusOK, status)
}

// CompleteChunkUpload 合并切片，完成上传
// POST /api/v1/files/upload/complete
func (h *FileHandler) CompleteChunkUpload(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	var req CompleteChunkUploadReq
	if err := c.Bind(&req); err != nil || req.UploadID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body: 'upload_id' required")
	}

	fileResp, err := h.fileService.CompleteChunkUpload(userID, req.UploadID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusCreated, fileResp)
}
