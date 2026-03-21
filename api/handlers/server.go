package handlers

import (
	"girls-rating-api/internal/middleware"
	"girls-rating-api/internal/service"
	"girls-rating-api/pkg/jwt"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Server HTTP 服务器
type Server struct {
	Engine               *gin.Engine
	UserService          *service.UserService
	RandomService        *service.RandomService
	ImageResourceService *service.ImageResourceService
	JWTService           *jwt.Service
}

// NewServer 创建服务器
func NewServer(userService *service.UserService, randomService *service.RandomService, imageResourceService *service.ImageResourceService, jwtService *jwt.Service, trustedProxies []string) *Server {
	server := &Server{
		Engine:               gin.Default(),
		UserService:          userService,
		RandomService:        randomService,
		ImageResourceService: imageResourceService,
		JWTService:           jwtService,
	}

	// 设置受信任的代理。
	// 由环境变量配置：GIN_TRUSTED_PROXIES（逗号分隔），值可以是单 IP 或 CIDR。
	server.Engine.SetTrustedProxies(trustedProxies)

	server.setupRouter()
	return server
}

// setupRouter 设置路由
func (s *Server) setupRouter() {
	// 健康检查
	s.Engine.GET("/health", Health)

	// 图片随机接口（公开，不走 JWT）
	s.Engine.GET("/api/random", s.Random)

	// 图片资源分页查询接口（公开，不走 JWT）
	s.Engine.GET("/api/cosplay", s.Cosplay)

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

	// Swagger UI
	s.Engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}

// Run 启动服务器
func (s *Server) Run(port string) error {
	return s.Engine.Run(":" + port)
}
