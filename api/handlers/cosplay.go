package handlers

import (
	"net/http"
	"strconv"

	"girls-rating-api/internal/service"
	"girls-rating-api/pkg/response"

	"github.com/gin-gonic/gin"
)

// CosplayListResponse 分页列表响应
type CosplayListResponse struct {
	Msg  string                   `json:"msg"`
	Code int                      `json:"code"`
	Data *CosplayListResponseData `json:"data"`
}

// CosplayListResponseData 分页数据
type CosplayListResponseData struct {
	TotalCount int64                   `json:"totalCount"`
	TotalPage  int64                   `json:"totalPage"`
	PageSize   int                     `json:"pageSize"`
	CurrPage   int                     `json:"currPage"`
	List       []service.PhotoResponse `json:"list"`
}

// Cosplay 分页查询 Cosplay 图片资源
// @Summary      分页查询 Cosplay 图片资源
// @Description  按时间倒序分页返回图片资源列表（统一 PhotoResponse：rating 浮点、rarity 卡片等级、作者来自 authors 表）；author_id 传入时按作者过滤（作者专区）
// @Tags         图片资源
// @Produce      json
// @Param        page      query     int    false  "页码，从 1 开始"  default(1)
// @Param        pageSize  query     int    false  "每页数量"  default(10)
// @Param        author_id query     int64  false  "作者 ID（authors.id，过滤该作者作品）"
// @Success      200      {object}  CosplayListResponse
// @Failure      500      {object}  ErrorResponse  "服务器错误"
// @Router       /api/cosplay [get]
func (s *Server) Cosplay(c *gin.Context) {
	// 解析分页参数
	page, pageSize := parsePagination(c, 10)

	var authorID int64
	if v := c.Query("author_id"); v != "" {
		authorID, _ = strconv.ParseInt(v, 10, 64)
	}

	// 查询数据
	result, err := s.ImageResourceService.List(c.Request.Context(), service.ListInput{
		Page:     page,
		PageSize: pageSize,
		AuthorID: authorID,
	})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	// 转换响应格式（统一 PhotoResponse，作者经作者服务解析）
	list := make([]service.PhotoResponse, 0, len(result.List))
	for _, item := range result.List {
		list = append(list, s.ImageResourceService.ToPhotoResponse(c.Request.Context(), item, true))
	}

	// 计算总页数
	totalPage := (result.Total + int64(pageSize) - 1) / int64(pageSize)
	if totalPage < 1 {
		totalPage = 1
	}

	response.Success(c, http.StatusOK, CosplayListResponseData{
		TotalCount: result.Total,
		TotalPage:  totalPage,
		PageSize:   pageSize,
		CurrPage:   page,
		List:       list,
	})
}
