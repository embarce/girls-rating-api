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
	"log"
	"math/rand"
	"path/filepath"
	"strings"
	"time"

	"girls-rating-api/internal/config"
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
	authors    *AuthorService
	photoCfg   config.PhotoConfig
	poolSetKey string
	s3Host     string
}

// NewUploadService 创建上传服务
func NewUploadService(r2Client *r2.Client, repo *repository.ImageResourceRepository, rdb *rediscache.Client, authors *AuthorService, photoCfg config.PhotoConfig, poolSetKey string, s3Host string) *UploadService {
	return &UploadService{
		r2Client:   r2Client,
		repo:       repo,
		rdb:        rdb,
		authors:    authors,
		photoCfg:   photoCfg,
		poolSetKey: poolSetKey,
		s3Host:     s3Host,
	}
}

// UploadInput 上传输入
type UploadInput struct {
	File         io.Reader
	Filename     string
	ContentType  string
	Size         int64
	Width        int     // 可选，0 表示自动检测
	Height       int     // 可选，0 表示自动检测
	Rating       float64 // 可选，0 表示使用默认值 4.0
	Views        string  // 可选，空表示使用默认值 "1k"
	AuthorID     int64   // 可选，指向 authors.id；无效值回退默认作者
	AuthorName   string  // 可选，find-or-create 兜底通道（优先级低于 AuthorID）
	AuthorAvatar string  // 可选，配合 AuthorName 创建作者时的头像外链
	Rarity       *int8   // 可选，显式定档（覆盖派生）；nil 按 rating 派生
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
		cfg, format, decodeErr := image.DecodeConfig(bytes.NewReader(data))
		if decodeErr != nil {
			log.Printf("[upload] image.DecodeConfig failed (format=%q, size=%d): %v", format, len(data), decodeErr)
		} else {
			width = cfg.Width
			height = cfg.Height
			log.Printf("[upload] detected image dimensions: %dx%d (format=%q)", width, height, format)
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

	// 作者解析优先级：author_id > author_name(find-or-create) > 默认作者
	authorID, err := s.resolveAuthorID(ctx, input)
	if err != nil {
		return nil, err
	}

	// 定档：显式 rarity 优先，缺省按 rating 阈值派生（photo-system.md §四.1）
	rarity := ResolveRarity(rating, input.Rarity, s.photoCfg)

	// 保存数据库记录（resource_url 存相对路径，查询时由 BuildS3URL 拼接完整 URL）
	resource := &models.ImageResource{
		ID:          snowflake.Generate(),
		CreateBy:    "Embrace",
		CreateTime:  now,
		ResourceURL: resourceURL,
		Width:       width,
		Height:      height,
		AuthorID:    authorID,
		Rating:      rating,
		Views:       views,
		Rarity:      rarity,
	}

	if err := s.repo.Create(ctx, resource); err != nil {
		return nil, fmt.Errorf("failed to save image resource: %w", err)
	}

	// 上传成功后更新缓存
	s.invalidateCache(ctx, resource)

	return resource, nil
}

// UploadAvatar 上传作者头像到 R2 avatars/ 前缀，返回完整公开 URL（管理接口用）。
func (s *UploadService) UploadAvatar(ctx context.Context, filename, contentType string, body io.Reader) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		ext = ".jpg"
	}
	key := fmt.Sprintf("avatars/%s/%06d%s",
		time.Now().Format("2006/01/02"),
		rand.Int31n(1000000),
		ext,
	)

	objectKey, err := s.r2Client.Upload(ctx, key, contentType, body)
	if err != nil {
		return "", fmt.Errorf("failed to upload avatar to r2: %w", err)
	}
	return buildS3URL(objectKey, s.s3Host), nil
}

// resolveAuthorID 上传链路的作者解析（docs/design/photo-system.md §5.3）。
// author_id 无效 → 回退默认作者；author_name → find-or-create；都缺 → 默认作者。
func (s *UploadService) resolveAuthorID(ctx context.Context, input UploadInput) (int64, error) {
	if input.AuthorID > 0 {
		return s.authors.EnsureValidID(ctx, input.AuthorID), nil
	}
	if input.AuthorName != "" {
		author, err := s.authors.FindOrCreateByName(ctx, input.AuthorName, input.AuthorAvatar)
		if err != nil {
			return 0, fmt.Errorf("failed to resolve author by name: %w", err)
		}
		return int64(author.ID), nil
	}
	return s.authors.DefaultID(), nil
}

// invalidateCache 清除分页缓存 + 将新图片加入随机池与抽卡池
func (s *UploadService) invalidateCache(ctx context.Context, resource *models.ImageResource) {
	if s.rdb == nil {
		return
	}

	// 1. 清除分页查询缓存（删除所有 cache:image_resource:list:v1:page:* key）
	s.clearListCache(ctx)

	// 2. 将新图片加入随机池（PhotoRow v2，含 rarity/authorId）
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

// addToRandomPool 将新图片加入 Redis 随机池（PhotoRow v2）
func (s *UploadService) addToRandomPool(ctx context.Context, resource *models.ImageResource) {
	if s.poolSetKey == "" {
		return
	}

	row := PhotoRowFromImage(resource)
	b, err := json.Marshal(row)
	if err != nil {
		return
	}

	_ = s.rdb.SAdd(ctx, s.poolSetKey, string(b)).Err()
}
