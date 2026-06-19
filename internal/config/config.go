package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config 应用配置结构
type Config struct {
	App    AppConfig
	MySQL  MySQLConfig
	Redis  RedisConfig
	JWT    JWTConfig
	Cache  CacheConfig
	Random RandomConfig
	R2     R2Config
}

// AppConfig 服务器配置
type AppConfig struct {
	Port           string
	Env            string
	TrustedProxies []string // 用于 Gin SetTrustedProxies，逗号分隔配置项：GIN_TRUSTED_PROXIES
	S3Host         string   // S3 图片资源 host，如 https://static.girls-rating.com
	UploadAPIKey   string   // 上传接口固定 API Key，通过 X-API-Key 请求头传递
}

// MySQLConfig 数据库配置
type MySQLConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Database string
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

// JWTConfig JWT 配置
type JWTConfig struct {
	Secret string
	Expire time.Duration
	Issuer string
}

// CacheConfig 缓存配置
type CacheConfig struct {
	// PaginationTTL 分页接口缓存 TTL
	PaginationTTL time.Duration
}

// R2Config Cloudflare R2 配置（S3 兼容）
type R2Config struct {
	AccountID string // Cloudflare 账户 ID
	AccessKey string // R2 Access Key ID
	SecretKey string // R2 Secret Access Key
	Bucket    string // R2 Bucket 名称
	PublicURL string // 公开访问 URL，复用 S3_HOST
}

// RandomConfig 随机接口缓存配置
type RandomConfig struct {
	// PoolSize 随机池大小（Redis 中预存多少条数据）
	PoolSize int
	// RefreshInterval 池刷新间隔
	RefreshInterval time.Duration
	// PoolLockTTL 刷新锁 TTL
	PoolLockTTL time.Duration
	// PoolSetKey Redis Set key（存储 JSON 序列化后的 ImageResourceRow）
	PoolSetKey string
}

// Load 加载配置文件
func Load() (*Config, error) {
	// 设置配置名称和路径
	viper.SetConfigName(".env")
	viper.SetConfigType("env")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./")

	// 读取环境变量作为备选
	viper.AutomaticEnv()

	// 尝试读取配置文件，如果不存在也不报错
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	config := &Config{
		App: AppConfig{
			Port:           getEnv("APP_PORT", "8080"),
			Env:            getEnv("APP_ENV", "development"),
			TrustedProxies: parseCSV(getEnv("GIN_TRUSTED_PROXIES", "")),
			S3Host:         getEnv("S3_HOST", ""),
			UploadAPIKey:   getEnv("UPLOAD_API_KEY", ""),
		},
		MySQL: MySQLConfig{
			Host:     getEnv("MYSQL_HOST", "localhost"),
			Port:     getEnv("MYSQL_PORT", "3306"),
			User:     getEnv("MYSQL_USER", "root"),
			Password: getEnv("MYSQL_PASSWORD", ""),
			Database: getEnv("MYSQL_DATABASE", "girls_rating"),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnv("REDIS_PORT", "6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       viper.GetInt("REDIS_DB"),
		},
		JWT: JWTConfig{
			Secret: getEnv("JWT_SECRET", "change-this-secret"),
			Expire: time.Duration(viper.GetInt("JWT_EXPIRE")) * time.Hour,
			Issuer: getEnv("JWT_ISSUER", "girls-rating-api"),
		},
		Cache: CacheConfig{
			PaginationTTL: time.Duration(viper.GetInt("PAGE_CACHE_TTL_SECONDS")) * time.Second,
		},
		Random: RandomConfig{
			PoolSize:        viper.GetInt("RANDOM_POOL_SIZE"),
			RefreshInterval: time.Duration(viper.GetInt("RANDOM_POOL_REFRESH_SECONDS")) * time.Second,
			PoolLockTTL:     time.Duration(viper.GetInt("RANDOM_POOL_LOCK_SECONDS")) * time.Second,
			PoolSetKey:      getEnv("RANDOM_POOL_SET_KEY", "cache:random_pool:v1:set"),
		},
		R2: R2Config{
			AccountID: getEnv("R2_ACCOUNT_ID", ""),
			AccessKey: getEnv("R2_ACCESS_KEY", ""),
			SecretKey: getEnv("R2_SECRET_KEY", ""),
			Bucket:    getEnv("R2_BUCKET", ""),
			PublicURL: getEnv("S3_HOST", ""),
		},
	}

	// 设置默认值
	if config.Redis.DB == 0 {
		config.Redis.DB = viper.GetInt("REDIS_DB")
	}
	if config.JWT.Expire == 0 {
		config.JWT.Expire = 24 * time.Hour
	}
	if config.Cache.PaginationTTL == 0 {
		config.Cache.PaginationTTL = 5 * time.Minute
	}
	if config.Random.PoolSize == 0 {
		// 默认预存 2 万条数据（按你库的实际规模可调）
		config.Random.PoolSize = 20000
	}
	if config.Random.RefreshInterval == 0 {
		config.Random.RefreshInterval = 1 * time.Hour
	}
	if config.Random.PoolLockTTL == 0 {
		config.Random.PoolLockTTL = 5 * time.Minute
	}

	return config, nil
}

// getEnv 获取环境变量，如果不存在则返回默认值
func getEnv(key, defaultValue string) string {
	value := viper.GetString(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// parseCSV 将形如 "a,b,c"（可带空格）解析成 []string{"a","b","c"}。
// 空字符串返回空切片。
func parseCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return []string{}
	}

	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// MySQLDSN 获取 MySQL DSN 连接字符串
func (c *MySQLConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.User,
		c.Password,
		c.Host,
		c.Port,
		c.Database,
	)
}

// RedisAddr 获取 Redis 地址
func (c *RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%s", c.Host, c.Port)
}

// Endpoint 获取 R2 S3 兼容 endpoint
func (c *R2Config) Endpoint() string {
	return fmt.Sprintf("https://%s.r2.cloudflarestorage.com", c.AccountID)
}
