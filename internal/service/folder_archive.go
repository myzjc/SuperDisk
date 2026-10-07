package service

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/myzjc/SuperDisk/internal/model"
)

// 支持的压缩格式常量
const (
	FormatZip   = "zip"
	FormatTarGz = "tar.gz"
	Format7z    = "7z"
)

// FolderArchiveResult 封装打包结果
type FolderArchiveResult struct {
	ArchiveName string
	File        io.ReadSeekCloser
	UpdatedAt   time.Time
	ContentType string
}

// DownloadFolderArchive 支持 .zip / .tar.gz / .7z 格式的文件夹打包下载（带 Range 断点续传）
func (s *FolderService) DownloadFolderArchive(ctx context.Context, userID uint, folderID uint, format string) (*FolderArchiveResult, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = FormatZip
	}
	if format != FormatZip && format != FormatTarGz && format != Format7z {
		return nil, fmt.Errorf("unsupported format '%s': must be zip, tar.gz, or 7z", format)
	}

	var rootFolder model.Folder
	if err := s.db.Where("id = ? AND user_id = ?", folderID, userID).First(&rootFolder).Error; err != nil {
		return nil, ErrFolderNotFound
	}

	tempDir := "./data/temp_archives"
	_ = os.MkdirAll(tempDir, 0o755)

	var ext, contentType string
	switch format {
	case FormatZip:
		ext = ".zip"
		contentType = "application/zip"
	case FormatTarGz:
		ext = ".tar.gz"
		contentType = "application/gzip"
	case Format7z:
		ext = ".7z"
		contentType = "application/x-7z-compressed"
	}

	archiveFileName := fmt.Sprintf("%d_%d%s", rootFolder.ID, rootFolder.UpdatedAt.UnixNano(), ext)
	archiveFilePath := filepath.Join(tempDir, archiveFileName)
	archiveName := fmt.Sprintf("%s%s", rootFolder.Name, ext)

	if file, err := os.Open(archiveFilePath); err == nil {
		return &FolderArchiveResult{
			ArchiveName: archiveName,
			File:        file,
			UpdatedAt:   rootFolder.UpdatedAt,
			ContentType: contentType,
		}, nil
	}

	folderPathMap := make(map[uint]string)
	folderPathMap[rootFolder.ID] = rootFolder.Name
	queue := []uint{rootFolder.ID}
	allFolderIDs := []uint{rootFolder.ID}

	for len(queue) > 0 {
		currID := queue[0]
		queue = queue[1:]
		currPath := folderPathMap[currID]

		var subFolders []model.Folder
		if err := s.db.Where("user_id = ? AND parent_id = ?", userID, currID).Find(&subFolders).Error; err == nil {
			for _, sub := range subFolders {
				subPath := filepath.Join(currPath, sub.Name)
				folderPathMap[sub.ID] = subPath
				allFolderIDs = append(allFolderIDs, sub.ID)
				queue = append(queue, sub.ID)
			}
		}
	}

	var files []model.File
	if err := s.db.Preload("Blob").Where("user_id = ? AND folder_id IN ?", userID, allFolderIDs).Find(&files).Error; err != nil {
		return nil, err
	}

	getFileStream := func(f *model.File) (io.ReadCloser, error) {
		if f.Blob.StorageType == model.StorageTypeS3 && s.s3Storage != nil {
			return s.s3Storage.Open(ctx, f.Blob.StorageName)
		}
		return s.storage.Open(f.Blob.StorageName)
	}

	var err error
	switch format {
	case FormatZip:
		err = s.buildZip(archiveFilePath, files, folderPathMap, getFileStream)
	case FormatTarGz:
		err = s.buildTarGz(archiveFilePath, files, folderPathMap, getFileStream)
	case Format7z:
		err = s.build7z(ctx, archiveFilePath, files, folderPathMap, getFileStream)
	}
	if err != nil {
		_ = os.Remove(archiveFilePath)
		return nil, fmt.Errorf("archive creation failed: %w", err)
	}

	resultFile, err := os.Open(archiveFilePath)
	if err != nil {
		return nil, err
	}

	return &FolderArchiveResult{
		ArchiveName: archiveName,
		File:        resultFile,
		UpdatedAt:   rootFolder.UpdatedAt,
		ContentType: contentType,
	}, nil
}

func (s *FolderService) buildZip(targetPath string, files []model.File, pathMap map[uint]string, getStream func(*model.File) (io.ReadCloser, error)) error {
	f, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	for _, file := range files {
		entryPath := filepath.Join(pathMap[file.FolderID], file.Filename)
		w, err := zw.Create(entryPath)
		if err != nil {
			return err
		}

		stream, err := getStream(&file)
		if err != nil {
			continue
		}
		_, _ = io.Copy(w, stream)
		_ = stream.Close()
	}
	return nil
}

func (s *FolderService) buildTarGz(targetPath string, files []model.File, pathMap map[uint]string, getStream func(*model.File) (io.ReadCloser, error)) error {
	f, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	for _, file := range files {
		entryPath := filepath.Join(pathMap[file.FolderID], file.Filename)
		hdr := &tar.Header{
			Name:    entryPath,
			Mode:    0o644,
			Size:    file.Blob.FileSize,
			ModTime: file.UpdatedAt,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}

		stream, err := getStream(&file)
		if err != nil {
			continue
		}
		_, _ = io.Copy(tw, stream)
		_ = stream.Close()
	}
	return nil
}

func (s *FolderService) build7z(ctx context.Context, targetPath string, files []model.File, pathMap map[uint]string, getStream func(*model.File) (io.ReadCloser, error)) error {
	stagingDir, err := os.MkdirTemp("", "7z_stage_*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stagingDir)

	for _, file := range files {
		relPath := filepath.Join(pathMap[file.FolderID], file.Filename)
		fullPath := filepath.Join(stagingDir, relPath)
		_ = os.MkdirAll(filepath.Dir(fullPath), 0o755)

		dst, err := os.Create(fullPath)
		if err != nil {
			continue
		}
		stream, err := getStream(&file)
		if err != nil {
			dst.Close()
			continue
		}
		_, _ = io.Copy(dst, stream)
		dst.Close()
		stream.Close()
	}

	absTarget, _ := filepath.Abs(targetPath)
	cmd := exec.CommandContext(ctx, "7z", "a", "-t7z", "-mx=5", absTarget, ".")
	cmd.Dir = stagingDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("7z error: %w, output: %s", err, string(output))
	}
	return nil
}
