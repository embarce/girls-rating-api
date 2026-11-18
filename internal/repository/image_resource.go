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
	AuthorID int64 // 可选，>0 时按作者过滤（作者专区）
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

// GetByID 按 id 查询图片资源。未命中返回 gorm.ErrRecordNotFound。
func (r *ImageResourceRepository) GetByID(ctx context.Context, id int64) (*models.ImageResource, error) {
	var resource models.ImageResource
	if err := r.db.WithContext(ctx).First(&resource, id).Error; err != nil {
		return nil, err
	}
	return &resource, nil
}

// Save 更新图片资源记录
func (r *ImageResourceRepository) Save(ctx context.Context, resource *models.ImageResource) error {
	return r.db.WithContext(ctx).Save(resource).Error
}

// List 分页查询图片资源，按创建时间倒序；AuthorID > 0 时按作者过滤
func (r *ImageResourceRepository) List(ctx context.Context, input ListInput) (*ListResult[models.ImageResource], error) {
	offset := (input.Page - 1) * input.PageSize

	scope := r.db.WithContext(ctx).Model(&models.ImageResource{})
	if input.AuthorID > 0 {
		scope = scope.Where("author_id = ?", input.AuthorID)
	}

	// 查询总数
	var total int64
	if err := scope.Count(&total).Error; err != nil {
		return nil, err
	}

	// 分页查询
	var list []models.ImageResource
	err := scope.
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
