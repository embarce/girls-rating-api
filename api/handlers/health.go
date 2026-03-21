package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"girls-rating-api/pkg/response"
)

// Health 健康检查处理器
// @Summary      健康检查
// @Description  检查 API 服务是否正常运行
// @Tags         Health
// @Produce      json
// @Success      200  {object}  response.APIResponse
// @Router       /health [get]
func Health(c *gin.Context) {
	response.Success(c, http.StatusOK, gin.H{
		"message": "ok",
	})
}
