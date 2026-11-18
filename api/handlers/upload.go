package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"girls-rating-api/internal/service"
	"girls-rating-api/pkg/response"

	"github.com/gin-gonic/gin"
)

// UploadImageResponseData 上传图片响应数据（回显 rarity 与 author，供爬虫确认定档结果）
type UploadImageResponseData struct {
	ID          string                 `json:"id"`
	ResourceURL string                 `json:"resourceUrl"`
	Width       int                    `json:"width"`
	Height      int                    `json:"height"`
	Rating      float64                `json:"rating"`
	Views       string                 `json:"views"`
	Rarity      string                 `json:"rarity"`
	Author      service.AuthorResponse `json:"author"`
}

// allowedContentTypes 允许的图片 MIME 类型
var allowedContentTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/gif":  true,
}

// parseRarityForm 解析表单 rarity：接受 0/1/2 或 "R"/"SR"/"SSR"。
// 返回 (指针, ok)；value 为空 → (nil, true) 表示未指定；无法解析 → (nil, false)。
func parseRarityForm(value string) (*int8, bool) {
	if value == "" {
		return nil, true
	}
	if n, err := strconv.Atoi(value); err == nil {
		if n < 0 || n > 2 {
			return nil, false
		}
		r := int8(n)
		return &r, true
	}
	if r, ok := service.ParseRarity(value); ok {
		return &r, true
	}
	return nil, false
}

// UploadImage 上传图片到 R2 并创建图片资源
// @Summary      上传图片到 R2 并创建图片资源
// @Description  上传图片文件到 Cloudflare R2，同时在数据库中创建图片资源记录。此接口使用固定 API Key 认证，供维护脚本调用。rarity 显式定档优先，缺省按 rating 阈值派生（>=9 SSR / >=7 SR）；作者优先级 author_id > author_name(find-or-create) > 默认作者
// @Tags         图片资源
// @Accept       multipart/form-data
// @Produce      json
// @Security     ApiKeyAuth
// @Param        file          formData  file    true  "图片文件 (JPEG/PNG/WebP/GIF)"
// @Param        rating        formData  number  false "评分 (默认 4.0)"
// @Param        views         formData  string  false "浏览量 (默认 '1k')"
// @Param        author_id     formData  integer false "作者 ID（无效值回退默认作者）"
// @Param        author_name   formData  string  false "作者名（find-or-create 兜底通道，优先级低于 author_id）"
// @Param        author_avatar formData  string  false "作者头像外链（配合 author_name 创建时使用）"
// @Param        rarity        formData  string  false "卡片等级显式定档：0/1/2 或 R/SR/SSR（缺省按 rating 派生）"
// @Param        width         formData  integer false "宽度 (默认自动检测)"
// @Param        height        formData  integer false "高度 (默认自动检测)"
// @Success      201       {object}  response.APIResponse  "data: UploadImageResponseData"
// @Failure      400       {object}  ErrorResponse  "请求参数错误"
// @Failure      401       {object}  ErrorResponse  "未授权：缺少或无效的 X-API-Key"
// @Failure      403       {object}  ErrorResponse  "禁止：未配置上传 API Key"
// @Failure      500       {object}  ErrorResponse  "服务器错误"
// @Router       /api/v1/upload/image [post]
func (s *Server) UploadImage(c *gin.Context) {
	// 获取上传文件
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	// 验证文件类型
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		// 尝试从文件扩展名推断
		ext := strings.ToLower(header.Filename)
		switch {
		case strings.HasSuffix(ext, ".jpg") || strings.HasSuffix(ext, ".jpeg"):
			contentType = "image/jpeg"
		case strings.HasSuffix(ext, ".png"):
			contentType = "image/png"
		case strings.HasSuffix(ext, ".webp"):
			contentType = "image/webp"
		case strings.HasSuffix(ext, ".gif"):
			contentType = "image/gif"
		}
	}
	if !allowedContentTypes[contentType] {
		response.Error(c, http.StatusBadRequest, "unsupported file type, allowed: jpeg, png, webp, gif")
		return
	}

	// 解析可选表单字段
	var width, height int
	var rating float64
	var authorID int64

	if v := c.PostForm("width"); v != "" {
		width, _ = strconv.Atoi(v)
	}
	if v := c.PostForm("height"); v != "" {
		height, _ = strconv.Atoi(v)
	}
	if v := c.PostForm("rating"); v != "" {
		rating, _ = strconv.ParseFloat(v, 64)
	}
	if v := c.PostForm("author_id"); v != "" {
		authorID, _ = strconv.ParseInt(v, 10, 64)
	}

	// rarity 显式定档（无法解析 → 400，避免静默把 SSR 传成 R）
	rarity, ok := parseRarityForm(c.PostForm("rarity"))
	if !ok {
		response.Error(c, http.StatusBadRequest, "invalid rarity, allowed: 0/1/2 or R/SR/SSR")
		return
	}

	// 调用上传服务
	input := service.UploadInput{
		File:         file,
		Filename:     header.Filename,
		ContentType:  contentType,
		Size:         header.Size,
		Width:        width,
		Height:       height,
		Rating:       rating,
		Views:        c.PostForm("views"),
		AuthorID:     authorID,
		AuthorName:   c.PostForm("author_name"),
		AuthorAvatar: c.PostForm("author_avatar"),
		Rarity:       rarity,
	}

	resource, err := s.UploadService.Upload(c.Request.Context(), input)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(c, http.StatusCreated, UploadImageResponseData{
		ID:          strconv.FormatInt(resource.ID, 10),
		ResourceURL: resource.ResourceURL,
		Width:       resource.Width,
		Height:      resource.Height,
		Rating:      resource.Rating,
		Views:       resource.Views,
		Rarity:      service.RarityName(resource.Rarity),
		Author:      s.AuthorService.Resolve(c.Request.Context(), resource.AuthorID),
	})
}
