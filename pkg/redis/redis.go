package redis

import (
	"context"
	"fmt"

	"girls-rating-api/internal/config"

	"github.com/go-redis/redis/v8"
)

// Client Redis 客户端
type Client struct {
	*redis.Client
}

// New 创建 Redis 客户端
func New(cfg config.RedisConfig) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr(),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx := context.Background()

	// 测试连接
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	return &Client{Client: rdb}, nil
}
