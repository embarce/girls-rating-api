package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"girls-rating-api/internal/config"
	"girls-rating-api/internal/models"
	"girls-rating-api/internal/repository"
	rediscache "girls-rating-api/pkg/redis"
)

// AuthorResponse 返回给前端展示的作者信息（当前无业务含义，固定硬编码）。
// @Description 作者信息
type AuthorResponse struct {
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

// RandomItemResponse 单条随机图片资源响应。
// @Description 随机图片资源项
type RandomItemResponse struct {
	ResourceURL string         `json:"resourceUrl"`
	Width       int            `json:"width"`
	Height      int            `json:"height"`
	Rating      int            `json:"rating"`
	Views       string         `json:"views"`
	Author      AuthorResponse `json:"author"`
}

// RandomService 随机图片资源服务。
type RandomService struct {
	repo *repository.RandomRepository
	rdb  *rediscache.Client

	poolSize        int
	refreshInterval time.Duration
	poolLockTTL     time.Duration
	poolSetKey      string
}

func NewRandomService(repo *repository.RandomRepository, rdb *rediscache.Client, cfg config.RandomConfig) *RandomService {
	s := &RandomService{
		repo:            repo,
		rdb:             rdb,
		poolSize:        cfg.PoolSize,
		refreshInterval: cfg.RefreshInterval,
		poolLockTTL:     cfg.PoolLockTTL,
		poolSetKey:      cfg.PoolSetKey,
	}

	// 启动时先尝试刷新一次，保证后续请求能直接从 Redis 抽。
	if s.rdb != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_ = s.RefreshPool(ctx)
		cancel()

		// 后台定时刷新随机池（使用 Redis 锁避免多实例重复刷新）。
		if s.refreshInterval > 0 {
			go s.refreshLoop()
		}
	}

	return s
}

// RandomResources 随机获取图片资源并组装返回结构。
func (s *RandomService) RandomResources(ctx context.Context, limit int) ([]RandomItemResponse, error) {
	// 如果 Redis 没配上，退回数据库 ORDER BY RAND() 的旧逻辑，保证功能不受影响。
	if s.rdb == nil {
		rows, err := s.repo.GetRandomResources(ctx, limit)
		if err != nil {
			return nil, err
		}

		items := make([]RandomItemResponse, 0, len(rows))
		for _, r := range rows {
			items = append(items, toRandomItemResponse(r))
		}
		return items, nil
	}

	// 优先从 Redis 随机池中抽取。
	members, err := s.rdb.SRandMemberN(ctx, s.poolSetKey, int64(limit)).Result()
	if err != nil {
		// Redis 异常时也退回数据库逻辑，避免直接 500。
		rows, dbErr := s.repo.GetRandomResources(ctx, limit)
		if dbErr != nil {
			return nil, err
		}
		items := make([]RandomItemResponse, 0, len(rows))
		for _, r := range rows {
			items = append(items, toRandomItemResponse(r))
		}
		return items, nil
	}

	// 池为空可能是刷新失败或刚启动；尝试刷新一次再取。
	if len(members) == 0 {
		_ = s.RefreshPool(ctx)
		members, _ = s.rdb.SRandMemberN(ctx, s.poolSetKey, int64(limit)).Result()
	}

	items := make([]RandomItemResponse, 0, len(members))
	for _, m := range members {
		var row models.ImageResourceRow
		if err := json.Unmarshal([]byte(m), &row); err != nil {
			continue
		}
		items = append(items, toRandomItemResponse(row))
	}
	return items, nil
}

// RefreshPool 刷新 Redis 随机池。
// 使用 Set key：poolSetKey；成员值为 ImageResourceRow 的 JSON 字符串。
func (s *RandomService) RefreshPool(ctx context.Context) error {
	if s.rdb == nil || s.poolSize <= 0 {
		return nil
	}

	lockKey := s.poolSetKey + ":lock"
	ok, err := s.rdb.SetNX(ctx, lockKey, "1", s.poolLockTTL).Result()
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	defer func() {
		_ = s.rdb.Del(context.Background(), lockKey).Err()
	}()

	rows, err := s.repo.GetPoolResources(ctx, s.poolSize)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	// 把旧 key 保留到 tmp 构建完成后再 RENAME，避免“刷新期间请求直接读到空池”。
	tmpKey := fmt.Sprintf("%s:tmp:%d", s.poolSetKey, time.Now().UnixNano())
	members := make([]string, 0, len(rows))
	for _, r := range rows {
		b, err := json.Marshal(r)
		if err != nil {
			continue
		}
		members = append(members, string(b))
	}
	if len(members) == 0 {
		return nil
	}

	const batchSize = 1000
	pipe := s.rdb.Pipeline()
	for start := 0; start < len(members); start += batchSize {
		end := start + batchSize
		if end > len(members) {
			end = len(members)
		}

		chunk := make([]interface{}, 0, end-start)
		for i := start; i < end; i++ {
			chunk = append(chunk, members[i])
		}
		pipe.SAdd(ctx, tmpKey, chunk...)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}

	// RENAME 会把 destination 覆盖掉（替换旧池）。
	if err := s.rdb.Rename(ctx, tmpKey, s.poolSetKey).Err(); err != nil {
		_ = s.rdb.Del(ctx, tmpKey).Err()
		return err
	}

	// 给池一个长一点的过期，避免“极端情况下刷新一直失败导致永久脏数据”。
	if s.refreshInterval > 0 {
		_ = s.rdb.Expire(ctx, s.poolSetKey, 2*s.refreshInterval).Err()
	}
	return nil
}

func (s *RandomService) refreshLoop() {
	ticker := time.NewTicker(s.refreshInterval)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		_ = s.RefreshPool(ctx)
		cancel()
	}
}

func toRandomItemResponse(r models.ImageResourceRow) RandomItemResponse {
	return RandomItemResponse{
		ResourceURL: r.ResourceURL,
		Width:       r.Width,
		Height:      r.Height,
		Rating:      r.Rating,
		Views:       r.Views,
		Author: AuthorResponse{
			Name:   "Embrace",
			Avatar: "http://localhost:3000/images/avatars/avatar1.webp",
		},
	}
}
