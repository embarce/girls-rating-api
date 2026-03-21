package service

import (
	"context"
	"errors"
	"fmt"

	"girls-rating-api/internal/models"
	"girls-rating-api/internal/repository"
	"girls-rating-api/pkg/jwt"

	"github.com/go-playground/validator/v10"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUserExists        = errors.New("username or email already exists")
	ErrInvalidPassword   = errors.New("invalid password")
	ErrInvalidValidation = errors.New("invalid validation")
)

// UserService 用户服务
type UserService struct {
	repo       *repository.UserRepository
	jwtService *jwt.Service
	validator  *validator.Validate
}

// NewUserService 创建用户服务
func NewUserService(repo *repository.UserRepository, jwtService *jwt.Service) *UserService {
	return &UserService{
		repo:       repo,
		jwtService: jwtService,
		validator:  validator.New(),
	}
}

// RegisterInput 注册输入
type RegisterInput struct {
	Username string `json:"username" validate:"required,min=3,max=50"`
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=6"`
}

// LoginInput 登录输入
type LoginInput struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// UserResponse 用户响应（不包含密码）
type UserResponse struct {
	ID        uint   `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	Nickname  string `json:"nickname"`
	Avatar    string `json:"avatar"`
	Status    int    `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Register 用户注册
func (s *UserService) Register(ctx context.Context, input *RegisterInput) (*models.User, error) {
	// 验证输入
	if err := s.validator.Struct(input); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidValidation, err)
	}

	// 检查用户名或邮箱是否已存在
	existingUser, err := s.repo.FindByUsername(ctx, input.Username)
	if err == nil && existingUser != nil {
		return nil, ErrUserExists
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to check username: %w", err)
	}

	existingUser, err = s.repo.FindByEmail(ctx, input.Email)
	if err == nil && existingUser != nil {
		return nil, ErrUserExists
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to check email: %w", err)
	}

	// 哈希密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// 创建用户
	user := &models.User{
		Username: input.Username,
		Email:    input.Email,
		Password: string(hashedPassword),
		Status:   1,
	}

	if err := s.repo.Create(ctx, user); err != nil {
		// 处理并发注册导致的唯一键冲突：让调用方收到业务语义，而不是裸 DB 错误。
		var mysqlErr *mysqlDriver.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return nil, ErrUserExists
		}
		return nil, err
	}

	return user, nil
}

// Login 用户登录
func (s *UserService) Login(ctx context.Context, input *LoginInput) (string, string, error) {
	// 验证输入
	if err := s.validator.Struct(input); err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrInvalidValidation, err)
	}

	// 查找用户
	user, err := s.repo.FindByUsername(ctx, input.Username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", ErrUserNotFound
		}
		return "", "", err
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.Password)); err != nil {
		return "", "", ErrInvalidPassword
	}

	// 检查用户状态
	if user.Status != 1 {
		return "", "", errors.New("user is disabled")
	}

	// 生成 token
	accessToken, err := s.jwtService.GenerateToken(user.ID, user.Username)
	if err != nil {
		return "", "", err
	}

	refreshToken, err := s.jwtService.GenerateRefreshToken(user.ID)
	if err != nil {
		return "", "", err
	}

	return accessToken, refreshToken, nil
}

// GetProfile 获取用户信息
func (s *UserService) GetProfile(ctx context.Context, userID uint) (*models.User, error) {
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}
