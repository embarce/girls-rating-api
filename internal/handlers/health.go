package handlers

import (
	"github.com/gin-gonic/gin"
)

// HealthResponse 健康检查响应
type HealthResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Health 健康检查处理器
func Health(c *gin.Context) {
	c.JSON(200, HealthResponse{
		Code:    200,
		Message: "ok",
	})
}
