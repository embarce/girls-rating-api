package handlers

import (
	"net/http"

	"girls-rating-api/internal/service"
	"girls-rating-api/pkg/response"

	"github.com/gin-gonic/gin"
)

// RandomAPIResponse 统一 API 响应结构
type RandomAPIResponse struct {
	Msg  string                  `json:"msg"`
	Code int                     `json:"code"`
	Data []service.PhotoResponse `json:"data"`
}

// Random 返回随机图片资源（公开接口，不需要 JWT）
// @Summary      获取随机图片资源
// @Description  随机返回图片资源列表（统一 PhotoResponse：rating 浮点、rarity 卡片等级、作者来自 authors 表），无需认证
// @Tags         图片资源
// @Produce      json
// @Success      200  {object}  RandomAPIResponse
// @Failure      500  {object}  response.APIResponse  "服务器错误"
// @Router       /api/random [get]
func (s *Server) Random(c *gin.Context) {
	items, err := s.RandomService.RandomResources(c.Request.Context(), 100)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "failed")
		return
	}

	response.Success(c, http.StatusOK, items)
}
