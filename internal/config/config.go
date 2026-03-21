package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config 应用配置结构
type Config struct {
	App   AppConfig
	MySQL MySQLConfig
	Redis RedisConfig
	JWT   JWTConfig
}

// AppConfig 服务器配置
type AppConfig struct {
	Port string
	Env  string
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
			Port: getEnv("APP_PORT", "8080"),
			Env:  getEnv("APP_ENV", "development"),
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
	}

	// 设置默认值
	if config.Redis.DB == 0 {
		config.Redis.DB = viper.GetInt("REDIS_DB")
	}
	if config.JWT.Expire == 0 {
		config.JWT.Expire = 24 * time.Hour
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
