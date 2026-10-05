// Package jwt 提供 JWT 的签发与解析功能。
package jwt

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrTokenExpired = errors.New("token has expired")
	ErrTokenInvalid = errors.New("invalid token")
)

// CustomClaims 自定义载荷结构体
type CustomClaims struct {
	UserID       uint   `json:"user_id"`
	Username     string `json:"username"`
	TokenVersion uint   `json:"token_version"`
	jwt.RegisteredClaims
}

// JWTManager 负责 Token 的签发与解析
type JWTManager struct {
	secretKey     []byte        // 签名密钥
	tokenDuration time.Duration // 有效期
	issuer        string        // 签发者名称
}

// NewJWTManager 构造函数
func NewJWTManager(secretKey string, tokenDuration time.Duration) *JWTManager {
	return &JWTManager{
		secretKey:     []byte(secretKey),
		tokenDuration: tokenDuration,
		issuer:        "netdisk-auth",
	}
}

// GenerateToken 为指定用户签发一个 JWT 字符串
func (m *JWTManager) GenerateToken(userID uint, username string, tokenVersion uint) (string, error) {
	now := time.Now()
	expiresAt := now.Add(m.tokenDuration)
	claims := CustomClaims{
		UserID:       userID,
		Username:     username,
		TokenVersion: tokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    m.issuer,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secretKey)
}

// ParseToken 解析并校验 Token 字符串，返回用户 Claims
func (m *JWTManager) ParseToken(tokenStr string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return m.secretKey, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrTokenInvalid
	}

	claims, ok := token.Claims.(*CustomClaims)
	if !ok || !token.Valid {
		return nil, ErrTokenInvalid
	}

	return claims, nil
}
