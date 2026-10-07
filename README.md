# SuperDisk

基于 Go + Echo 构建的轻量级云盘后端服务，冰岩实习题NetDisk。

## 快速运行

```bash
# 启动服务（默认监听 :8080，使用本地 SQLite 与磁盘存储）
go run cmd/server/main.go
```

## 可选配置 (S3/MinIO)

不配置时默认使用本地存储，如需接入对象存储：

```bash
export S3_ENDPOINT="127.0.0.1:9000"
export S3_BUCKET="superdisk"
export S3_ACCESS_KEY="minioadmin"
export S3_SECRET_KEY="minioadmin"
```
