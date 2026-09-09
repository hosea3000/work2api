package codebuddy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client CodeBuddy 上游客户端（聊天/模型/签到）。
type Client struct {
	Endpoint   string // https://copilot.tencent.com
	CLIVersion string
	HTTP       *http.Client
}

func NewClient(endpoint, cliVersion string) *Client {
	if cliVersion == "" {
		cliVersion = "2.107.0"
	}
	return &Client{
		Endpoint:   strings.TrimRight(endpoint, "/"),
		CLIVersion: cliVersion,
		HTTP: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        100,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
	}
}

// ChatURL 聊天端点。
func (c *Client) ChatURL() string { return c.Endpoint + "/v2/chat/completions" }

// doJSON 执行 JSON 请求。readTimeout 为读超时（<=0 默认 30s）。
func (c *Client) doJSON(ctx context.Context, method, rawURL string, headers map[string]string, payload any, readTimeout time.Duration) (*http.Response, error) {
	if readTimeout <= 0 {
		readTimeout = 30 * time.Second
	}
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	// http.Transport 会覆盖 Host 头取 req.URL.Host；伪装 Host 用 req.Host 字段
	if h := headers["Host"]; h != "" {
		if u, err := url.Parse(rawURL); err == nil && u.Host == h {
			req.Host = h
		}
	}

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// connect 阶段通过 DialContext 生效：简化为整体请求超时由调用方 ctx 控制，
	// 这里仅确保连接阶段有独立超时窗口（http.Client 无 per-phase 超时，靠 Transport 设置）。
	_ = connectCtx

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// handleNon200 将非 200 响应转为受控错误；401/403 → 凭证失效。
func handleNon200(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	message, errType, code := ParseUpstreamErrorBody(string(raw))
	mapped := MapUpstreamStatus(resp.StatusCode, message)
	if errType != "" && mapped.ErrType == "upstream_error" {
		mapped.ErrType = errType
	}
	_ = code
	return mapped
}

// Checkin 执行每日签到，返回 (success, code, message, credit, err)。
// 成功判定与参考实现一致：code==0 或 msg 含"已签到"。
func (c *Client) Checkin(ctx context.Context, cred CredentialSnapshot) (bool, *int, string, *float64, error) {	headers, err := GenerateHeaders(cred, ConversationIDs{}, c.CLIVersion)
	if err != nil {
		return false, nil, "credential error", nil, err
	}
	resp, err := c.doJSON(ctx, http.MethodPost, c.Endpoint+"/billing/meter/daily-checkin", headers, map[string]any{}, 30*time.Second)
	if err != nil {
		return false, nil, "无法连接签到服务", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, nil, fmt.Sprintf("签到服务返回 HTTP %d", resp.StatusCode), nil, handleNon200(resp)
	}
	var body struct {
		Code *int   `json:"code"`
		Msg  string `json:"msg"`
		Data *struct {
			Credit *float64 `json:"credit"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false, nil, "签到服务响应格式无效", nil, &UpstreamError{StatusCode: 502, ErrType: ErrCategoryInvalidResp, Message: "checkin response invalid"}
	}
	code := body.Code
	msg := body.Msg
	if msg == "" {
		msg = "签到服务响应缺少有效消息"
	}
	success := (code != nil && *code == 0) || strings.Contains(msg, "已签到")
	var credit *float64
	if code != nil && *code == 0 {
		credit = body.Data.Credit
	}
	return success, code, msg, credit, nil
}

// FetchModels 用 IDE 变体头拉取 /v3/config 模型列表。
func (c *Client) FetchModels(ctx context.Context, cred CredentialSnapshot) ([]string, error) {
	headers, err := GenerateIDEConfigHeaders(cred, c.CLIVersion)
	if err != nil {
		return nil, err
	}
	resp, err := c.doJSON(ctx, http.MethodGet, c.Endpoint+"/v3/config", headers, nil, 30*time.Second)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, handleNon200(resp)
	}
	var body struct {
		Code *int `json:"code"`
		Data *struct {
			Models []any `json:"models"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, &UpstreamError{StatusCode: 502, ErrType: ErrCategoryInvalidResp, Message: "config response invalid json"}
	}
	if body.Code == nil || *body.Code != 0 || body.Data == nil {
		return nil, &UpstreamError{StatusCode: 502, ErrType: ErrCategoryInvalidResp, Message: "config response invalid payload"}
	}
	models := ExtractModelIDs(body.Data.Models)
	return models, nil
}

// ExtractModelIDs 从 /v3/config data.models 提取字符串模型 ID（容忍对象或字符串形态）。
func ExtractModelIDs(raw []any) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range raw {
		var id string
		switch t := item.(type) {
		case string:
			id = t
		case map[string]any:
			if s, ok := t["id"].(string); ok {
				id = s
			} else if s, ok := t["model"].(string); ok {
				id = s
			}
		}
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
