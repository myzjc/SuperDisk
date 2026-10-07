package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Config 对象存储配置
type S3Config struct {
	Endpoint  string //
	Bucket    string // 存储桶名称，如 "superdisk"
	AccessKey string // 访问密钥 AK
	SecretKey string // 秘密密钥 SK
	UseSSL    bool   // 是否使用 HTTPS
}

// S3Storage 封装 MinIO 客户端
type S3Storage struct {
	config S3Config
	client *minio.Client
}

// NewS3Storage 初始化 MinIO 客户端并自动创建 Bucket
func NewS3Storage(cfg S3Config) (*S3Storage, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize MinIO client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err == nil && !exists {
		_ = client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{})
	}

	return &S3Storage{
		config: cfg,
		client: client,
	}, nil
}

// Upload 流式上传文件到对象存储
func (s *S3Storage) Upload(ctx context.Context, objectKey string, reader io.Reader, size int64, contentType string) error {
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	_, err := s.client.PutObject(ctx, s.config.Bucket, objectKey, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("failed to upload object to S3: %w", err)
	}
	return nil
}

// Open 获取 S3 对象的流式读取句柄（MinIO 的 GetObject 原生返回可读流）
func (s *S3Storage) Open(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.config.Bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to open S3 object: %w", err)
	}
	return obj, nil
}

// GetPresignedURL 生成用于浏览器直接下载的预签名直链
func (s *S3Storage) GetPresignedURL(ctx context.Context, objectKey string, downloadFilename string, expiry time.Duration) (string, error) {
	reqParams := make(url.Values)
	if downloadFilename != "" {
		encodedFilename := url.PathEscape(downloadFilename)
		contentDisposition := fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s", encodedFilename, encodedFilename)
		reqParams.Set("response-content-disposition", contentDisposition)
	}

	presignedURL, err := s.client.PresignedGetObject(ctx, s.config.Bucket, objectKey, expiry, reqParams)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return presignedURL.String(), nil
}

// Delete 从对象存储中删除文件
func (s *S3Storage) Delete(ctx context.Context, objectKey string) error {
	err := s.client.RemoveObject(ctx, s.config.Bucket, objectKey, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete object from S3: %w", err)
	}
	return nil
}
