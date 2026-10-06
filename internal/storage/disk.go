// Package storage 提供文件存储功能
package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Storage interface {
	Save(src io.Reader) (relPath string, size int64, hash string, err error)
	Open(relPath string) (io.ReadSeekCloser, error)
	Delete(relPath string) error
}

type DiskStorage struct {
	baseDir string
}

func NewDiskStorage(baseDir string) (*DiskStorage, error) {
	cleanDir := filepath.Clean(baseDir)
	if err := os.MkdirAll(cleanDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create base storage directory: %w", err)
	}

	return &DiskStorage{
		baseDir: cleanDir,
	}, nil
}

func (s *DiskStorage) Save(src io.Reader) (string, int64, string, error) {
	now := time.Now()
	subDir := fmt.Sprintf("%d/%02d", now.Year(), now.Month())
	filename := fmt.Sprintf("%s.dat", uuid.New().String())

	relPath := filepath.Join(subDir, filename)

	fullSubDir := filepath.Join(s.baseDir, subDir)
	if err := os.MkdirAll(fullSubDir, 0o755); err != nil {
		return "", 0, "", fmt.Errorf("failed to create subdir: %w", err)
	}

	fullPath := filepath.Join(s.baseDir, relPath)

	dstFile, err := os.Create(fullPath)
	if err != nil {
		return "", 0, "", fmt.Errorf("failed to create target file: %w", err)
	}

	var success bool
	defer func() {
		_ = dstFile.Close()
		if !success {
			_ = os.Remove(fullPath)
		}
	}()

	hasher := sha256.New()
	multiWriter := io.MultiWriter(dstFile, hasher)

	written, err := io.Copy(multiWriter, src)
	if err != nil {
		return "", 0, "", fmt.Errorf("failed to write stream to disk: %w", err)
	}

	success = true
	hashHex := hex.EncodeToString(hasher.Sum(nil))

	return relPath, written, hashHex, nil
}

// Open 打开本地物理文件并返回支持 Seek 寻址的句柄
func (s *DiskStorage) Open(relPath string) (io.ReadSeekCloser, error) {
	fullPath, err := s.resolveSafePath(relPath)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("file not found on disk")
		}
		return nil, err
	}
	return file, nil
}

// Delete 删除本地物理文件
func (s *DiskStorage) Delete(relPath string) error {
	fullPath, err := s.resolveSafePath(relPath)
	if err != nil {
		return err
	}

	err = os.Remove(fullPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove disk file: %w", err)
	}

	return nil
}

// resolveSafePath 解析安全路径
func (s *DiskStorage) resolveSafePath(relPath string) (string, error) {
	cleanRel := filepath.Clean(relPath)
	fullPath := filepath.Join(s.baseDir, cleanRel)

	absBase, err := filepath.Abs(s.baseDir)
	if err != nil {
		return "", err
	}
	absTarget, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}

	if !strings.HasPrefix(absTarget, absBase) {
		return "", errors.New("illegal file path: path traversal detected")
	}

	return fullPath, nil
}
