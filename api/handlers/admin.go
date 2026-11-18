package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"girls-rating-api/internal/models"
	"girls-rating-api/internal/repository"
	"girls-rating-api/internal/service"
	"girls-rating-api/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AdminAuthorResponse 管理接口作者响应（统一包裹，existed 仅幂等创建时返回）。
type AdminAuthorResponse struct {
	Author  *models.Author `json:"author"`
	Existed bool           `json:"existed,omitempty"`
}

// AdminAuthorAffectedResponse 删除作者响应（带受影响图片计数）。
type AdminAuthorAffectedResponse struct {
	AffectedImages int64 `json:"affectedImages"`
}

// AdminAuthorCreateRequest 创建作者请求（面向爬虫自动化：幂等创建）。
type AdminAuthorCreateRequest struct {
	Name   string `json:"name" binding:"required"`
	Avatar string `json:"avatar"`
}

// AdminAuthorUpdateRequest 更新作者请求（指针字段：nil 跳过，显式空串可清空 avatar）。
type AdminAuthorUpdateRequest struct {
	Name   *string `json:"name"`
	Avatar *string `json:"avatar"`
}

// AdminImageUpdateRequest 图片改档请求（指针字段：nil 跳过）。
type AdminImageUpdateRequest struct {
	Rating   *float64 `json:"rating"`
	Rarity   *int8    `json:"rarity"` // 0=R 1=SR 2=SSR；nil 且 rating 有变时按新 rating 派生
	Views    *string  `json:"views"`
	AuthorID *int64   `json:"author_id"`
}

// AdminAuthors 管理接口：作者列表 / 存在性检查。
// @Summary      作者列表（作者存在性检查入口）
// @Description  name 精确匹配（命中返回单条，供爬虫先查后建）；keyword 模糊分页列表
// @Tags         管理端-作者
// @Produce      json
// @Security     ApiKeyAuth
// @Param        name     query     string  false  "精确匹配作者名（存在性检查）"
// @Param        keyword  query     string  false  "模糊匹配作者名"
// @Param        page     query     int     false  "页码，从 1 开始"  default(1)
// @Param        pageSize query     int     false  "每页数量"  default(20)
// @Success      200  {object}  response.APIResponse  "data: { list: [Author], total }"
// @Failure      401  {object}  response.APIResponse  "未授权：缺少或无效的 X-API-Key"
// @Failure      403  {object}  response.APIResponse  "禁止：未配置 API Key"
// @Router       /api/v1/admin/authors [get]
func (s *Server) AdminAuthors(c *gin.Context) {
	page, pageSize := parsePagination(c, 20)
	name := c.Query("name")
	keyword := c.Query("keyword")

	result, err := s.AuthorService.AdminList(c.Request.Context(), name, keyword, page, pageSize)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(c, http.StatusOK, gin.H{
		"list":     result.List,
		"total":    result.Total,
		"page":     page,
		"pageSize": pageSize,
	})
}

// AdminAuthorCreate 管理接口：幂等创建作者。
// @Summary      创建作者（幂等）
// @Description  name 已存在时返回 200 + 既有记录 + existed:true；新建返回 201。爬虫崩溃重试不会产生重复
// @Tags         管理端-作者
// @Accept       json
// @Produce      json
// @Security     ApiKeyAuth
// @Param        body body AdminAuthorCreateRequest true "作者信息"
// @Success      201  {object}  response.APIResponse  "data: { author, existed:false }"
// @Success      200  {object}  response.APIResponse  "data: { author, existed:true }"
// @Failure      400  {object}  response.APIResponse  "请求参数错误"
// @Failure      401  {object}  response.APIResponse  "未授权"
// @Failure      403  {object}  response.APIResponse  "禁止"
// @Router       /api/v1/admin/authors [post]
func (s *Server) AdminAuthorCreate(c *gin.Context) {
	var req AdminAuthorCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	author, existed, err := s.AuthorService.AdminCreate(c.Request.Context(), req.Name, req.Avatar)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	code := http.StatusCreated
	if existed {
		code = http.StatusOK
	}
	response.Success(c, code, AdminAuthorResponse{Author: author, Existed: existed})
}

// AdminAuthorGet 管理接口：作者详情。
// @Summary      作者详情
// @Description  不存在或已软删返回 404
// @Tags         管理端-作者
// @Produce      json
// @Security     ApiKeyAuth
// @Param        id path int true "作者 ID"
// @Success      200  {object}  response.APIResponse  "data: { author }"
// @Failure      401  {object}  response.APIResponse  "未授权"
// @Failure      404  {object}  response.APIResponse  "作者不存在"
// @Router       /api/v1/admin/authors/{id} [get]
func (s *Server) AdminAuthorGet(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}

	author, err := s.AuthorService.AdminGet(c.Request.Context(), id)
	if err != nil {
		mapAuthorServiceError(c, err)
		return
	}

	response.Success(c, http.StatusOK, AdminAuthorResponse{Author: author})
}

// AdminAuthorUpdate 管理接口：更新作者 name / avatar。
// @Summary      更新作者
// @Description  改名撞唯一索引返回 409
// @Tags         管理端-作者
// @Accept       json
// @Produce      json
// @Security     ApiKeyAuth
// @Param        id   path int                          true "作者 ID"
// @Param        body body AdminAuthorUpdateRequest     true "更新内容（nil 字段跳过）"
// @Success      200  {object}  response.APIResponse  "data: { author }"
// @Failure      400  {object}  response.APIResponse  "请求参数错误"
// @Failure      401  {object}  response.APIResponse  "未授权"
// @Failure      404  {object}  response.APIResponse  "作者不存在"
// @Failure      409  {object}  response.APIResponse  "名称冲突"
// @Router       /api/v1/admin/authors/{id} [patch]
func (s *Server) AdminAuthorUpdate(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}

	var req AdminAuthorUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	author, err := s.AuthorService.AdminUpdate(c.Request.Context(), id, req.Name, req.Avatar)
	if err != nil {
		if repository.IsDuplicateEntryError(err) {
			response.Error(c, http.StatusConflict, "author name conflict")
			return
		}
		mapAuthorServiceError(c, err)
		return
	}

	response.Success(c, http.StatusOK, AdminAuthorResponse{Author: author})
}

// AdminAuthorDelete 管理接口：软删除作者。
// @Summary      删除作者（软删除）
// @Description  同事务把 name 改写为 "{name}#del#{id}" 释放唯一槽位，允许重建同名；响应带 affectedImages 计数
// @Tags         管理端-作者
// @Produce      json
// @Security     ApiKeyAuth
// @Param        id path int true "作者 ID"
// @Success      200  {object}  response.APIResponse  "data: { affectedImages }"
// @Failure      401  {object}  response.APIResponse  "未授权"
// @Failure      404  {object}  response.APIResponse  "作者不存在"
// @Router       /api/v1/admin/authors/{id} [delete]
func (s *Server) AdminAuthorDelete(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}

	affected, err := s.AuthorService.AdminDelete(c.Request.Context(), id)
	if err != nil {
		mapAuthorServiceError(c, err)
		return
	}

	response.Success(c, http.StatusOK, AdminAuthorAffectedResponse{AffectedImages: affected})
}

// AdminAuthorAvatar 管理接口：上传作者头像到 R2 avatars/ 前缀。
// @Summary      上传作者头像
// @Description  multipart 头像文件 → R2 avatars/ 前缀 → 更新 URL + 失效作者缓存
// @Tags         管理端-作者
// @Accept       multipart/form-data
// @Produce      json
// @Security     ApiKeyAuth
// @Param        id   path int     true "作者 ID"
// @Param        file formData file true "头像文件 (JPEG/PNG/WebP/GIF)"
// @Success      200  {object}  response.APIResponse  "data: { author }"
// @Failure      400  {object}  response.APIResponse  "请求参数错误"
// @Failure      401  {object}  response.APIResponse  "未授权"
// @Failure      404  {object}  response.APIResponse  "作者不存在"
// @Failure      500  {object}  response.APIResponse  "服务器错误"
// @Router       /api/v1/admin/authors/{id}/avatar [post]
func (s *Server) AdminAuthorAvatar(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if contentType == "" || !allowedContentTypes[contentType] {
		response.Error(c, http.StatusBadRequest, "unsupported file type, allowed: jpeg, png, webp, gif")
		return
	}

	avatarURL, err := s.UploadService.UploadAvatar(c.Request.Context(), header.Filename, contentType, file)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	author, err := s.AuthorService.AdminUpdateAvatar(c.Request.Context(), id, avatarURL)
	if err != nil {
		mapAuthorServiceError(c, err)
		return
	}

	response.Success(c, http.StatusOK, AdminAuthorResponse{Author: author})
}

// AdminImageUpdate 管理接口：改图（rating/rarity/views/author_id）。
// @Summary      修改图片资源（重算定档）
// @Description  改 rating/rarity/views/author_id：重算定档落库、清分页缓存、同步随机池与抽卡池
// @Tags         管理端-图片
// @Accept       json
// @Produce      json
// @Security     ApiKeyAuth
// @Param        id   path int                       true "图片 ID"
// @Param        body body AdminImageUpdateRequest    true "更新内容（nil 字段跳过）"
// @Success      200  {object}  response.APIResponse  "data: PhotoResponse"
// @Failure      400  {object}  response.APIResponse  "请求参数错误"
// @Failure      401  {object}  response.APIResponse  "未授权"
// @Failure      404  {object}  response.APIResponse  "图片不存在"
// @Router       /api/v1/admin/images/{id} [patch]
func (s *Server) AdminImageUpdate(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, http.StatusBadRequest, "invalid image id")
		return
	}

	var req AdminImageUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	input := service.AdminImageUpdateInput{
		Rating:   req.Rating,
		Rarity:   req.Rarity,
		Views:    req.Views,
		AuthorID: req.AuthorID,
	}

	item, err := s.ImageResourceService.AdminUpdateImage(c.Request.Context(), id, input)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Error(c, http.StatusNotFound, "image not found")
			return
		}
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	// 池同步：SADD 新成员（PhotoRow v2）；旧成员由池定时刷新 RENAME 全量替换兜底
	row := service.PhotoRowFromImage(item)
	_ = s.RandomService.AddToPool(c.Request.Context(), row)
	_ = s.GachaService.AddToPool(c.Request.Context(), row)

	response.Success(c, http.StatusOK, s.ImageResourceService.ToPhotoResponse(c.Request.Context(), *item, true))
}

// mapAuthorServiceError 作者服务错误 → HTTP 语义（404 / 500）。
func mapAuthorServiceError(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.Error(c, http.StatusNotFound, "author not found")
		return
	}
	response.Error(c, http.StatusInternalServerError, err.Error())
}

// parsePathID 解析路径参数 id。
func parsePathID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		response.Error(c, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

// parsePagination 解析分页参数（默认值与上限由调用方给定）。
func parsePagination(c *gin.Context, defaultPageSize int) (int, int) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	pageSize, err := strconv.Atoi(c.DefaultQuery("pageSize", strconv.Itoa(defaultPageSize)))
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = defaultPageSize
	}
	return page, pageSize
}
