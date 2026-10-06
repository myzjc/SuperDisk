package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
)

// SaveChunk 保存单个切片到临时目录 data/chunks/{uploadID}/{index}.part
func (s *DiskStorage) SaveChunk(uploadID string, chunkIndex int, src io.Reader) error {
	sessionDir := filepath.Join(s.chunksDir, uploadID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return fmt.Errorf("failed to create session chunks dir: %w", err)
	}

	chunkPath := filepath.Join(sessionDir, fmt.Sprintf("%d.part", chunkIndex))
	file, err := os.Create(chunkPath)
	if err != nil {
		return fmt.Errorf("failed to create chunk file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, src); err != nil {
		_ = os.Remove(chunkPath)
		return fmt.Errorf("failed to write chunk data: %w", err)
	}

	return nil
}

// GetUploadedChunks 扫描目录，获取当前会话已成功落盘的所有分片索引（用于断点查询）
func (s *DiskStorage) GetUploadedChunks(uploadID string) ([]int, error) {
	sessionDir := filepath.Join(s.chunksDir, uploadID)
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []int{}, nil
		}
		return nil, err
	}

	var indexes []int
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		var idx int
		if _, err := fmt.Sscanf(entry.Name(), "%d.part", &idx); err == nil {
			indexes = append(indexes, idx)
		}
	}
	sort.Ints(indexes)
	return indexes, nil
}

// MergeChunks 按顺序流式合并所有切片，计算 SHA-256，并在完成后自动清理临时切片
func (s *DiskStorage) MergeChunks(uploadID string, totalChunks int) (string, int64, string, error) {
	now := time.Now()
	subDir := fmt.Sprintf("%d/%02d", now.Year(), now.Month())
	filename := fmt.Sprintf("%s.dat", uuid.New().String())

	relPath := filepath.Join(subDir, filename)
	fullSubDir := filepath.Join(s.baseDir, subDir)
	if err := os.MkdirAll(fullSubDir, 0o755); err != nil {
		return "", 0, "", fmt.Errorf("failed to create target subdir: %w", err)
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
	var totalWritten int64

	sessionDir := filepath.Join(s.chunksDir, uploadID)

	for i := 0; i < totalChunks; i++ {
		chunkPath := filepath.Join(sessionDir, fmt.Sprintf("%d.part", i))
		chunkFile, err := os.Open(chunkPath)
		if err != nil {
			return "", 0, "", fmt.Errorf("missing chunk %d: %w", i, err)
		}

		n, err := io.Copy(multiWriter, chunkFile)
		_ = chunkFile.Close()
		if err != nil {
			return "", 0, "", fmt.Errorf("failed to merge chunk %d: %w", i, err)
		}
		totalWritten += n
	}

	success = true
	hashHex := hex.EncodeToString(hasher.Sum(nil))

	_ = os.RemoveAll(sessionDir)

	return relPath, totalWritten, hashHex, nil
}
