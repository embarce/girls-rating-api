package middleware

import (
	"net/http"

	"girls-rating-api/pkg/response"

	"github.com/gin-gonic/gin"
)

// APIKeyAuth 固定 API Key 认证中间件。
// 请求头携带 X-API-Key: <key>，与预配置的 key 比对。
// key 为空字符串时拒绝所有请求（未配置 key 视为禁用上传）。
func APIKeyAuth(validKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if validKey == "" {
			response.Error(c, http.StatusForbidden, "upload API key not configured")
			c.Abort()
			return
		}

		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			response.Error(c, http.StatusUnauthorized, "missing X-API-Key header")
			c.Abort()
			return
		}

		if apiKey != validKey {
			response.Error(c, http.StatusUnauthorized, "invalid API key")
			c.Abort()
			return
		}

		c.Next()
	}
}
