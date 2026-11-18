package middleware

import (
	"net/http"

	"girls-rating-api/internal/service"
	"girls-rating-api/pkg/response"

	"github.com/gin-gonic/gin"
)

// Turnstile Cloudflare Turnstile 人机验证中间件。
// 服务端未配置 TURNSTILE_SECRET_KEY 时直接放行（功能未启用，保持向后兼容）；
// 启用后要求请求头 X-Turnstile-Token，siteverify 校验失败一律 403。
// 保护范围：用户注册 / 登录 / 邮件反馈。
func Turnstile(ts *service.TurnstileService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !ts.Enabled() {
			c.Next()
			return
		}

		token := c.GetHeader("X-Turnstile-Token")
		if token == "" {
			response.Error(c, http.StatusForbidden, "missing turnstile token")
			c.Abort()
			return
		}

		ok, err := ts.Verify(c.Request.Context(), token, c.ClientIP())
		if err != nil || !ok {
			response.Error(c, http.StatusForbidden, "turnstile verification failed")
			c.Abort()
			return
		}

		c.Next()
	}
}
