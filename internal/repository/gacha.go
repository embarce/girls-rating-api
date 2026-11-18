package repository

import (
	"context"

	"girls-rating-api/internal/models"

	"gorm.io/gorm"
)

// GachaRepository 抽卡记录与抽卡池查询仓库。
type GachaRepository struct {
	db *gorm.DB
}

func NewGachaRepository(db *gorm.DB) *GachaRepository {
	return &GachaRepository{db: db}
}

// FindByDeviceAndDate 查询某设备某天的抽取记录（每日幂等判定）。
// 未命中返回 gorm.ErrRecordNotFound。
func (r *GachaRepository) FindByDeviceAndDate(ctx context.Context, deviceID, drawDate string) (*models.GachaDraw, error) {
	var row models.GachaDraw
	if err := r.db.WithContext(ctx).
		Where("device_id = ? AND draw_date = ?", deviceID, drawDate).
		First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// Create 插入抽取记录；同日重复插入会撞唯一索引 uq_gacha_draw_device_date，
// 由调用方用 IsDuplicateEntryError 转补发逻辑（并发双击首抽的收敛点）。
func (r *GachaRepository) Create(ctx context.Context, draw *models.GachaDraw) error {
	return r.db.WithContext(ctx).Create(draw).Error
}

// ListByDevice 设备的全部抽取记录，按日期倒序（卡册展开分页的数据源）。
func (r *GachaRepository) ListByDevice(ctx context.Context, deviceID string) ([]models.GachaDraw, error) {
	rows := make([]models.GachaDraw, 0, 16)
	if err := r.db.WithContext(ctx).
		Where("device_id = ?", deviceID).
		Order("created_at DESC, draw_date DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetPoolResourcesByRarity 按卡片等级预取池数据（gacha:pool:{ssr|sr|r} 的成员来源）。
func (r *GachaRepository) GetPoolResourcesByRarity(ctx context.Context, rarity int8, limit int) ([]models.PhotoRow, error) {
	query := `
SELECT
  COALESCE(resource_url,'') AS resource_url,
  COALESCE(width,0) AS width,
  COALESCE(height,0) AS height,
  COALESCE(rating,0) AS rating,
  COALESCE(views,'') AS views,
  COALESCE(rarity,0) AS rarity,
  COALESCE(author_id,1) AS author_id
FROM tb_girls_rating_image_resource
WHERE rarity = ?
ORDER BY create_time DESC
LIMIT ?
`

	rows := make([]models.PhotoRow, 0, limit)
	if err := r.db.WithContext(ctx).Raw(query, rarity, limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetRandomResourcesByRarity 按卡片等级随机取图（Redis 不可用时的兜底，照抄 RandomService 模式）。
func (r *GachaRepository) GetRandomResourcesByRarity(ctx context.Context, rarity int8, limit int) ([]models.PhotoRow, error) {
	query := `
SELECT
  COALESCE(resource_url,'') AS resource_url,
  COALESCE(width,0) AS width,
  COALESCE(height,0) AS height,
  COALESCE(rating,0) AS rating,
  COALESCE(views,'') AS views,
  COALESCE(rarity,0) AS rarity,
  COALESCE(author_id,1) AS author_id
FROM tb_girls_rating_image_resource
WHERE rarity = ?
ORDER BY RAND()
LIMIT ?
`

	rows := make([]models.PhotoRow, 0, limit)
	if err := r.db.WithContext(ctx).Raw(query, rarity, limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
