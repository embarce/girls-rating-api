package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"girls-rating-api/internal/config"
	"girls-rating-api/internal/models"
	"girls-rating-api/internal/repository"
	rediscache "girls-rating-api/pkg/redis"
	"girls-rating-api/pkg/snowflake"

	"gorm.io/gorm"
)

// GachaCardResponse 卡牌结构：draw / status / collection 三接口共用。
// 内嵌 PhotoResponse（JSON 平铺），抽卡卡面与画廊共用同一照片形状。
// 快照即此结构的 JSON 数组（存储时 DrawDate 为空），读时原样反序列化，
// 不受后续图片删除/改档影响。
type GachaCardResponse struct {
	PhotoResponse
	DrawDate string `json:"drawDate,omitempty"` // 卡册展开时填充；当日抽卡/补发不带
}

// GachaStats 设备累计统计（按快照展开统计）。
type GachaStats struct {
	TotalDays  int64 `json:"totalDays"`
	TotalCards int64 `json:"totalCards"`
	SSR        int64 `json:"ssr"`
	SR         int64 `json:"sr"`
	R          int64 `json:"r"`
}

// DrawResult POST /api/gacha/draw 响应。
type DrawResult struct {
	Date     string              `json:"date"`
	Replayed bool                `json:"replayed"`
	Cards    []GachaCardResponse `json:"cards"`
	Stats    GachaStats          `json:"stats"`
}

// StatusResult GET /api/gacha/status 响应。
type StatusResult struct {
	Date          string              `json:"date"`
	DrawnToday    bool                `json:"drawnToday"`
	Cards         []GachaCardResponse `json:"cards"`
	Probabilities map[string]float64  `json:"probabilities"`
	Stats         GachaStats          `json:"stats"`
	Pool          map[string]int64    `json:"pool"`
}

// GachaCatalog 图鉴完成度统计：卡册内按 resourceUrl 去重后的各档张数
// （与 status.pool 的全站各档池存量相除即得完成度）。
type GachaCatalog struct {
	Total int `json:"total"`
	SSR   int `json:"ssr"`
	SR    int `json:"sr"`
	R     int `json:"r"`
}

// CollectionResult GET /api/gacha/collection 响应。
type CollectionResult struct {
	List     []GachaCardResponse `json:"list"`
	Total    int                 `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"pageSize"`
	Stats    GachaStats          `json:"stats"`
	Catalog  GachaCatalog        `json:"catalog"`
}

// GachaService 抽卡服务：每日幂等（唯一索引）+ 固定权重抽样 + 按档池随机。
type GachaService struct {
	repo    *repository.GachaRepository
	rdb     *rediscache.Client
	authors *AuthorService
	s3Host  string
	cfg     config.GachaConfig

	poolKeys map[int8]string
}

func NewGachaService(repo *repository.GachaRepository, rdb *rediscache.Client, authors *AuthorService, cfg config.GachaConfig, s3Host string) *GachaService {
	s := &GachaService{
		repo:    repo,
		rdb:     rdb,
		authors: authors,
		s3Host:  s3Host,
		cfg:     cfg,
		poolKeys: map[int8]string{
			models.RaritySSR: cfg.PoolSetKeyPrefix + ":ssr",
			models.RaritySR:  cfg.PoolSetKeyPrefix + ":sr",
			models.RarityR:   cfg.PoolSetKeyPrefix + ":r",
		},
	}

	// 启动时先尝试刷新一次 + 定时刷新（Redis 锁防多实例重复刷新）。
	if s.rdb != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_ = s.RefreshPools(ctx)
		cancel()

		if s.cfg.RefreshInterval > 0 {
			go s.refreshLoop()
		}
	}

	return s
}

// Draw 抽卡（幂等补发）：当天已抽时原样补发快照（replayed=true），不返回 409。
// 并发双击首抽靠唯一索引冲突收敛到同一份结果。
func (s *GachaService) Draw(ctx context.Context, deviceID string) (*DrawResult, error) {
	date := s.today()

	// 1. 幂等查：当天已有 → 补发
	row, err := s.repo.FindByDeviceAndDate(ctx, deviceID, date)
	if err == nil {
		return s.replay(ctx, row)
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	// 2. 掷档抽样：权重掷档 × DrawSize，档内池随机，单次内按 resourceUrl 去重
	cards, err := s.drawCards(ctx)
	if err != nil {
		return nil, err
	}

	// 3. INSERT（唯一索引兜底并发首抽）；冲突 → 转补发
	b, err := json.Marshal(cards)
	if err != nil {
		return nil, err
	}
	draw := &models.GachaDraw{
		ID:        snowflake.Generate(),
		DeviceID:  deviceID,
		DrawDate:  date,
		Cards:     string(b),
		CreatedAt: time.Now(),
	}
	if err := s.repo.Create(ctx, draw); err != nil {
		if repository.IsDuplicateEntryError(err) {
			row, getErr := s.repo.FindByDeviceAndDate(ctx, deviceID, date)
			if getErr != nil {
				return nil, getErr
			}
			return s.replay(ctx, row)
		}
		return nil, err
	}

	stats, err := s.computeStats(ctx, deviceID)
	if err != nil {
		return nil, err
	}

	return &DrawResult{Date: date, Replayed: false, Cards: cards, Stats: *stats}, nil
}

// Status 今日状态 + 公示概率 + 累计统计。
func (s *GachaService) Status(ctx context.Context, deviceID string) (*StatusResult, error) {
	date := s.today()

	row, err := s.repo.FindByDeviceAndDate(ctx, deviceID, date)
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}

	var cards []GachaCardResponse
	if err == nil {
		cards, err = s.rowCards(row)
		if err != nil {
			return nil, err
		}
	}

	stats, err := s.computeStats(ctx, deviceID)
	if err != nil {
		return nil, err
	}

	return &StatusResult{
		Date:       date,
		DrawnToday: err == nil,
		Cards:      cards,
		Probabilities: map[string]float64{
			"R":   s.cfg.RWeight,
			"SR":  s.cfg.SRWeight,
			"SSR": s.cfg.SSRWeight,
		},
		Stats: *stats,
		Pool:  s.poolCounts(ctx),
	}, nil
}

// Collection 卡册分页：按设备取全部快照按时间倒序展开，应用层切片分页
// （行数 = 天数，单设备一年 365 行，内存切片毫无压力）。
func (s *GachaService) Collection(ctx context.Context, deviceID string, page, pageSize int) (*CollectionResult, error) {
	rows, err := s.repo.ListByDevice(ctx, deviceID)
	if err != nil {
		return nil, err
	}

	all := make([]GachaCardResponse, 0, len(rows)*s.cfg.DrawSize)
	seen := make(map[string]int8, len(rows)*s.cfg.DrawSize)
	for i := range rows {
		cards, err := s.rowCards(&rows[i])
		if err != nil {
			return nil, err
		}
		for j := range cards {
			cards[j].DrawDate = rows[i].DrawDate
			// 图鉴：按 resourceUrl 去重（0=R 1=SR 2=SSR，首次抽到时的档位）
			if _, dup := seen[cards[j].ResourceURL]; !dup {
				switch cards[j].Rarity {
				case "SSR":
					seen[cards[j].ResourceURL] = 2
				case "SR":
					seen[cards[j].ResourceURL] = 1
				default:
					seen[cards[j].ResourceURL] = 0
				}
			}
		}
		all = append(all, cards...)
	}

	stats := summarize(all)

	catalog := GachaCatalog{}
	for _, rarity := range seen {
		catalog.Total++
		switch rarity {
		case 2:
			catalog.SSR++
		case 1:
			catalog.SR++
		default:
			catalog.R++
		}
	}

	start := (page - 1) * pageSize
	if start > len(all) {
		start = len(all)
	}
	end := start + pageSize
	if end > len(all) {
		end = len(all)
	}

	list := make([]GachaCardResponse, 0, end-start)
	list = append(list, all[start:end]...)

	return &CollectionResult{
		List:     list,
		Total:    len(all),
		Page:     page,
		PageSize: pageSize,
		Stats:    stats,
		Catalog:  catalog,
	}, nil
}

// AddToPool 热加路径：新图片进对应稀有度池（PhotoRow v2）。
// 改档后旧档位的残留成员由定时刷新 RENAME 全量替换兜底。
func (s *GachaService) AddToPool(ctx context.Context, row models.PhotoRow) error {
	if s.rdb == nil {
		return nil
	}
	key, ok := s.poolKeys[row.Rarity]
	if !ok {
		key = s.poolKeys[models.RarityR]
	}
	b, err := json.Marshal(row)
	if err != nil {
		return err
	}
	return s.rdb.SAdd(ctx, key, string(b)).Err()
}

// RefreshPools 刷新三个稀有度池（照抄 RandomService.RefreshPool 的 tmp+RENAME 模式）。
func (s *GachaService) RefreshPools(ctx context.Context) error {
	if s.rdb == nil || s.cfg.PoolSize <= 0 {
		return nil
	}

	for rarity := range s.poolKeys {
		if err := s.refreshPool(ctx, rarity); err != nil {
			return err
		}
	}
	return nil
}

func (s *GachaService) refreshPool(ctx context.Context, rarity int8) error {
	key := s.poolKeys[rarity]
	lockKey := key + ":lock"
	ok, err := s.rdb.SetNX(ctx, lockKey, "1", s.cfg.PoolLockTTL).Result()
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	defer func() {
		_ = s.rdb.Del(context.Background(), lockKey).Err()
	}()

	rows, err := s.repo.GetPoolResourcesByRarity(ctx, rarity, s.cfg.PoolSize)
	if err != nil {
		return err
	}

	// 该档无图：清掉可能残留的旧池，让抽样走降级链
	if len(rows) == 0 {
		return s.rdb.Del(ctx, key).Err()
	}

	tmpKey := fmt.Sprintf("%s:tmp:%d", key, time.Now().UnixNano())
	const batchSize = 1000
	pipe := s.rdb.Pipeline()
	for start := 0; start < len(rows); start += batchSize {
		end := start + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		chunk := make([]interface{}, 0, end-start)
		for _, r := range rows[start:end] {
			b, err := json.Marshal(r)
			if err != nil {
				continue
			}
			chunk = append(chunk, string(b))
		}
		if len(chunk) == 0 {
			continue
		}
		pipe.SAdd(ctx, tmpKey, chunk...)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}

	if err := s.rdb.Rename(ctx, tmpKey, key).Err(); err != nil {
		_ = s.rdb.Del(ctx, tmpKey).Err()
		return err
	}

	if s.cfg.RefreshInterval > 0 {
		_ = s.rdb.Expire(ctx, key, 2*s.cfg.RefreshInterval).Err()
	}
	return nil
}

func (s *GachaService) refreshLoop() {
	ticker := time.NewTicker(s.cfg.RefreshInterval)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		_ = s.RefreshPools(ctx)
		cancel()
	}
}

// tierCandidates 单档候选队列：一次取足去重所需数量，游标推进避免重复请求。
type tierCandidates struct {
	rows   []models.PhotoRow
	cursor int
}

// drawCards 抽 10 张：权重掷档 × DrawSize；掷中档池空 → SSR→SR→R 依次降级；
// 全空 → 500（库里没图，画廊同样为空）。单次内按 resourceUrl 去重，不足不补重复。
func (s *GachaService) drawCards(ctx context.Context) ([]GachaCardResponse, error) {
	// 每档候选数量：覆盖去重后的最大需求，留余量
	want := s.cfg.DrawSize*2 + 10
	tiers := make(map[int8]*tierCandidates, 3)

	picked := make(map[string]struct{}, s.cfg.DrawSize)
	cards := make([]GachaCardResponse, 0, s.cfg.DrawSize)

	for len(cards) < s.cfg.DrawSize {
		rarity := s.rollRarity()

		row, ok, err := s.pickFromTier(ctx, rarity, want, tiers, picked)
		if err != nil {
			return nil, err
		}
		if !ok {
			// 掷中档空 → 向下降级
			for fallback := rarity - 1; fallback >= models.RarityR && !ok; fallback-- {
				row, ok, err = s.pickFromTier(ctx, fallback, want, tiers, picked)
				if err != nil {
					return nil, err
				}
			}
		}
		if !ok {
			return nil, fmt.Errorf("gacha pool exhausted")
		}

		picked[row.ResourceURL] = struct{}{}
		cards = append(cards, s.toCard(ctx, row))
	}
	return cards, nil
}

// pickFromTier 从档候选队列取下一张未抽中的图；队列未加载则先加载。
// ok=false 表示该档已耗尽（池空或全部与已抽重复）。
func (s *GachaService) pickFromTier(ctx context.Context, rarity int8, want int, tiers map[int8]*tierCandidates, picked map[string]struct{}) (models.PhotoRow, bool, error) {
	tier, ok := tiers[rarity]
	if !ok {
		rows, err := s.loadTier(ctx, rarity, want)
		if err != nil {
			return models.PhotoRow{}, false, err
		}
		tier = &tierCandidates{rows: rows}
		tiers[rarity] = tier
	}

	for tier.cursor < len(tier.rows) {
		row := tier.rows[tier.cursor]
		tier.cursor++
		if _, dup := picked[row.ResourceURL]; !dup {
			return row, true, nil
		}
	}
	return models.PhotoRow{}, false, nil
}

// loadTier 加载单档候选：优先 Redis 池随机取，Redis 不可用/异常 → DB ORDER BY RAND() 回退。
func (s *GachaService) loadTier(ctx context.Context, rarity int8, want int) ([]models.PhotoRow, error) {
	if s.rdb != nil {
		key := s.poolKeys[rarity]
		members, err := s.rdb.SRandMemberN(ctx, key, int64(want)).Result()
		if err == nil {
			rows := make([]models.PhotoRow, 0, len(members))
			for _, m := range members {
				var row models.PhotoRow
				if json.Unmarshal([]byte(m), &row) == nil && row.ResourceURL != "" {
					rows = append(rows, row)
				}
			}
			if len(rows) > 0 {
				return rows, nil
			}
			// 池空：尝试刷新一次再取，仍空则落 DB 回退（该档可能真的没图）
			_ = s.refreshPool(ctx, rarity)
			members, err = s.rdb.SRandMemberN(ctx, key, int64(want)).Result()
			if err == nil {
				rows = rows[:0]
				for _, m := range members {
					var row models.PhotoRow
					if json.Unmarshal([]byte(m), &row) == nil && row.ResourceURL != "" {
						rows = append(rows, row)
					}
				}
				if len(rows) > 0 {
					return rows, nil
				}
				return []models.PhotoRow{}, nil
			}
		}
		// Redis 异常 → 落 DB 回退
	}

	// 无 Redis 或 Redis 异常：直接查库
	dbRows, err := s.repo.GetRandomResourcesByRarity(ctx, rarity, want)
	if err != nil {
		if s.rdb != nil {
			return []models.PhotoRow{}, nil // 该档按空处理，走降级
		}
		return nil, err
	}
	return dbRows, nil
}

// rollRarity 固定权重掷档：SSR → SR → R（累计概率），库内组成只影响同图重复频率，不影响档位概率。
func (s *GachaService) rollRarity() int8 {
	total := s.cfg.SSRWeight + s.cfg.SRWeight + s.cfg.RWeight
	if total <= 0 {
		return models.RarityR
	}
	roll := rand.Float64() * total
	if roll < s.cfg.SSRWeight {
		return models.RaritySSR
	}
	if roll < s.cfg.SSRWeight+s.cfg.SRWeight {
		return models.RaritySR
	}
	return models.RarityR
}

// replay 补发当日快照：读时原样反序列化，不受后续图片删除/改档影响。
func (s *GachaService) replay(ctx context.Context, row *models.GachaDraw) (*DrawResult, error) {
	cards, err := s.rowCards(row)
	if err != nil {
		return nil, err
	}

	stats, err := s.computeStats(ctx, row.DeviceID)
	if err != nil {
		return nil, err
	}

	return &DrawResult{
		Date:     row.DrawDate,
		Replayed: true,
		Cards:    cards,
		Stats:    *stats,
	}, nil
}

// rowCards 反序列化快照行。
func (s *GachaService) rowCards(row *models.GachaDraw) ([]GachaCardResponse, error) {
	cards := make([]GachaCardResponse, 0, s.cfg.DrawSize)
	if row.Cards == "" {
		return cards, nil
	}
	if err := json.Unmarshal([]byte(row.Cards), &cards); err != nil {
		return nil, fmt.Errorf("failed to unmarshal gacha cards snapshot: %w", err)
	}
	return cards, nil
}

// computeStats 设备累计统计（按快照展开）。
func (s *GachaService) computeStats(ctx context.Context, deviceID string) (*GachaStats, error) {
	rows, err := s.repo.ListByDevice(ctx, deviceID)
	if err != nil {
		return nil, err
	}

	stats := &GachaStats{TotalDays: int64(len(rows))}
	for i := range rows {
		cards, err := s.rowCards(&rows[i])
		if err != nil {
			return nil, err
		}
		mergeStats(stats, cards)
	}
	return stats, nil
}

// poolCounts 各档池现存量（供管理页/调试；无 Redis 返回空 map，前端可缺省）。
func (s *GachaService) poolCounts(ctx context.Context) map[string]int64 {
	pool := make(map[string]int64, 3)
	if s.rdb == nil {
		return pool
	}
	for rarity, key := range s.poolKeys {
		count, err := s.rdb.SCard(ctx, key).Result()
		if err != nil {
			continue
		}
		pool[RarityName(rarity)] = count
	}
	return pool
}

// toCard 池行 → 卡牌响应（作者响应期解析，快照定格抽卡时刻的作者数据）。
func (s *GachaService) toCard(ctx context.Context, row models.PhotoRow) GachaCardResponse {
	return GachaCardResponse{
		PhotoResponse: PhotoResponse{
			ResourceURL: buildS3URL(row.ResourceURL, s.s3Host),
			Width:       row.Width,
			Height:      row.Height,
			Rating:      row.Rating,
			Views:       row.Views,
			Rarity:      RarityName(row.Rarity),
			Author:      s.authors.Resolve(ctx, row.AuthorID),
		},
	}
}

func summarize(cards []GachaCardResponse) GachaStats {
	stats := GachaStats{}
	mergeStats(&stats, cards)
	return stats
}

func mergeStats(stats *GachaStats, cards []GachaCardResponse) {
	for _, c := range cards {
		switch c.Rarity {
		case "SSR":
			stats.SSR++
		case "SR":
			stats.SR++
		default:
			stats.R++
		}
	}
	stats.TotalCards += int64(len(cards))
}

// today 服务器按配置时区取当日日期（draw_date 权威来源，前端不自己算）。
func (s *GachaService) today() string {
	loc, err := time.LoadLocation(s.cfg.Timezone)
	if err != nil {
		loc = time.Local
	}
	return time.Now().In(loc).Format("2006-01-02")
}
