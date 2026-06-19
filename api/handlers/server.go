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
	UploadService        *service.UploadService
	JWTService           *jwt.Service
	uploadAPIKey         string
}

// NewServer 创建服务器
func NewServer(userService *service.UserService, randomService *service.RandomService, imageResourceService *service.ImageResourceService, uploadService *service.UploadService, jwtService *jwt.Service, trustedProxies []string, uploadAPIKey string) *Server {
	server := &Server{
		Engine:               gin.Default(),
		UserService:          userService,
		RandomService:        randomService,
		ImageResourceService: imageResourceService,
		UploadService:        uploadService,
		JWTService:           jwtService,
		uploadAPIKey:         uploadAPIKey,
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

		// 需要 JWT 认证的路由
		protected := v1.Group("")
		protected.Use(middleware.Auth(s.JWTService))
		{
			protected.GET("/user", s.GetProfile)
		}

		// 上传路由（固定 API Key 认证，供维护脚本使用）
		upload := v1.Group("/upload")
		upload.Use(middleware.APIKeyAuth(s.uploadAPIKey))
		{
			upload.POST("/image", s.UploadImage)
		}
	}

	// Swagger UI
	s.Engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}

// Run 启动服务器
func (s *Server) Run(port string) error {
	return s.Engine.Run(":" + port)
}
