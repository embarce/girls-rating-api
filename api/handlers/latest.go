package handlers

import (
	"net/http"
	"strconv"

	"girls-rating-api/internal/service"
	"girls-rating-api/pkg/response"

	"github.com/gin-gonic/gin"
)

// LatestResponse 入口页最新图片响应
type LatestResponse struct {
	Msg  string                  `json:"msg"`
	Code int                     `json:"code"`
	Data []service.PhotoResponse `json:"data"`
}

// Latest 入口页最新图片（公开接口，不需要 JWT）
// @Summary      入口页最新图片
// @Description  按入档时间倒序返回最新一批图片（默认 10 张，limit 1-20），data 直接是数组、无分页包装，供入口页高斯模糊背景使用
// @Tags         图片资源
// @Produce      json
// @Param        limit  query  int  false  "数量，1-20"  default(10)
// @Success      200    {object}  LatestResponse
// @Failure      500    {object}  response.APIResponse  "服务器错误"
// @Router       /api/latest [get]
func (s *Server) Latest(c *gin.Context) {
	limit := 10
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 20 {
			limit = n
		}
	}

	// 复用分页查询：create_time DESC，取第一页即最新 N 张；页缓存（page:1）与 /api/cosplay 共享
	result, err := s.ImageResourceService.List(c.Request.Context(), service.ListInput{
		Page:     1,
		PageSize: limit,
	})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	list := make([]service.PhotoResponse, 0, len(result.List))
	for _, item := range result.List {
		list = append(list, s.ImageResourceService.ToPhotoResponse(c.Request.Context(), item, true))
	}

	response.Success(c, http.StatusOK, list)
}
