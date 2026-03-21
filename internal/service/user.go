package service

import (
	"context"
	"errors"
	"fmt"

	"girls-rating-api/internal/models"
	"girls-rating-api/internal/repository"
	"girls-rating-api/pkg/jwt"

	"github.com/go-playground/validator/v10"
	"golang.org/x/crypto/bcrypt"
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

// Register 用户注册
func (s *UserService) Register(ctx context.Context, input *RegisterInput) (*models.User, error) {
	// 验证输入
	if err := s.validator.Struct(input); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidValidation, err)
	}

	// 检查用户名或邮箱是否已存在
	existingUser, _ := s.repo.FindByUsername(ctx, input.Username)
	if existingUser != nil {
		return nil, ErrUserExists
	}

	existingUser, _ = s.repo.FindByEmail(ctx, input.Email)
	if existingUser != nil {
		return nil, ErrUserExists
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
		return "", "", ErrUserNotFound
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
		return nil, ErrUserNotFound
	}
	return user, nil
}
