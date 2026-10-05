package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/myzjc/SuperDisk/internal/middleware"
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

type CreateFolderReq struct {
	Name     string `json:"name"`
	ParentID uint   `json:"parent_id"`
}

type RenameFolderReq struct {
	Name string `json:"name"`
}

type MoveFolderReq struct {
	TargetParentID uint `json:"target_parent_id"`
}

func (h *FolderHandler) Create(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	var req CreateFolderReq
	if err := c.Bind(&req); err != nil || req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body: 'name' is required")
	}

	resp, err := h.folderService.CreateFolder(userID, req.Name, req.ParentID)
	if err != nil {
		if errors.Is(err, service.ErrFolderAlreadyExists) {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusCreated, resp)
}

func (h *FolderHandler) Rename(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid folder ID")
	}

	var req RenameFolderReq
	if err := c.Bind(&req); err != nil || req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body: 'name' is required")
	}

	resp, err := h.folderService.RenameFolder(userID, uint(id), req.Name)
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

func (h *FolderHandler) Move(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid folder ID")
	}

	var req MoveFolderReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	resp, err := h.folderService.MoveFolder(userID, uint(id), req.TargetParentID)
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

func (h *FolderHandler) Delete(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid folder ID")
	}

	if err := h.folderService.DeleteFolder(userID, uint(id)); err != nil {
		if errors.Is(err, service.ErrFolderNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *FolderHandler) GetRootContents(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	resp, err := h.folderService.GetFolderContents(userID, 0)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *FolderHandler) GetFolderContents(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid folder ID")
	}

	resp, err := h.folderService.GetFolderContents(userID, uint(id))
	if err != nil {
		if errors.Is(err, service.ErrFolderNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}
