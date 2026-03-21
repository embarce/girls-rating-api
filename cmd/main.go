package main

import (
	"log"

	"girls-rating-api/internal/config"
	"girls-rating-api/internal/database"
	"girls-rating-api/internal/handlers"
	"girls-rating-api/internal/repository"
	"girls-rating-api/internal/service"
	"girls-rating-api/pkg/jwt"
	redisClient "girls-rating-api/pkg/redis"
)

func main() {
	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 初始化数据库
	db, err := database.NewMySQL(cfg.MySQL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// 自动迁移模型
	if err := database.AutoMigrate(db); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

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

	// 创建并启动服务器
	server := handlers.NewServer(userService, jwtService)

	log.Printf("Starting server on port %s", cfg.App.Port)
	if err := server.Run(cfg.App.Port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
