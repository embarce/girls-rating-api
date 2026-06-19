package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"girls-rating-api/internal/service"
	"girls-rating-api/pkg/response"

	"github.com/gin-gonic/gin"
)

// UploadImageResponse 上传图片响应
type UploadImageResponse struct {
	Msg  string                   `json:"msg"`
	Code int                      `json:"code"`
	Data *UploadImageResponseData `json:"data"`
}

// UploadImageResponseData 上传图片响应数据
type UploadImageResponseData struct {
	ID          string  `json:"id"`
	ResourceURL string  `json:"resourceUrl"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	Rating      float64 `json:"rating"`
	Views       string  `json:"views"`
}

// allowedContentTypes 允许的图片 MIME 类型
var allowedContentTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/gif":  true,
}

// UploadImage 上传图片到 R2 并创建图片资源
// @Summary      上传图片到 R2 并创建图片资源
// @Description  上传图片文件到 Cloudflare R2，同时在数据库中创建图片资源记录。此接口使用固定 API Key 认证，供维护脚本调用。
// @Tags         图片资源
// @Accept       multipart/form-data
// @Produce      json
// @Security     ApiKeyAuth
// @Param        file      formData  file    true  "图片文件 (JPEG/PNG/WebP/GIF)"
// @Param        rating    formData  number  false "评分 (默认 4.0)"
// @Param        views     formData  string  false "浏览量 (默认 '1k')"
// @Param        author_id formData  integer false "作者 ID"
// @Param        width     formData  integer false "宽度 (默认自动检测)"
// @Param        height    formData  integer false "高度 (默认自动检测)"
// @Success      201       {object}  UploadImageResponse
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
	views := c.PostForm("views")

	// 调用上传服务
	input := service.UploadInput{
		File:        file,
		Filename:    header.Filename,
		ContentType: contentType,
		Size:        header.Size,
		Width:       width,
		Height:      height,
		Rating:      rating,
		Views:       views,
		AuthorID:    authorID,
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
	})
}
