package handlers

import (
	"net/http"

	"girls-rating-api/internal/service"
	"girls-rating-api/pkg/response"

	"github.com/gin-gonic/gin"
)

// RegisterRequest 注册请求
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// RegisterResponse 注册响应（与统一 API 格式一致，供 Swagger 展示）
type RegisterResponse struct {
	Msg  string                `json:"msg"`
	Code int                   `json:"code"`
	Data *service.UserResponse `json:"data"`
}

// LoginResponse 登录响应（与统一 API 格式一致，供 Swagger 展示）
type LoginResponse struct {
	Msg  string            `json:"msg"`
	Code int               `json:"code"`
	Data map[string]string `json:"data"`
}

// ErrorResponse 错误响应（与统一 API 格式一致；data 实际为 JSON 空对象 {}）
type ErrorResponse struct {
	Msg  string `json:"msg"`
	Code int    `json:"code"`
	Data any    `json:"data"`
}

// Register 用户注册
// @Summary      用户注册
// @Description  创建新用户账号
// @Tags         用户认证
// @Accept       json
// @Produce      json
// @Param        request  body      RegisterRequest  true  "注册信息"
// @Success      201      {object}  RegisterResponse
// @Failure      400      {object}  ErrorResponse  "请求参数错误"
// @Failure      409      {object}  ErrorResponse  "用户名或邮箱已存在"
// @Failure      500      {object}  ErrorResponse  "服务器错误"
// @Router       /api/v1/register [post]
func (s *Server) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	input := &service.RegisterInput{
		Username: req.Username,
		Email:    req.Email,
		Password: req.Password,
	}

	user, err := s.UserService.Register(c.Request.Context(), input)
	if err != nil {
		if err == service.ErrUserExists {
			response.Error(c, http.StatusConflict, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(c, http.StatusCreated, user)
}

// Login 用户登录
// @Summary      用户登录
// @Description  使用用户名和密码登录，返回 JWT token
// @Tags         用户认证
// @Accept       json
// @Produce      json
// @Param        request  body      LoginRequest  true  "登录信息"
// @Success      200      {object}  LoginResponse
// @Failure      400      {object}  ErrorResponse  "请求参数错误"
// @Failure      401      {object}  ErrorResponse  "用户名或密码错误"
// @Failure      500      {object}  ErrorResponse  "服务器错误"
// @Router       /api/v1/login [post]
func (s *Server) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	input := &service.LoginInput{
		Username: req.Username,
		Password: req.Password,
	}

	accessToken, refreshToken, err := s.UserService.Login(c.Request.Context(), input)
	if err != nil {
		if err == service.ErrUserNotFound || err == service.ErrInvalidPassword {
			response.Error(c, http.StatusUnauthorized, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(c, http.StatusOK, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

// GetProfile 获取当前用户信息
// @Summary      获取当前用户信息
// @Description  获取当前登录用户的详细信息
// @Tags         用户认证
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.APIResponse
// @Failure      401  {object}  ErrorResponse  "未授权"
// @Failure      404  {object}  ErrorResponse  "用户不存在"
// @Failure      500  {object}  ErrorResponse  "服务器错误"
// @Router       /api/v1/user [get]
func (s *Server) GetProfile(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		response.Error(c, http.StatusUnauthorized, "user not found in context")
		return
	}

	user, err := s.UserService.GetProfile(c.Request.Context(), userID.(uint))
	if err != nil {
		if err == service.ErrUserNotFound {
			response.Error(c, http.StatusNotFound, err.Error())
		} else {
			response.Error(c, http.StatusInternalServerError, err.Error())
		}
		return
	}

	response.Success(c, http.StatusOK, user)
}
