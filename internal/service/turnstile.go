package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"girls-rating-api/internal/config"
)

// TurnstileService Cloudflare Turnstile 人机验证（服务端 siteverify）。
// TURNSTILE_SECRET_KEY 未配置时视为未启用：中间件直接放行，保持向后兼容。
// 保护范围：用户注册 / 登录 / 邮件反馈（docs/API.md「认证方式」）。
type TurnstileService struct {
	secret    string
	verifyURL string
	client    *http.Client
}

func NewTurnstileService(cfg config.TurnstileConfig) *TurnstileService {
	return &TurnstileService{
		secret:    cfg.SecretKey,
		verifyURL: cfg.VerifyURL,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Enabled 是否启用人机验证。
func (s *TurnstileService) Enabled() bool {
	return s != nil && s.secret != ""
}

type turnstileVerifyResponse struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes"`
}

// Verify 校验前端 Turnstile widget 产出的 token（一次性，10 分钟内有效）。
// remoteip 可选；网络异常按失败处理（fail-closed），避免验证形同虚设。
func (s *TurnstileService) Verify(ctx context.Context, token, remoteIP string) (bool, error) {
	if !s.Enabled() {
		return true, nil
	}

	form := url.Values{}
	form.Set("secret", s.secret)
	form.Set("response", token)
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.verifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("turnstile siteverify http %d", resp.StatusCode)
	}

	var result turnstileVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}
	return result.Success, nil
}
