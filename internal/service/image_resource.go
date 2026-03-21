package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"girls-rating-api/internal/models"
	"girls-rating-api/internal/repository"
	rediscache "girls-rating-api/pkg/redis"

	"github.com/go-redis/redis/v8"
)

// ImageResourceService 图片资源服务
type ImageResourceService struct {
	repo   *repository.ImageResourceRepository
	rdb    *rediscache.Client
	ttl    time.Duration
	s3Host string
}

// NewImageResourceService 创建图片资源服务
func NewImageResourceService(repo *repository.ImageResourceRepository, rdb *rediscache.Client, ttl time.Duration, s3Host string) *ImageResourceService {
	return &ImageResourceService{repo: repo, rdb: rdb, ttl: ttl, s3Host: s3Host}
}

// ListInput 分页查询输入
type ListInput struct {
	Page     int
	PageSize int
}

// List 分页查询图片资源
func (s *ImageResourceService) List(ctx context.Context, input ListInput) (*repository.ListResult[models.ImageResource], error) {
	cacheKey := fmt.Sprintf("cache:image_resource:list:v1:page:%d:pageSize:%d", input.Page, input.PageSize)

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
