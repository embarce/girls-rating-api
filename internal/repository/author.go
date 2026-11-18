package repository

import (
	"context"
	"errors"
	"strconv"
	"time"

	"girls-rating-api/internal/models"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

// IsDuplicateEntryError 判断是否为唯一索引冲突（MySQL 1062）。
// 幂等创建 / 改名冲突 / 每日一抽兜底都依赖此判断。
func IsDuplicateEntryError(err error) bool {
	if err == nil {
		return false
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	return false
}

// AuthorRepository 作者仓库
type AuthorRepository struct {
	db *gorm.DB
}

func NewAuthorRepository(db *gorm.DB) *AuthorRepository {
	return &AuthorRepository{db: db}
}

// FindByID 按 id 查询作者（GORM 自动过滤软删行）。
func (r *AuthorRepository) FindByID(ctx context.Context, id int) (*models.Author, error) {
	var author models.Author
	if err := r.db.WithContext(ctx).First(&author, id).Error; err != nil {
		return nil, err
	}
	return &author, nil
}

// FindByName 按 name 精确查询作者（存在性检查入口）。
func (r *AuthorRepository) FindByName(ctx context.Context, name string) (*models.Author, error) {
	var author models.Author
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&author).Error; err != nil {
		return nil, err
	}
	return &author, nil
}

// ListAll 全量加载作者（作者服务内存缓存的数据源，表极小）。
func (r *AuthorRepository) ListAll(ctx context.Context) ([]models.Author, error) {
	authors := make([]models.Author, 0, 32)
	if err := r.db.WithContext(ctx).Order("id ASC").Find(&authors).Error; err != nil {
		return nil, err
	}
	return authors, nil
}

// List 作者列表：keyword 模糊匹配 name，分页按 id 升序。
func (r *AuthorRepository) List(ctx context.Context, keyword string, page, pageSize int) (*ListResult[models.Author], error) {
	query := r.db.WithContext(ctx).Model(&models.Author{})
	if keyword != "" {
		query = query.Where("name LIKE ?", "%"+keyword+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	offset := (page - 1) * pageSize
	list := make([]models.Author, 0, pageSize)
	if err := query.Offset(offset).Limit(pageSize).Order("id ASC").Find(&list).Error; err != nil {
		return nil, err
	}

	return &ListResult[models.Author]{List: list, Total: total}, nil
}

// Create 创建作者。
func (r *AuthorRepository) Create(ctx context.Context, author *models.Author) error {
	return r.db.WithContext(ctx).Create(author).Error
}

// Save 更新作者（全字段保存）。
func (r *AuthorRepository) Save(ctx context.Context, author *models.Author) error {
	return r.db.WithContext(ctx).Save(author).Error
}

// CountImagesByAuthor 统计引用该作者的图片数（删除作者时向调用方报告影响面）。
func (r *AuthorRepository) CountImagesByAuthor(ctx context.Context, authorID int64) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&models.ImageResource{}).
		Where("author_id = ?", authorID).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// SoftDeleteWithRename 软删除作者，并同事务把 name 改写为 "{name}#del#{id}"，
// 释放唯一索引槽位以允许重建同名作者（docs/design/photo-system.md §5.2）。
// 注意：GORM 会给 UPDATE 附加 deleted_at IS NULL 条件，这里必须 Unscoped 直接写 deleted_at。
func (r *AuthorRepository) SoftDeleteWithRename(ctx context.Context, author *models.Author) error {
	newName := author.Name + "#del#" + strconv.Itoa(author.ID)
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Unscoped().Model(author).Updates(map[string]any{
			"name":       newName,
			"deleted_at": time.Now(),
		}).Error
	})
}
