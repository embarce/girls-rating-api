package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math/rand"
	"path/filepath"
	"strings"
	"time"

	"girls-rating-api/internal/models"
	"girls-rating-api/internal/repository"
	"girls-rating-api/pkg/r2"
	rediscache "girls-rating-api/pkg/redis"
	"girls-rating-api/pkg/snowflake"
)

// UploadService 图片上传服务
type UploadService struct {
	r2Client   *r2.Client
	repo       *repository.ImageResourceRepository
	rdb        *rediscache.Client
	poolSetKey string
	s3Host     string
}

// NewUploadService 创建上传服务
func NewUploadService(r2Client *r2.Client, repo *repository.ImageResourceRepository, rdb *rediscache.Client, poolSetKey string, s3Host string) *UploadService {
	return &UploadService{
		r2Client:   r2Client,
		repo:       repo,
		rdb:        rdb,
		poolSetKey: poolSetKey,
		s3Host:     s3Host,
	}
}

// UploadInput 上传输入
type UploadInput struct {
	File        io.Reader
	Filename    string
	ContentType string
	Size        int64
	Width       int     // 可选，0 表示自动检测
	Height      int     // 可选，0 表示自动检测
	Rating      float64 // 可选，0 表示使用默认值 4.0
	Views       string  // 可选，空表示使用默认值 "1k"
	AuthorID    int64   // 可选，0 表示无作者
}

// Upload 上传图片到 R2 并保存数据库记录
func (s *UploadService) Upload(ctx context.Context, input UploadInput) (*models.ImageResource, error) {
	// 读取文件内容（需要同时用于尺寸检测和上传）
	data, err := io.ReadAll(input.File)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// 自动检测图片尺寸（如果未提供）
	width := input.Width
	height := input.Height
	if width == 0 || height == 0 {
		cfg, _, decodeErr := image.DecodeConfig(bytes.NewReader(data))
		if decodeErr == nil {
			width = cfg.Width
			height = cfg.Height
		}
	}

	// 生成 R2 对象 key
	ext := strings.ToLower(filepath.Ext(input.Filename))
	if ext == "" {
		ext = ".jpg"
	}
	now := time.Now()
	key := fmt.Sprintf("images/%s/%06d%s",
		now.Format("2006/01/02"),
		rand.Int31n(1000000),
		ext,
	)

	// 上传到 R2
	resourceURL, err := s.r2Client.Upload(ctx, key, input.ContentType, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to upload to r2: %w", err)
	}

	// 设置默认值
	rating := input.Rating
	if rating == 0 {
		rating = 4.0
	}
	views := input.Views
	if views == "" {
		views = "1k"
	}

	// 保存数据库记录
	resource := &models.ImageResource{
		ID:          snowflake.Generate(),
		CreateBy:    fmt.Sprintf("%d", input.AuthorID),
		CreateTime:  now,
		ResourceURL: resourceURL,
		Width:       width,
		Height:      height,
		AuthorID:    input.AuthorID,
		Rating:      rating,
		Views:       views,
	}

	if err := s.repo.Create(ctx, resource); err != nil {
		return nil, fmt.Errorf("failed to save image resource: %w", err)
	}

	// 上传成功后更新缓存
	s.invalidateCache(ctx, resource)

	return resource, nil
}

// invalidateCache 清除分页缓存 + 将新图片加入随机池
func (s *UploadService) invalidateCache(ctx context.Context, resource *models.ImageResource) {
	if s.rdb == nil {
		return
	}

	// 1. 清除分页查询缓存（删除所有 cache:image_resource:list:v1:page:* key）
	s.clearListCache(ctx)

	// 2. 将新图片加入随机池
	s.addToRandomPool(ctx, resource)
}

// clearListCache 清除分页缓存
func (s *UploadService) clearListCache(ctx context.Context) {
	// 用 SCAN 匹配分页缓存 key 并删除
	var cursor uint64
	pattern := "cache:image_resource:list:v1:page:*"
	for {
		keys, nextCursor, err := s.rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			break
		}
		if len(keys) > 0 {
			_ = s.rdb.Del(ctx, keys...).Err()
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
}

// addToRandomPool 将新图片加入 Redis 随机池
func (s *UploadService) addToRandomPool(ctx context.Context, resource *models.ImageResource) {
	if s.poolSetKey == "" {
		return
	}

	row := models.ImageResourceRow{
		ResourceURL: resource.ResourceURL,
		Width:       resource.Width,
		Height:      resource.Height,
		Rating:      int(resource.Rating),
		Views:       resource.Views,
	}

	b, err := json.Marshal(row)
	if err != nil {
		return
	}

	_ = s.rdb.SAdd(ctx, s.poolSetKey, string(b)).Err()
}
