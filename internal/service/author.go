package service

import (
	"context"
	"sync"
	"time"

	"girls-rating-api/internal/config"
	"girls-rating-api/internal/models"
	"girls-rating-api/internal/repository"

	"gorm.io/gorm"
)

// authorCacheTTL 作者全表缓存 TTL。作者表极小（几个到几十行）、读极频繁；
// 多实例靠 TTL 收敛，管理接口写操作后主动失效本实例。
const authorCacheTTL = 5 * time.Minute

// maxAuthorNameLength authors.name 为 varchar(50)，超长截断（按 rune，避免截断多字节字符）。
const maxAuthorNameLength = 50

// AuthorService 作者服务。
// Resolve miss（含作者已软删）时回退默认作者（seed id=1），绝不因作者问题 500。
type AuthorService struct {
	repo      *repository.AuthorRepository
	defaultID int64

	mu       sync.RWMutex
	byID     map[int]models.Author
	loadedAt time.Time
}

func NewAuthorService(repo *repository.AuthorRepository, cfg config.PhotoConfig) *AuthorService {
	return &AuthorService{
		repo:      repo,
		defaultID: cfg.DefaultAuthorID,
		byID:      make(map[int]models.Author),
	}
}

// DefaultID 默认作者 id。
func (s *AuthorService) DefaultID() int64 {
	return s.defaultID
}

// load 保证缓存可用（惰性刷新，5min TTL）。加载失败不清空旧缓存。
func (s *AuthorService) load(ctx context.Context) error {
	s.mu.RLock()
	expired := time.Since(s.loadedAt) > authorCacheTTL || s.byID == nil
	s.mu.RUnlock()
	if !expired {
		return nil
	}

	authors, err := s.repo.ListAll(ctx)
	if err != nil {
		return err
	}

	byID := make(map[int]models.Author, len(authors))
	for _, a := range authors {
		byID[a.ID] = a
	}

	s.mu.Lock()
	s.byID = byID
	s.loadedAt = time.Now()
	s.mu.Unlock()
	return nil
}

// invalidate 清空本实例缓存（管理接口写操作后调用，下次读取重新加载）。
func (s *AuthorService) invalidate() {
	s.mu.Lock()
	s.byID = nil
	s.loadedAt = time.Time{}
	s.mu.Unlock()
}

// Resolve 解析作者为响应结构。miss（不存在/已软删/加载失败）回退默认作者；
// 默认作者也缺失时返回空作者（前端静默降级），不报错。
func (s *AuthorService) Resolve(ctx context.Context, authorID int64) AuthorResponse {
	// load 失败不阻断：走缓存/兜底查库逻辑
	_ = s.load(ctx)

	s.mu.RLock()
	author, ok := s.byID[int(authorID)]
	if !ok {
		author, ok = s.byID[int(s.defaultID)]
	}
	s.mu.RUnlock()

	if !ok {
		// 缓存未命中且没有默认作者（极小概率：加载失败 + 空缓存）→ 兜底查库一次
		if a, err := s.repo.FindByID(ctx, int(authorID)); err == nil {
			return toAuthorResponse(*a)
		}
		if a, err := s.repo.FindByID(ctx, int(s.defaultID)); err == nil {
			return toAuthorResponse(*a)
		}
		return AuthorResponse{}
	}
	return toAuthorResponse(author)
}

// EnsureValidID 校验作者 id 有效性，无效（不存在/已软删/加载失败）回退默认作者。
// 上传链路 author_id 参数语义生效的入口。
func (s *AuthorService) EnsureValidID(ctx context.Context, authorID int64) int64 {
	_ = s.load(ctx)

	s.mu.RLock()
	_, ok := s.byID[int(authorID)]
	s.mu.RUnlock()
	if ok {
		return authorID
	}
	return s.defaultID
}

// FindByID 按 id 查作者（管理接口详情）。
func (s *AuthorService) FindByID(ctx context.Context, id int) (*models.Author, error) {
	return s.repo.FindByID(ctx, id)
}

// FindByName 按 name 精确查询（爬虫存在性检查 + find-or-create 前置）。
func (s *AuthorService) FindByName(ctx context.Context, name string) (*models.Author, error) {
	return s.repo.FindByName(ctx, name)
}

// FindOrCreateByName find-or-create：name 已存在（含并发撞唯一索引后回查）返回既有记录。
// 上传接口 author_name 兜底通道使用；爬虫主流程应先查/建作者再以 author_id 上传。
func (s *AuthorService) FindOrCreateByName(ctx context.Context, name, avatar string) (*models.Author, error) {
	name = truncateName(name)
	if name == "" {
		return nil, gorm.ErrInvalidData
	}

	if author, err := s.repo.FindByName(ctx, name); err == nil {
		return author, nil
	}

	author := &models.Author{Name: name, Avatar: avatar}
	if err := s.repo.Create(ctx, author); err != nil {
		if repository.IsDuplicateEntryError(err) {
			// 并发创建撞唯一索引 → 回查既有记录，保持幂等
			return s.repo.FindByName(ctx, name)
		}
		return nil, err
	}

	s.invalidate()
	return author, nil
}

// AdminList 管理接口作者列表：name 精确（存在性检查）/ keyword 模糊分页。
func (s *AuthorService) AdminList(ctx context.Context, name, keyword string, page, pageSize int) (*repository.ListResult[models.Author], error) {
	if name != "" {
		author, err := s.repo.FindByName(ctx, name)
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return &repository.ListResult[models.Author]{List: []models.Author{}, Total: 0}, nil
			}
			return nil, err
		}
		return &repository.ListResult[models.Author]{List: []models.Author{*author}, Total: 1}, nil
	}
	return s.repo.List(ctx, keyword, page, pageSize)
}

// AdminGet 管理接口作者详情。
func (s *AuthorService) AdminGet(ctx context.Context, id int) (*models.Author, error) {
	return s.repo.FindByID(ctx, id)
}

// AdminCreate 幂等创建：name 已存在 → 返回既有记录 + existed=true（爬虫崩溃重试安全）。
func (s *AuthorService) AdminCreate(ctx context.Context, name, avatar string) (*models.Author, bool, error) {
	name = truncateName(name)

	if author, err := s.repo.FindByName(ctx, name); err == nil {
		return author, true, nil
	}

	author := &models.Author{Name: name, Avatar: avatar}
	if err := s.repo.Create(ctx, author); err != nil {
		if repository.IsDuplicateEntryError(err) {
			existing, findErr := s.repo.FindByName(ctx, name)
			if findErr != nil {
				return nil, false, err
			}
			return existing, true, nil
		}
		return nil, false, err
	}

	s.invalidate()
	return author, false, nil
}

// AdminUpdate 更新 name / avatar（nil 字段跳过）。改名撞唯一索引原样返回错误，
// 由 handler 映射为 409。
func (s *AuthorService) AdminUpdate(ctx context.Context, id int, name, avatar *string) (*models.Author, error) {
	author, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if name != nil {
		author.Name = truncateName(*name)
	}
	if avatar != nil {
		author.Avatar = *avatar
	}
	if err := s.repo.Save(ctx, author); err != nil {
		return nil, err
	}
	s.invalidate()
	return author, nil
}

// AdminDelete 软删除作者（同事务改写 name 释放唯一槽位），返回受影响图片数。
// 被引用图片经 Resolve 回退默认作者。
func (s *AuthorService) AdminDelete(ctx context.Context, id int) (int64, error) {
	author, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return 0, err
	}

	affected, err := s.repo.CountImagesByAuthor(ctx, int64(id))
	if err != nil {
		return 0, err
	}

	if err := s.repo.SoftDeleteWithRename(ctx, author); err != nil {
		return 0, err
	}

	s.invalidate()
	return affected, nil
}

// AdminUpdateAvatar 更新头像 URL（R2 上传或外链）。
func (s *AuthorService) AdminUpdateAvatar(ctx context.Context, id int, avatarURL string) (*models.Author, error) {
	return s.AdminUpdate(ctx, id, nil, &avatarURL)
}

func toAuthorResponse(a models.Author) AuthorResponse {
	return AuthorResponse{ID: int64(a.ID), Name: a.Name, Avatar: a.Avatar}
}

func truncateName(name string) string {
	runes := []rune(name)
	if len(runes) <= maxAuthorNameLength {
		return name
	}
	return string(runes[:maxAuthorNameLength])
}
