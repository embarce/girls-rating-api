package handlers

import (
	"net/http"

	"girls-rating-api/internal/service"
	"girls-rating-api/pkg/response"

	"github.com/gin-gonic/gin"
)

// RandomAPIResponse 统一 API 响应结构
type RandomAPIResponse struct {
	Msg  string                       `json:"msg"`
	Code int                          `json:"code"`
	Data []service.RandomItemResponse `json:"data"`
}

// RandomItemResponse 单条随机图片资源响应
type RandomItemResponse struct {
	ResourceURL string         `json:"resourceUrl"`
	Width       int            `json:"width"`
	Height      int            `json:"height"`
	Rating      int            `json:"rating"`
	Views       string         `json:"views"`
	Author      AuthorResponse `json:"author"`
}

// AuthorResponse 作者信息
type AuthorResponse struct {
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

// Random 返回随机图片资源（公开接口，不需要 JWT）
// @Summary      获取随机图片资源
// @Description  随机返回图片资源列表，无需认证
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
