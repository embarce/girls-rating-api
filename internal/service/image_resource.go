package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"girls-rating-api/internal/config"
	"girls-rating-api/internal/models"
	"girls-rating-api/internal/repository"
	rediscache "girls-rating-api/pkg/redis"

	"github.com/go-redis/redis/v8"
)

// ImageResourceService 图片资源服务
type ImageResourceService struct {
	repo     *repository.ImageResourceRepository
	rdb      *rediscache.Client
	authors  *AuthorService
	photoCfg config.PhotoConfig
	ttl      time.Duration
	s3Host   string
}

// NewImageResourceService 创建图片资源服务
func NewImageResourceService(repo *repository.ImageResourceRepository, rdb *rediscache.Client, authors *AuthorService, photoCfg config.PhotoConfig, ttl time.Duration, s3Host string) *ImageResourceService {
	return &ImageResourceService{repo: repo, rdb: rdb, authors: authors, photoCfg: photoCfg, ttl: ttl, s3Host: s3Host}
}

// ListInput 分页查询输入
type ListInput struct {
	Page     int
	PageSize int
	AuthorID int64 // 可选，>0 时按作者过滤（作者专区）
}

// List 分页查询图片资源
func (s *ImageResourceService) List(ctx context.Context, input ListInput) (*repository.ListResult[models.ImageResource], error) {
	cacheKey := fmt.Sprintf("cache:image_resource:list:v1:page:%d:pageSize:%d:author:%d", input.Page, input.PageSize, input.AuthorID)

	// 1) cache hit
	if s.rdb != nil {
		if b, err := s.rdb.Get(ctx, cacheKey).Bytes(); err == nil {
			var cached repository.ListResult[models.ImageResource]
			if err := json.Unmarshal(b, &cached); err == nil {
				return &cached, nil
			}
		} else if err != redis.Nil {
			// cache miss 或 redis 异常都不中断业务
		}
	}

	repoInput := repository.ListInput{
		Page:     input.Page,
		PageSize: input.PageSize,
		AuthorID: input.AuthorID,
	}

	// 2) cache miss => db
	result, err := s.repo.List(ctx, repoInput)
	if err != nil {
		return nil, err
	}

	// 3) write cache
	if s.rdb != nil && s.ttl > 0 {
		if b, err := json.Marshal(result); err == nil {
			_ = s.rdb.Set(ctx, cacheKey, b, s.ttl).Err()
		}
	}

	return result, nil
}

// ClearListCache 清除分页查询缓存（改档/改作者后必须调用，否则分页接口读到旧 rarity/author）。
func (s *ImageResourceService) ClearListCache(ctx context.Context) {
	if s.rdb == nil {
		return
	}

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

// ToPhotoResponse 图片资源行 → 统一照片响应（rating 浮点、rarity 名、作者表解析）。
func (s *ImageResourceService) ToPhotoResponse(ctx context.Context, item models.ImageResource, withID bool) PhotoResponse {
	resp := PhotoResponse{
		ResourceURL: s.BuildS3URL(item.ResourceURL),
		Width:       item.Width,
		Height:      item.Height,
		Rating:      item.Rating,
		Views:       item.Views,
		Rarity:      RarityName(item.Rarity),
		Author:      s.authors.Resolve(ctx, item.AuthorID),
	}
	if withID {
		resp.ID = fmt.Sprintf("%d", item.ID)
	}
	return resp
}

// AdminImageUpdateInput 管理接口改图输入（nil 字段跳过）。
type AdminImageUpdateInput struct {
	Rating   *float64
	Rarity   *int8 // 显式定档；nil 且 rating 有变时按新 rating 派生
	Views    *string
	AuthorID *int64 // 无效值回退默认作者
}

// AdminUpdateImage 管理接口修改图片（rating/rarity/views/author_id）：
// 重算定档落库 + 清分页缓存。池同步（随机池/抽卡池 SADD 新成员）由 handler 层调服务完成。
func (s *ImageResourceService) AdminUpdateImage(ctx context.Context, id int64, input AdminImageUpdateInput) (*models.ImageResource, error) {
	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Rating != nil {
		item.Rating = *input.Rating
	}
	if input.Views != nil {
		item.Views = *input.Views
	}
	if input.AuthorID != nil {
		item.AuthorID = s.authors.EnsureValidID(ctx, *input.AuthorID)
	}

	// 定档规则（photo-system.md §四.1）：显式 rarity > 派生（rating 变了才重算）
	if input.Rarity != nil {
		switch *input.Rarity {
		case models.RarityR, models.RaritySR, models.RaritySSR:
			item.Rarity = *input.Rarity
		}
	} else if input.Rating != nil {
		item.Rarity = DeriveRarity(item.Rating, s.photoCfg)
	}

	if err := s.repo.Save(ctx, item); err != nil {
		return nil, err
	}

	s.ClearListCache(ctx)
	return item, nil
}

// BuildS3URL 如果 s3Host 不为空，则将资源路径拼接为完整 URL；否则直接返回原路径
func (s *ImageResourceService) BuildS3URL(resourceURL string) string {
	if s.s3Host == "" {
		return resourceURL
	}
	// 避免双斜杠问题：s3Host 可能以 / 结尾，resourceURL 可能以 / 开头
	s3Host := strings.TrimSuffix(s.s3Host, "/")
	resourceURL = strings.TrimPrefix(resourceURL, "/")
	return s3Host + "/" + resourceURL
}
