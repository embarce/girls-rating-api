package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config 应用配置结构
type Config struct {
	App       AppConfig
	MySQL     MySQLConfig
	Redis     RedisConfig
	JWT       JWTConfig
	Cache     CacheConfig
	Random    RandomConfig
	Photo     PhotoConfig
	Gacha     GachaConfig
	Turnstile TurnstileConfig
	Mail      MailConfig
	R2        R2Config
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
	// PoolSetKey Redis Set key（存储 JSON 序列化后的 PhotoRow）
	// 结构升级时必须 bump 版本（如 v1 → v2），避免新旧 JSON 混池。
	PoolSetKey string
}

// PhotoConfig 照片领域配置（评分定档阈值权威默认值见 docs/design/photo-system.md §四）
type PhotoConfig struct {
	// SSRMinRating rating >= 此值派生为 SSR（与 002 迁移回填 SQL 同源）
	SSRMinRating float64
	// SRMinRating rating >= 此值派生为 SR
	SRMinRating float64
	// DefaultAuthorID 默认作者 id（seed 的 id=1），作者解析 miss 时回退
	DefaultAuthorID int64
}

// GachaConfig 抽卡配置（docs/design/gacha-system.md §四.3）
type GachaConfig struct {
	// DrawSize 每日一抽固定张数（拍板固定 10）
	DrawSize int
	// SSRWeight / SRWeight / RWeight 固定权重（先掷档、再从档内池取图）
	SSRWeight float64
	SRWeight  float64
	RWeight   float64
	// Timezone draw_date 按此时区取服务器日期
	Timezone string
	// PoolSize 每档抽卡池上限
	PoolSize int
	// RefreshInterval 池刷新间隔
	RefreshInterval time.Duration
	// PoolLockTTL 刷新锁 TTL
	PoolLockTTL time.Duration
	// PoolSetKeyPrefix 抽卡池 key 前缀，实际 key 为 {prefix}:{ssr|sr|r}
	PoolSetKeyPrefix string
}

// TurnstileConfig Cloudflare Turnstile 人机验证。
// SecretKey 为空表示未启用：抽卡接口跳过验证，保持向后兼容。
type TurnstileConfig struct {
	SecretKey string // TURNSTILE_SECRET_KEY
	VerifyURL string // siteverify 接口地址
}

// MailConfig Resend 邮件发送（https://resend.com）。
// ResendAPIKey 为空表示未配置；反馈中继额外要求 ContactTo（收件邮箱）。
type MailConfig struct {
	ResendAPIKey string // RESEND_API_KEY
	From         string // 发件人，"Name <addr>" 或纯地址，需为 Resend 已验证域名
	ContactTo    string // 反馈中继收件邮箱（MAIL_CONTACT_TO，支持团队收件箱）
	APIURL       string // Resend 发信接口地址
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
			// v2：池成员结构升级为 PhotoRow（含 rarity/authorId），旧 v1 key 弃用
			PoolSetKey: getEnv("RANDOM_POOL_SET_KEY", "cache:random_pool:v2:set"),
		},
		Photo: PhotoConfig{
			SSRMinRating:    viper.GetFloat64("PHOTO_SSR_MIN_RATING"),
			SRMinRating:     viper.GetFloat64("PHOTO_SR_MIN_RATING"),
			DefaultAuthorID: int64(viper.GetInt("PHOTO_DEFAULT_AUTHOR_ID")),
		},
		Gacha: GachaConfig{
			DrawSize:         viper.GetInt("GACHA_DRAW_SIZE"),
			SSRWeight:        viper.GetFloat64("GACHA_SSR_WEIGHT"),
			SRWeight:         viper.GetFloat64("GACHA_SR_WEIGHT"),
			RWeight:          viper.GetFloat64("GACHA_R_WEIGHT"),
			Timezone:         getEnv("GACHA_TIMEZONE", "Asia/Shanghai"),
			PoolSize:         viper.GetInt("GACHA_POOL_SIZE"),
			RefreshInterval:  time.Duration(viper.GetInt("GACHA_POOL_REFRESH_SECONDS")) * time.Second,
			PoolLockTTL:      time.Duration(viper.GetInt("GACHA_POOL_LOCK_SECONDS")) * time.Second,
			PoolSetKeyPrefix: getEnv("GACHA_POOL_SET_KEY_PREFIX", "cache:gacha_pool:v2"),
		},
		Turnstile: TurnstileConfig{
			SecretKey: getEnv("TURNSTILE_SECRET_KEY", ""),
			VerifyURL: getEnv("TURNSTILE_VERIFY_URL", "https://challenges.cloudflare.com/turnstile/v0/siteverify"),
		},
		Mail: MailConfig{
			ResendAPIKey: getEnv("RESEND_API_KEY", ""),
			From:         getEnv("MAIL_FROM", ""),
			ContactTo:    getEnv("MAIL_CONTACT_TO", ""),
			APIURL:       getEnv("RESEND_API_URL", "https://api.resend.com/emails"),
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
	if config.Photo.SSRMinRating == 0 {
		config.Photo.SSRMinRating = 9.0
	}
	if config.Photo.SRMinRating == 0 {
		config.Photo.SRMinRating = 7.0
	}
	if config.Photo.DefaultAuthorID == 0 {
		config.Photo.DefaultAuthorID = 1
	}
	if config.Gacha.DrawSize == 0 {
		config.Gacha.DrawSize = 10
	}
	if config.Gacha.SSRWeight == 0 {
		config.Gacha.SSRWeight = 6
	}
	if config.Gacha.SRWeight == 0 {
		config.Gacha.SRWeight = 24
	}
	if config.Gacha.RWeight == 0 {
		config.Gacha.RWeight = 70
	}
	if _, err := time.LoadLocation(config.Gacha.Timezone); err != nil {
		// 时区名非法时回退默认值，避免 draw_date 计算 panic
		config.Gacha.Timezone = "Asia/Shanghai"
	}
	if config.Gacha.PoolSize == 0 {
		config.Gacha.PoolSize = 500
	}
	if config.Gacha.RefreshInterval == 0 {
		config.Gacha.RefreshInterval = 1 * time.Hour
	}
	if config.Gacha.PoolLockTTL == 0 {
		config.Gacha.PoolLockTTL = 5 * time.Minute
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
