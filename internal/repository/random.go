package repository

import (
	"context"

	"girls-rating-api/internal/models"

	"gorm.io/gorm"
)

// RandomRepository 随机图片资源查询仓库。
type RandomRepository struct {
	db *gorm.DB
}

func NewRandomRepository(db *gorm.DB) *RandomRepository {
	return &RandomRepository{db: db}
}

// GetPoolResources 从 tb_girls_rating_image_resource 预取一批数据到 Redis 池。
// 注意：这里不使用 ORDER BY RAND()，避免在“刷新池”这一步也造成高开销。
func (r *RandomRepository) GetPoolResources(ctx context.Context, limit int) ([]models.ImageResourceRow, error) {
	const query = `
SELECT
  COALESCE(resource_url,'') AS resource_url,
  COALESCE(width,0) AS width,
  COALESCE(height,0) AS height,
  CAST(rating AS UNSIGNED) AS rating,
  COALESCE(views,'') AS views
FROM tb_girls_rating_image_resource
ORDER BY create_time DESC
LIMIT ?
`

	rows := make([]models.ImageResourceRow, 0, limit)
	if err := r.db.WithContext(ctx).Raw(query, limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetRandomResources 从 tb_girls_rating_image_resource 随机取数据。
func (r *RandomRepository) GetRandomResources(ctx context.Context, limit int) ([]models.ImageResourceRow, error) {
	// 说明：
	// - ORDER BY RAND() 属于随机排序的通用实现，但大数据量时会慢；本接口为第一版，后续可以再优化。
	// - 使用 COALESCE/CAST，避免 NULL 或 decimal 导致 JSON 结构与前端期望不一致。
	const query = `
SELECT
  COALESCE(resource_url,'') AS resource_url,
  COALESCE(width,0) AS width,
  COALESCE(height,0) AS height,
  CAST(rating AS UNSIGNED) AS rating,
  COALESCE(views,'') AS views
FROM tb_girls_rating_image_resource
ORDER BY RAND()
LIMIT ?
`

	rows := make([]models.ImageResourceRow, 0, limit)
	if err := r.db.WithContext(ctx).Raw(query, limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
