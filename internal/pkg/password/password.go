// Package password 用于生成hash和加密
package password

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword 对明文密码进行 bcrypt
func HashPassword(plainPassword string) (string, error) {
	if len(plainPassword) == 0 {
		return "", errors.New("password cannot be empty")
	}

	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(plainPassword), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	return string(hashedBytes), nil
}

func CheckPassword(plainPassword, hashedPassword string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(plainPassword))
}
