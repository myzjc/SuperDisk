package handler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/myzjc/SuperDisk/internal/middleware"
	"github.com/myzjc/SuperDisk/internal/service"
)

type AuthHandler struct {
	userService *service.UserService
}

func NewAuthHandler(us *service.UserService) *AuthHandler {
	return &AuthHandler{
		userService: us,
	}
}

// AuthRequest 注册与登录通用的请求体
type AuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AuthResponse 登录成功后的响应体
type AuthResponse struct {
	Token    string `json:"token"`
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
}

// UpdateProfileReq 修改个人信息请求体
type UpdateProfileReq struct {
	Nickname string `json:"nickname"`
	Email    string `json:"email"`
	Bio      string `json:"bio"`
}

// ChangePasswordReq 修改密码请求体
type ChangePasswordReq struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// Register 用户注册
func (h *AuthHandler) Register(c echo.Context) error {
	var req AuthRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	user, err := h.userService.Register(req.Username, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrUserAlreadyExists) {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusCreated, echo.Map{
		"id":       user.ID,
		"username": user.Username,
		"message":  "registered successfully",
	})
}

// Login 用户登录
func (h *AuthHandler) Login(c echo.Context) error {
	var req AuthRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	token, user, err := h.userService.Login(req.Username, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			return echo.NewHTTPError(http.StatusUnauthorized, "Invalid username or password")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "Internal login error")
	}

	return c.JSON(http.StatusOK, AuthResponse{
		Token:    token,
		UserID:   user.ID,
		Username: user.Username,
	})
}

// GetProfile 获取当前登录用户的个人资料
// GET /api/v1/users/me
func (h *AuthHandler) GetProfile(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	profile, err := h.userService.GetProfile(userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	return c.JSON(http.StatusOK, profile)
}

// UpdateProfile 修改个人信息
// PATCH /api/v1/users/me
func (h *AuthHandler) UpdateProfile(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	var req UpdateProfileReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	updated, err := h.userService.UpdateProfile(userID, req.Nickname, req.Email, req.Bio)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, updated)
}

// ChangePassword 修改密码
// PUT /api/v1/users/me/password
func (h *AuthHandler) ChangePassword(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	var req ChangePasswordReq
	if err := c.Bind(&req); err != nil || req.OldPassword == "" || req.NewPassword == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body: both old_password and new_password required")
	}

	if err := h.userService.ChangePassword(userID, req.OldPassword, req.NewPassword); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, echo.Map{"message": "password updated successfully, please log in again"})
}

// Logout 用户登出（使当前 Token 作废）
// POST /api/v1/users/logout
func (h *AuthHandler) Logout(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	if err := h.userService.Logout(userID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, echo.Map{"message": "logged out successfully"})
}

// DeleteAccount 注销账户
// DELETE /api/v1/users/me
func (h *AuthHandler) DeleteAccount(c echo.Context) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	if err := h.userService.DeleteAccount(userID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}
