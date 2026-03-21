package main

import (
	"log"

	apiHandlers "girls-rating-api/api/handlers"
	"girls-rating-api/internal/config"
	"girls-rating-api/internal/database"
	"girls-rating-api/internal/repository"
	"girls-rating-api/internal/service"
	"girls-rating-api/pkg/jwt"
	redisClient "girls-rating-api/pkg/redis"

	"github.com/gin-gonic/gin"

	// 必须导入以使 docs 包 init() 中 swag.Register 执行，Swagger UI 才能加载 doc.json
	_ "girls-rating-api/docs"
)

// @title           Girls Rating API
// @version         1.0
// @description     基于 Go 构建的 RESTful API 服务，支持用户注册、登录、认证等功能
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@swagger.io

// @license.name   MIT
// @license.url    https://opensource.org/licenses/MIT

// @host      localhost:8080
// @BasePath  /
// @schemes   http https

// @securityDefinitions.apikey  BearerAuth
// @in header
// @name Authorization
// @description 使用 JWT token，格式：Bearer {token}

func main() {
	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 设置 Gin 模式
	switch cfg.App.Env {
	case "production":
		gin.SetMode(gin.ReleaseMode)
	case "test":
		gin.SetMode(gin.TestMode)
	default:
		gin.SetMode(gin.DebugMode)
	}

	// 初始化数据库
	db, err := database.NewMySQL(cfg.MySQL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// 不在程序启动时自动建表/迁移数据库。
	// 你将手动维护 migrations/*.sql（或你自己的迁移脚本）来更新数据库结构。

	// 初始化 Redis
	rdb, err := redisClient.New(cfg.Redis)
	if err != nil {
		log.Fatalf("Failed to connect to redis: %v", err)
	}
	defer rdb.Close()

	// 初始化 JWT
	jwtService := jwt.NewService(jwt.Config{
		Secret: cfg.JWT.Secret,
		Expire: cfg.JWT.Expire,
		Issuer: cfg.JWT.Issuer,
	})

	// 初始化仓库和服务
	userRepo := repository.NewUserRepository(db)
	userService := service.NewUserService(userRepo, jwtService)

	// 初始化随机图片资源服务
	randomRepo := repository.NewRandomRepository(db)
	randomService := service.NewRandomService(randomRepo, rdb, cfg.Random)

	// 初始化图片资源服务
	imageResourceRepo := repository.NewImageResourceRepository(db)
	imageResourceService := service.NewImageResourceService(imageResourceRepo, rdb, cfg.Cache.PaginationTTL)

	// 创建并启动服务器
	server := apiHandlers.NewServer(userService, randomService, imageResourceService, jwtService, cfg.App.TrustedProxies)

	log.Printf("Starting server on port %s", cfg.App.Port)
	if err := server.Run(cfg.App.Port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
