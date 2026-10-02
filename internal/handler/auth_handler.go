package handler

import (
	"errors"
	"net/http"

	"github.com/myzjc/SuperDisk/internal/service"

	"github.com/labstack/echo/v4"
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

// Register 用户注册
// POST /api/v1/auth/register
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
// POST /api/v1/auth/login
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
