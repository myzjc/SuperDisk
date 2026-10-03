package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/myzjc/SuperDisk/internal/service"
)

type FolderHandler struct {
	folderService *service.FolderService
}

func NewFolderHandler(fs *service.FolderService) *FolderHandler {
	return &FolderHandler{
		folderService: fs,
	}
}

// CreateFolderReq 新建文件夹请求体
type CreateFolderReq struct {
	Name     string `json:"name"`
	ParentID uint   `json:"parent_id"`
}

// RenameFolderReq 重命名文件夹请求体
type RenameFolderReq struct {
	Name string `json:"name"`
}

// MoveFolderReq 移动文件夹请求体
type MoveFolderReq struct {
	TargetParentID uint `json:"target_parent_id"`
}

// Create 新建文件夹
// POST /api/v1/folders
func (h *FolderHandler) Create(c echo.Context) error {
	var req CreateFolderReq
	if err := c.Bind(&req); err != nil || req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body: 'name' is required")
	}

	resp, err := h.folderService.CreateFolder(req.Name, req.ParentID)
	if err != nil {
		if errors.Is(err, service.ErrFolderAlreadyExists) {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusCreated, resp)
}

// Rename 重命名文件夹
// PATCH /api/v1/folders/:id
func (h *FolderHandler) Rename(c echo.Context) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid folder ID")
	}

	var req RenameFolderReq
	if err := c.Bind(&req); err != nil || req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body: 'name' is required")
	}

	resp, err := h.folderService.RenameFolder(uint(id), req.Name)
	if err != nil {
		if errors.Is(err, service.ErrFolderNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		if errors.Is(err, service.ErrFolderAlreadyExists) {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

// Move 移动文件夹
// PATCH /api/v1/folders/:id/move
func (h *FolderHandler) Move(c echo.Context) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid folder ID")
	}

	var req MoveFolderReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	resp, err := h.folderService.MoveFolder(uint(id), req.TargetParentID)
	if err != nil {
		if errors.Is(err, service.ErrCannotMoveToChild) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		if errors.Is(err, service.ErrFolderNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		if errors.Is(err, service.ErrFolderAlreadyExists) {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

// Delete 递归删除文件夹
// DELETE /api/v1/folders/:id
func (h *FolderHandler) Delete(c echo.Context) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid folder ID")
	}

	if err := h.folderService.DeleteFolder(uint(id)); err != nil {
		if errors.Is(err, service.ErrFolderNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

// GetRootContents 获取根目录内容
// GET /api/v1/folders/contents
func (h *FolderHandler) GetRootContents(c echo.Context) error {
	resp, err := h.folderService.GetFolderContents(0)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// GetFolderContents 获取指定目录内容
// GET /api/v1/folders/:id/contents
func (h *FolderHandler) GetFolderContents(c echo.Context) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid folder ID")
	}

	resp, err := h.folderService.GetFolderContents(uint(id))
	if err != nil {
		if errors.Is(err, service.ErrFolderNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}
