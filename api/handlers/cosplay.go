package handlers

import (
	"fmt"
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
	TotalCount int64                 `json:"totalCount"`
	TotalPage  int64                 `json:"totalPage"`
	PageSize   int                   `json:"pageSize"`
	CurrPage   int                   `json:"currPage"`
	List       []CosplayItemResponse `json:"list"`
}

// CosplayItemResponse 单条 Cosplay 图片资源响应
type CosplayItemResponse struct {
	ID          string         `json:"id"`
	ResourceURL string         `json:"resourceUrl"`
	Width       int            `json:"width"`
	Height      int            `json:"height"`
	Rating      int            `json:"rating"`
	Views       string         `json:"views"`
	Author      AuthorResponse `json:"author"`
}

// Cosplay 分页查询 Cosplay 图片资源
// @Summary      分页查询 Cosplay 图片资源
// @Description  按时间倒序分页返回图片资源列表
// @Tags         图片资源
// @Produce      json
// @Param        page     query     int  false  "页码，从 1 开始"  default(1)
// @Param        pageSize query     int  false  "每页数量"  default(10)
// @Success      200      {object}  CosplayListResponse
// @Failure      500      {object}  ErrorResponse  "服务器错误"
// @Router       /api/cosplay [get]
func (s *Server) Cosplay(c *gin.Context) {
	// 解析分页参数
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("pageSize", "10")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	// 查询数据
	result, err := s.ImageResourceService.List(c.Request.Context(), service.ListInput{
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	// 转换响应格式
	list := make([]CosplayItemResponse, 0, len(result.List))
	for _, item := range result.List {
		list = append(list, CosplayItemResponse{
			ID:          fmt.Sprintf("%d", item.ID),
			ResourceURL: item.ResourceURL,
			Width:       item.Width,
			Height:      item.Height,
			Rating:      int(item.Rating),
			Views:       item.Views,
			Author: AuthorResponse{
				Name:   "Embrace",
				Avatar: "http://localhost:3000/images/avatars/avatar1.webp",
			},
		})
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
