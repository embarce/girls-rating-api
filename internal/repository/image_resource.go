package repository

import (
	"context"

	"girls-rating-api/internal/models"

	"gorm.io/gorm"
)

// ImageResourceRepository 图片资源仓库
type ImageResourceRepository struct {
	db *gorm.DB
}

// NewImageResourceRepository 创建图片资源仓库
func NewImageResourceRepository(db *gorm.DB) *ImageResourceRepository {
	return &ImageResourceRepository{db: db}
}

// ListInput 分页查询输入参数
type ListInput struct {
	Page     int
	PageSize int
}

// ListResult 分页查询结果
type ListResult[T any] struct {
	List  []T   `json:"list"`
	Total int64 `json:"total"`
}

// Create 创建图片资源记录
func (r *ImageResourceRepository) Create(ctx context.Context, resource *models.ImageResource) error {
	return r.db.WithContext(ctx).Create(resource).Error
}

// List 分页查询图片资源，按创建时间倒序
func (r *ImageResourceRepository) List(ctx context.Context, input ListInput) (*ListResult[models.ImageResource], error) {
	offset := (input.Page - 1) * input.PageSize

	// 查询总数
	var total int64
	if err := r.db.WithContext(ctx).Model(&models.ImageResource{}).Count(&total).Error; err != nil {
		return nil, err
	}

	// 分页查询
	var list []models.ImageResource
	err := r.db.WithContext(ctx).
		Offset(offset).
		Limit(input.PageSize).
		Order("create_time DESC").
		Find(&list).Error
	if err != nil {
		return nil, err
	}

	return &ListResult[models.ImageResource]{
		List:  list,
		Total: total,
	}, nil
}
