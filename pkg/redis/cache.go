package redis

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-redis/redis/v8"
)

// Cache 是一个对 Redis 的 JSON 缓存封装，便于业务层统一进行 key 管理和序列化。
type Cache struct {
	client *Client
	prefix string
}

func NewCache(client *Client, prefix string) *Cache {
	return &Cache{
		client: client,
		prefix: prefix,
	}
}

func (c *Cache) withPrefix(key string) string {
	if c == nil || c.prefix == "" {
		return key
	}
	return c.prefix + ":" + key
}

// GetJSON 从缓存读取 JSON，并反序列化到 dest（dest 建议传指针）。
// 返回 found 表示是否存在该 key。
func (c *Cache) GetJSON(ctx context.Context, key string, dest any) (found bool, err error) {
	val, err := c.client.Get(ctx, c.withPrefix(key)).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	if err := json.Unmarshal([]byte(val), dest); err != nil {
		return false, err
	}
	return true, nil
}

// SetJSON 将 value 序列化为 JSON 写入缓存。
// ttl <= 0 表示不设置过期时间。
func (c *Cache) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}

	if ttl <= 0 {
		return c.client.Set(ctx, c.withPrefix(key), b, 0).Err()
	}
	return c.client.Set(ctx, c.withPrefix(key), b, ttl).Err()
}

// Delete 删除缓存 key。
// 返回值为删除的 key 数量（0 或 1）。
func (c *Cache) Delete(ctx context.Context, key string) (int64, error) {
	return c.client.Del(ctx, c.withPrefix(key)).Result()
}
