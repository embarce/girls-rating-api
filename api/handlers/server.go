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
	AuthorService        *service.AuthorService
	GachaService         *service.GachaService
	TurnstileService     *service.TurnstileService
	MailService          *service.MailService
	JWTService           *jwt.Service
	uploadAPIKey         string
}

// NewServer 创建服务器
func NewServer(userService *service.UserService, randomService *service.RandomService, imageResourceService *service.ImageResourceService, uploadService *service.UploadService, authorService *service.AuthorService, gachaService *service.GachaService, turnstileService *service.TurnstileService, mailService *service.MailService, jwtService *jwt.Service, trustedProxies []string, uploadAPIKey string) *Server {
	server := &Server{
		Engine:               gin.Default(),
		UserService:          userService,
		RandomService:        randomService,
		ImageResourceService: imageResourceService,
		UploadService:        uploadService,
		AuthorService:        authorService,
		GachaService:         gachaService,
		TurnstileService:     turnstileService,
		MailService:          mailService,
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

	// 入口页最新图片（公开，不走 JWT，供入口页高斯模糊背景）
	s.Engine.GET("/api/latest", s.Latest)

	// 抽卡接口（公开，不走 JWT，匿名 X-Device-Id 标识设备）
	gacha := s.Engine.Group("/api/gacha")
	{
		gacha.POST("/draw", s.GachaDraw)
		gacha.GET("/status", s.GachaStatus)
		gacha.GET("/collection", s.GachaCollection)
	}

	// API v1 路由组
	v1 := s.Engine.Group("/api/v1")
	{
		// 公开路由（注册/登录受 Turnstile 人机验证保护；未配置 secret 时中间件直接放行）
		public := v1.Group("")
		public.Use(middleware.Turnstile(s.TurnstileService))
		{
			public.POST("/register", s.Register)
			public.POST("/login", s.Login)
		}

		// 邮件反馈（公开 + Turnstile 人机验证，经 Resend 中继到支持邮箱）
		mail := v1.Group("/mail")
		mail.Use(middleware.Turnstile(s.TurnstileService))
		{
			mail.POST("/contact", s.MailContact)
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

		// 管理路由（固定 API Key 认证，与管理脚本同一信任域，面向爬虫自动化设计）
		admin := v1.Group("/admin")
		admin.Use(middleware.APIKeyAuth(s.uploadAPIKey))
		{
			admin.GET("/authors", s.AdminAuthors)
			admin.POST("/authors", s.AdminAuthorCreate)
			admin.GET("/authors/:id", s.AdminAuthorGet)
			admin.PATCH("/authors/:id", s.AdminAuthorUpdate)
			admin.DELETE("/authors/:id", s.AdminAuthorDelete)
			admin.POST("/authors/:id/avatar", s.AdminAuthorAvatar)
			admin.PATCH("/images/:id", s.AdminImageUpdate)
		}
	}

	// Swagger UI
	s.Engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}

// Run 启动服务器
func (s *Server) Run(port string) error {
	return s.Engine.Run(":" + port)
}
