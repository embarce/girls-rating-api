package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"time"

	"girls-rating-api/internal/config"
)

// MailService Resend 邮件发送（https://resend.com）。
// RESEND_API_KEY 未配置时视为未启用，调用方按各自语义降级（反馈接口返回 503）。
type MailService struct {
	apiKey    string
	from      string
	contactTo string
	apiURL    string
	client    *http.Client
}

func NewMailService(cfg config.MailConfig) *MailService {
	return &MailService{
		apiKey:    cfg.ResendAPIKey,
		from:      cfg.From,
		contactTo: cfg.ContactTo,
		apiURL:    cfg.APIURL,
		client:    &http.Client{Timeout: 15 * time.Second},
	}
}

// Enabled 是否具备发信能力（RESEND_API_KEY + MAIL_FROM 均已配置）。
func (s *MailService) Enabled() bool {
	return s != nil && s.apiKey != "" && s.from != ""
}

// CanContact 反馈中继是否可用（额外要求配置收件邮箱 MAIL_CONTACT_TO）。
func (s *MailService) CanContact() bool {
	return s.Enabled() && s.contactTo != ""
}

type resendEmailRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	ReplyTo string   `json:"reply_to,omitempty"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html,omitempty"`
	Text    string   `json:"text,omitempty"`
}

type resendEmailResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

// Send 通过 Resend 发送邮件，返回邮件 id。上游非 200 时返回携带 Resend message 的错误。
func (s *MailService) Send(ctx context.Context, to []string, replyTo, subject, htmlBody, text string) (string, error) {
	if !s.Enabled() {
		return "", fmt.Errorf("mail not configured")
	}
	if len(to) == 0 {
		return "", fmt.Errorf("invalid recipients")
	}

	payload := resendEmailRequest{
		From:    s.from,
		To:      to,
		ReplyTo: replyTo,
		Subject: subject,
		HTML:    htmlBody,
		Text:    text,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiURL, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result resendEmailResponse
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if resp.StatusCode != http.StatusOK {
		msg := result.Message
		if msg == "" {
			msg = fmt.Sprintf("resend http %d", resp.StatusCode)
		}
		return "", fmt.Errorf("resend: %s", msg)
	}
	return result.ID, nil
}

// SendContact 用户反馈中继：把反馈正文（HTML 转义后拼信）发到支持邮箱，Reply-To 指向用户邮箱。
func (s *MailService) SendContact(ctx context.Context, senderEmail, senderName, message string) (string, error) {
	if !s.CanContact() {
		return "", fmt.Errorf("mail not configured")
	}

	name := senderName
	if name == "" {
		name = "匿名用户"
	}

	// 用户输入全部转义后再拼 HTML，避免注入
	safeName := html.EscapeString(name)
	safeEmail := html.EscapeString(senderEmail)
	safeMessage := html.EscapeString(message)

	subject := "[Girls Rating] 用户反馈"
	htmlBody := fmt.Sprintf(
		`<p><strong>%s</strong>（%s）通过支持表单留言：</p><p style="white-space:pre-wrap;">%s</p>`,
		safeName,
		safeEmail,
		safeMessage,
	)
	text := fmt.Sprintf("%s（%s）:\n%s", name, senderEmail, message)

	return s.Send(ctx, []string{s.contactTo}, senderEmail, subject, htmlBody, text)
}
