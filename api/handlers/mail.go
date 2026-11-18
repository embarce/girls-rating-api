package handlers

import (
	"net/http"

	"girls-rating-api/pkg/response"

	"github.com/gin-gonic/gin"
)

// MailContactRequest 用户反馈请求（经 Turnstile 人机验证后由 Resend 中继到支持邮箱）
type MailContactRequest struct {
	Email   string `json:"email" binding:"required,email"`
	Name    string `json:"name"`
	Message string `json:"message" binding:"required"`
}

// MailContact 用户反馈邮件
// @Summary      用户反馈邮件（Resend 中继）
// @Description  将用户反馈转发到支持邮箱（MAIL_CONTACT_TO），Reply-To 指向用户邮箱。请求头携带 X-Turnstile-Token（服务端启用人机验证时必填）
// @Tags         邮件
// @Accept       json
// @Produce      json
// @Param        X-Turnstile-Token  header  string              false "Turnstile 人机验证 token（启用时必填）"
// @Param        request            body    MailContactRequest  true  "反馈内容"
// @Success      200  {object}  response.APIResponse  "data: {}"
// @Failure      400  {object}  response.APIResponse  "请求参数错误"
// @Failure      403  {object}  response.APIResponse  "人机验证失败"
// @Failure      502  {object}  response.APIResponse  "Resend 上游错误"
// @Failure      503  {object}  response.APIResponse  "邮件未配置"
// @Router       /api/v1/mail/contact [post]
func (s *Server) MailContact(c *gin.Context) {
	var req MailContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	if len(req.Message) > 5000 {
		response.Error(c, http.StatusBadRequest, "message too long (max 5000)")
		return
	}
	if len(req.Name) > 100 {
		response.Error(c, http.StatusBadRequest, "name too long (max 100)")
		return
	}

	if !s.MailService.CanContact() {
		response.Error(c, http.StatusServiceUnavailable, "mail not configured")
		return
	}

	if _, err := s.MailService.SendContact(c.Request.Context(), req.Email, req.Name, req.Message); err != nil {
		response.Error(c, http.StatusBadGateway, err.Error())
		return
	}

	response.StatusOK(c, gin.H{})
}
