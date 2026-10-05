// Package middleware 提供 Echo 中间件，包括 JWT 身份验证与上下文处理。
package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"github.com/myzjc/SuperDisk/internal/model"
	"github.com/myzjc/SuperDisk/internal/pkg/jwt"
)

const (
	ContextKeyUserID   = "userID"
	ContextKeyUsername = "username"
)

// JWTMiddleware 创建一个 JWT 身份验证中间件
func JWTMiddleware(jwtManager *jwt.JWTManager, db *gorm.DB) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			tokenStr := extractToken(c)
			if tokenStr == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "Missing authorization token")
			}

			claims, err := jwtManager.ParseToken(tokenStr)
			if err != nil {
				if errors.Is(err, jwt.ErrTokenExpired) {
					return echo.NewHTTPError(http.StatusUnauthorized, "Token has expired, please log in again")
				}
				return echo.NewHTTPError(http.StatusUnauthorized, "Invalid token")
			}

			var user model.User
			if err := db.Select("token_version").First(&user, claims.UserID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return echo.NewHTTPError(http.StatusUnauthorized, "User account no longer exists")
				}
				return echo.NewHTTPError(http.StatusInternalServerError, "Failed to verify authentication status")
			}

			if claims.TokenVersion < user.TokenVersion {
				return echo.NewHTTPError(http.StatusUnauthorized, "Token has been invalidated (logged out or password changed)")
			}

			c.Set(ContextKeyUserID, claims.UserID)
			c.Set(ContextKeyUsername, claims.Username)

			return next(c)
		}
	}
}

// extractToken 提取 Token，支持 Bearer Header 与 Query 参数
func extractToken(c echo.Context) string {
	authHeader := c.Request().Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
	}

	if tokenParam := c.QueryParam("token"); tokenParam != "" {
		return strings.TrimSpace(tokenParam)
	}

	return ""
}

// GetUserID 安全地从上下文中获取已认证的用户 ID
func GetUserID(c echo.Context) (uint, error) {
	val := c.Get(ContextKeyUserID)
	if val == nil {
		return 0, errors.New("user ID not found in context")
	}

	userID, ok := val.(uint)
	if !ok {
		return 0, errors.New("invalid user ID type in context")
	}

	return userID, nil
}
