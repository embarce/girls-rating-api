package handlers

import (
	"girls-rating-api/internal/middleware"
	"girls-rating-api/internal/service"
	"girls-rating-api/pkg/jwt"

	"github.com/gin-gonic/gin"
)

// Server HTTP 服务器
type Server struct {
	Engine      *gin.Engine
	UserService *service.UserService
	JWTService  *jwt.Service
}

// NewServer 创建服务器
func NewServer(userService *service.UserService, jwtService *jwt.Service) *Server {
	server := &Server{
		Engine:      gin.Default(),
		UserService: userService,
		JWTService:  jwtService,
	}

	server.setupRouter()
	return server
}

// setupRouter 设置路由
func (s *Server) setupRouter() {
	// 健康检查
	s.Engine.GET("/health", Health)

	// API v1 路由组
	v1 := s.Engine.Group("/api/v1")
	{
		// 公开路由
		public := v1.Group("")
		{
			public.POST("/register", s.Register)
			public.POST("/login", s.Login)
		}

		// 需要认证的路由
		protected := v1.Group("")
		protected.Use(middleware.Auth(s.JWTService))
		{
			protected.GET("/user", s.GetProfile)
		}
	}
}

// Run 启动服务器
func (s *Server) Run(port string) error {
	return s.Engine.Run(":" + port)
}
