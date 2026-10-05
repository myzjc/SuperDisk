package handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/myzjc/SuperDisk/internal/middleware"
	"github.com/myzjc/SuperDisk/internal/service"
)

type ShareHandler struct {
	shareService *service.ShareService
}

func NewShareHandler(ss *service.ShareService) *ShareHandler {
	return &ShareHandler{
		shareService: ss,
	}
}

// CreateShareReq 创建分享请求体
type CreateShareReq struct {
	TargetType string `json:"target_type"` // "file" 或 "folder"
	TargetID   uint   `json:"target_id"`   // 目标 ID
}

// Create 创建分享链接（需登录）
// POST /api/v1/shares
func (h *ShareHandler) Create(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	var req CreateShareReq
	if err := c.Bind(&req); err != nil || req.TargetType == "" || req.TargetID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body: 'target_type' and 'target_id' are required")
	}

	resp, err := h.shareService.CreateShare(userID, req.TargetType, req.TargetID)
	if err != nil {
		if errors.Is(err, service.ErrInvalidShareTarget) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusCreated, resp)
}

// List 列出当前登录用户的分享链接（需登录）
// GET /api/v1/shares
func (h *ShareHandler) List(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	shares, err := h.shareService.ListShares(userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Failed to list shares: %v", err))
	}
	return c.JSON(http.StatusOK, shares)
}

// Delete 删除分享链接（需登录）
// DELETE /api/v1/shares/:id
func (h *ShareHandler) Delete(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid share ID")
	}

	if err := h.shareService.DeleteShare(userID, uint(id)); err != nil {
		if errors.Is(err, service.ErrShareNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

// GetPublicDetail 免登录公开查看分享详情（文件信息或文件夹内容）
// GET /api/v1/public/shares/:code
func (h *ShareHandler) GetPublicDetail(c echo.Context) error {
	code := c.Param("code")
	if code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing share code")
	}

	detail, err := h.shareService.GetPublicShareDetail(code)
	if err != nil {
		if errors.Is(err, service.ErrShareNotFound) || errors.Is(err, service.ErrShareTargetNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, detail)
}

// DownloadPublicFile 免登录流式下载分享的文件
// GET /api/v1/public/shares/:code/download
func (h *ShareHandler) DownloadPublicFile(c echo.Context) error {
	code := c.Param("code")
	if code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing share code")
	}

	fileRecord, stream, err := h.shareService.DownloadSharedFile(code)
	if err != nil {
		if errors.Is(err, service.ErrShareNotFound) || errors.Is(err, service.ErrShareTargetNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	defer stream.Close()

	// 编码文件名并配置 Content-Disposition，从 Blob 读取实际物理大小和类型
	encodedFilename := url.PathEscape(fileRecord.Filename)
	contentDisposition := fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s", encodedFilename, encodedFilename)
	c.Response().Header().Set(echo.HeaderContentDisposition, contentDisposition)

	if fileRecord.Blob.FileSize > 0 {
		c.Response().Header().Set(echo.HeaderContentLength, strconv.FormatInt(fileRecord.Blob.FileSize, 10))
	}

	return c.Stream(http.StatusOK, fileRecord.Blob.ContentType, stream)
}
