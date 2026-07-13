// Package storage 提供统一的存储抽象层
//
// 用途：
//   - 屏蔽媒体文件（视频/封面/头像/图片）落到本地磁盘与七牛云对象存储的差异，
//     调用方仅感知 Put/Delete 两个语义，由 config.StorageConfig.Driver 决定底层实现
//
// 当前实现：
//   - LocalStorage：本地磁盘 + 原子 rename 写入，返回 /uploads/{key} 静态服务路径
//   - QiniuStorage：七牛云对象存储（github.com/qiniu/go-sdk/v7），返回 {scheme}://{domain}/{key}
//
// 设计原则：
//   - Put 必须保证外部观察者要么看到完整文件，要么看不到文件（原子语义）
//   - LocalStorage 通过「写 .tmp 临时文件 → os.Rename」实现，避免半截文件
//   - QiniuStorage 依赖七牛服务端整对象写入的原子性
//   - 删除失败视为非致命错误（文件可能本就不存在），由调用方决定是否记录
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"fake_tiktok/internal/config"

	"github.com/qiniu/go-sdk/v7/auth/qbox"
	qiniu "github.com/qiniu/go-sdk/v7/storage"
)

// Storage 存储抽象接口
//
// 调用方约定：
//   - key 使用正斜杠分隔的相对路径，如 "video/42.mp4" / "cover/42.jpg" / "avatar/xx.jpg"
//   - 实现负责拼接到底层存储介质的物理路径
//   - Put 返回的 url 已是可直接对外暴露的访问 URL
//   - size 为待写入数据的字节数（对象存储上传必需；本地存储可忽略）
type Storage interface {
	// Put 将 r 中的数据写入到 key 对应位置，返回可公开访问的 URL
	// 必须保证原子语义：写入成功后外部观察者只看到完整文件
	Put(ctx context.Context, key string, r io.Reader, size int64) (url string, err error)

	// Delete 删除 key 对应的对象
	// 对象不存在时返回 nil（删除是幂等的）
	Delete(ctx context.Context, key string) error
}

// LocalStorage 本地文件系统存储实现
//
// 字段说明：
//   - baseDir：本地磁盘根目录，对应 config.StorageConfig.BaseDir（如 ./uploads）
//   - publicPrefix：对外暴露的 URL 前缀，对应 Router.StaticFS("/uploads", ...) 的路由
//     Put 返回 {publicPrefix}{key}，前端可直接通过 HTTP 静态服务访问
type LocalStorage struct {
	baseDir      string
	publicPrefix string
}

// Put 将 r 中的数据原子写入 {baseDir}/{key}
//
// 流程：
//  1. 计算目标路径 savePath 与临时路径 tmpPath = savePath + ".tmp"
//  2. os.MkdirAll 创建父目录
//  3. 先写入 tmpPath（io.Copy）
//  4. 写入成功后 os.Rename 原子提交为 savePath
//  5. 任一中间环节失败时，确保删除 tmpPath 残留
//
// size 参数对本地存储无意义，仅为满足接口签名。
func (s *LocalStorage) Put(_ context.Context, key string, r io.Reader, _ int64) (string, error) {
	savePath := filepath.Join(s.baseDir, filepath.FromSlash(key))
	// tmpPath 必须与 savePath 处于同一目录（同一文件系统），
	// 保证 os.Rename 是原子操作（跨文件系统的 rename 不保证原子性）
	tmpPath := savePath + ".tmp"

	// 创建父目录，忽略已存在错误
	if err := os.MkdirAll(filepath.Dir(savePath), 0o755); err != nil {
		return "", fmt.Errorf("storage: mkdir parent failed: %w", err)
	}

	// 始终在退出时尝试清理 tmpPath 残留
	// rename 成功后置 nil，避免误删已提交的文件
	tmpCleaned := false
	defer func() {
		if !tmpCleaned {
			_ = os.Remove(tmpPath)
		}
	}()

	// 写入临时文件
	f, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("storage: create tmp file failed: %w", err)
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("storage: write tmp file failed: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("storage: close tmp file failed: %w", err)
	}

	// 原子提交：rename 后外部观察者只会看到完整文件或不存在
	if err := os.Rename(tmpPath, savePath); err != nil {
		return "", fmt.Errorf("storage: rename tmp to final failed: %w", err)
	}
	tmpCleaned = true

	return s.publicPrefix + key, nil
}

// Delete 删除 {baseDir}/{key}
// 对象不存在视为删除成功（幂等）
func (s *LocalStorage) Delete(_ context.Context, key string) error {
	path := filepath.Join(s.baseDir, filepath.FromSlash(key))
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("storage: remove file failed: %w", err)
	}
	return nil
}

// QiniuStorage 七牛云对象存储实现
//
// 字段说明：
//   - mac：七牛鉴权对象（AccessKey + SecretKey）
//   - bucket：存储空间名
//   - domain：绑定的公开访问域名（不含 scheme）
//   - scheme：访问 URL 协议（http/https）
//   - cfg：七牛上传配置（区域 / 是否走 CDN 上传域名等）
type QiniuStorage struct {
	mac    *qbox.Mac
	bucket string
	domain string
	scheme string
	cfg    *qiniu.Config
}

// Put 上传数据到七牛云 bucket 的 key 位置，返回可公开访问的 URL。
//
// 采用覆盖式上传（PutPolicy.Scope = bucket:key），相同 key 重复上传会覆盖旧对象，
// 保证 video/{id}.mp4 等固定 key 的幂等语义。
func (s *QiniuStorage) Put(ctx context.Context, key string, r io.Reader, size int64) (string, error) {
	// 覆盖式上传：Scope 指定为 bucket:key
	putPolicy := qiniu.PutPolicy{Scope: fmt.Sprintf("%s:%s", s.bucket, key)}
	upToken := putPolicy.UploadToken(s.mac)

	formUploader := qiniu.NewFormUploader(s.cfg)
	ret := qiniu.PutRet{}
	if err := formUploader.Put(ctx, &ret, upToken, key, r, size, nil); err != nil {
		return "", fmt.Errorf("storage: qiniu upload failed: %w", err)
	}

	return fmt.Sprintf("%s://%s/%s", s.scheme, s.domain, strings.TrimPrefix(ret.Key, "/")), nil
}

// Delete 删除七牛云 bucket 中 key 对应的对象；对象不存在视为删除成功（幂等）。
func (s *QiniuStorage) Delete(ctx context.Context, key string) error {
	bucketManager := qiniu.NewBucketManager(s.mac, s.cfg)
	if err := bucketManager.Delete(s.bucket, key); err != nil {
		// 612：对象不存在，幂等视为成功
		if strings.Contains(err.Error(), "no such file or directory") || strings.Contains(err.Error(), "612") {
			return nil
		}
		return fmt.Errorf("storage: qiniu delete failed: %w", err)
	}
	return nil
}

// qiniuZone 根据区域标识返回对应的七牛 Zone。
func qiniuZone(zone string) *qiniu.Zone {
	switch strings.ToLower(zone) {
	case "z0":
		return &qiniu.Zone_z0
	case "z1":
		return &qiniu.Zone_z1
	case "z2":
		return &qiniu.Zone_z2
	case "na0":
		return &qiniu.Zone_na0
	case "as0":
		return &qiniu.Zone_as0
	default:
		// 缺省回退到华南（z2），与默认配置保持一致
		return &qiniu.Zone_z2
	}
}

// NewStorage 根据 config 创建对应的存储实现
//
// 选择逻辑：
//   - driver=local → LocalStorage，publicPrefix 固定为 "/uploads/"，与
//     Router.StaticFS("/uploads", ...) 静态服务路由对齐
//   - driver=qiniu → QiniuStorage，上传到七牛云并返回公开访问 URL
//   - 其他 → 返回错误，由 validateConfig 兜底
func NewStorage(storageCfg *config.StorageConfig, qiniuCfg *config.QiniuConfig) (Storage, error) {
	switch storageCfg.Driver {
	case "local":
		return &LocalStorage{
			baseDir:      storageCfg.BaseDir,
			publicPrefix: "/uploads/",
		}, nil
	case "qiniu":
		scheme := "http"
		if qiniuCfg.UseHTTPS {
			scheme = "https"
		}
		return &QiniuStorage{
			mac:    qbox.NewMac(qiniuCfg.AccessKey, qiniuCfg.SecretKey),
			bucket: qiniuCfg.Bucket,
			domain: strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(qiniuCfg.Domain, "https://"), "http://"), "/"),
			scheme: scheme,
			cfg: &qiniu.Config{
				Zone:          qiniuZone(qiniuCfg.Zone),
				UseHTTPS:      qiniuCfg.UseHTTPS,
				UseCdnDomains: qiniuCfg.UseCdnDomains,
			},
		}, nil
	default:
		return nil, fmt.Errorf("storage: unsupported driver %q, expect local or qiniu", storageCfg.Driver)
	}
}
