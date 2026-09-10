package trae

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client TRAE 上游客户端：OAuth 域（ExchangeToken/GetUserInfo）、ug 域（签到/积分）、
// agent 域（聊天补全/模型表）。
type Client struct {
	// HTTP 用于短 JSON 请求（ExchangeToken/GetUserInfo/签到/积分/模型表），有总超时兜底。
	HTTP *http.Client
	// StreamHTTP 用于 SSE 流式对话：不设总超时，避免长流被截断；
	// 通过 Transport.ResponseHeaderTimeout 兜底首字节悬挂。与 HTTP 共享 Transport。
	StreamHTTP *http.Client

	OAuthHost string // https://api.trae.com.cn
	UgHost    string // https://api.trae.cn
	AgentHost string // https://trae-api-cn.mchost.guru
	ClientID  string
}

// New 生产默认客户端。
func New() *Client {
	tr := &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 120 * time.Second, // 首字节兜底（长推理预留），不限制整流时长
	}
	return &Client{
		HTTP:       &http.Client{Timeout: 30 * time.Second, Transport: tr},
		StreamHTTP: &http.Client{Transport: tr}, // 无总超时
		OAuthHost:  OAuthHost,
		UgHost:     UgHost,
		AgentHost:  AgentHost,
		ClientID:   ClientID,
	}
}

// ChatURL SOLO 聊天补全端点。
func (c *Client) ChatURL() string { return c.AgentHost + EpChat }

// SOLOHeaders 设置 llm_utils_chat / get_detail_param 所需的 SOLO 专属头（对齐参考实现，实测必须）。
func SOLOHeaders(req *http.Request, accessToken, uid, machineID, deviceID string, stream bool) {
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	req.Header.Set("User-Agent", "Trae/"+IdeVersion)
	req.Header.Set("Authorization", "Cloud-IDE-JWT "+accessToken)
	req.Header.Set("X-Cloudide-Token", accessToken)
	req.Header.Set("X-Ide-Token", accessToken)
	if uid != "" {
		req.Header.Set("X-Uid", uid)
	}
	req.Header.Set("X-App-Id", AppID)
	req.Header.Set("X-App-Version", "default")
	req.Header.Set("X-Ide-Version", IdeVersion)
	req.Header.Set("X-Ide-Version-Code", IdeVersionCode)
	req.Header.Set("X-App-Version-Code", IdeVersionCode)
	req.Header.Set("X-Ide-Version-Type", "stable")
	req.Header.Set("X-Device-Type", "windows")
	req.Header.Set("X-OS-Version", OSVersion)
	req.Header.Set("X-Device-Brand", DeviceBrand)
	req.Header.Set("Request-Traffic-Type", "prod")
	if machineID != "" {
		req.Header.Set("X-Machine-Id", machineID)
	}
	if deviceID != "" {
		req.Header.Set("X-Device-Id", deviceID)
	}
}

// ModelInfo 动态模型信息。
type ModelInfo struct {
	ID   string // config_name
	Name string // display_name
}

// FetchModels 拉 SOLO 模型表（get_detail_param）。
func (c *Client) FetchModels(accessToken, uid, machineID, deviceID string) ([]ModelInfo, error) {
	body := map[string]any{
		"function":            Function,
		"config_names":        nil,
		"need_prompt":         false,
		"current_config_info": nil,
		"poly_prompt":         true,
		"mode_type":           nil,
		"agent_type":          nil,
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, c.AgentHost+EpModels, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	SOLOHeaders(req, accessToken, uid, machineID, deviceID, false)
	data, err := c.doJSON(req)
	if err != nil {
		return nil, err
	}
	var resp struct {
		ConfigInfoList []struct {
			ConfigName    string `json:"config_name"`
			DisplayConfig struct {
				DisplayName string `json:"display_name"`
			} `json:"display_config"`
		} `json:"config_info_list"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("models parse: %w", err)
	}
	out := make([]ModelInfo, 0, len(resp.ConfigInfoList))
	for _, cfg := range resp.ConfigInfoList {
		if cfg.ConfigName == "" {
			continue
		}
		out = append(out, ModelInfo{ID: cfg.ConfigName, Name: cfg.DisplayConfig.DisplayName})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("models api returned empty list")
	}
	return out, nil
}

// oauthHeaders 设置 ExchangeToken / GetUserInfo 所需头（无签名，仅 UA，对齐参考实现）。
func oauthHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Trae/"+IdeVersion)
}

// doJSON 发请求并解 JSON；HTTP 非 2xx 时返回错误（含 body 片段）。
func (c *Client) doJSON(req *http.Request) (json.RawMessage, error) {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		msg := string(raw)
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return nil, fmt.Errorf("upstream %d: %s", resp.StatusCode, msg)
	}
	return raw, nil
}

// TokenPair ExchangeToken 结果（refreshToken 轮换）。
type TokenPair struct {
	AccessToken  string
	RefreshToken string // 响应未含时调用方保留旧值
	ExpiresAt    int64  // Unix 秒
}

// ExchangeToken 用 refreshToken 换新 access token（refreshToken 每次轮换）。
// host 允许凭证自带 ApiHost 覆盖（空则用默认 OAuthHost）。
func (c *Client) ExchangeToken(refreshToken, host string) (*TokenPair, error) {
	if refreshToken == "" {
		return nil, fmt.Errorf("no refreshToken")
	}
	if host == "" {
		host = c.OAuthHost
	}
	body, _ := json.Marshal(map[string]any{
		"ClientID":     c.ClientID,
		"RefreshToken": refreshToken,
		"ClientSecret": "-",
		"UserID":       "",
	})
	req, err := http.NewRequest(http.MethodPost, host+EpExchange, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	oauthHeaders(req)
	data, err := c.doJSON(req)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Result struct {
			Token               string `json:"Token"`
			TokenExpireAt       int64  `json:"TokenExpireAt"`
			TokenExpireDuration int64  `json:"TokenExpireDuration"`
			RefreshToken        string `json:"RefreshToken"`
		} `json:"Result"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("exchange parse: %w", err)
	}
	if resp.Result.Token == "" {
		return nil, fmt.Errorf("refresh_failed: no token in response — re-login required")
	}
	pair := &TokenPair{
		AccessToken:  resp.Result.Token,
		RefreshToken: resp.Result.RefreshToken,
		ExpiresAt:    ExpireAtFromExchange(resp.Result.TokenExpireAt, resp.Result.TokenExpireDuration, time.Now()),
	}
	return pair, nil
}

// UserInfo GetUserInfo 结果。
type UserInfo struct {
	UID          string
	Nickname     string
	EnterpriseID string
}

// GetUserInfo 用 access token 查询账号信息（uid/nickname/enterprise）。
func (c *Client) GetUserInfo(accessToken, host string) (*UserInfo, error) {
	if host == "" {
		host = c.OAuthHost
	}
	body, _ := json.Marshal(map[string]any{"ReqSource": "IDE", "IDEVersion": IdeVersion})
	req, err := http.NewRequest(http.MethodPost, host+EpUserInfo, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	oauthHeaders(req)
	req.Header.Set("X-Cloudide-Token", accessToken)
	data, err := c.doJSON(req)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Result struct {
			UserID       string `json:"UserID"`
			ScreenName   string `json:"ScreenName"`
			EnterpriseID string `json:"EnterpriseID"`
		} `json:"Result"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("userinfo parse: %w", err)
	}
	return &UserInfo{
		UID:          resp.Result.UserID,
		Nickname:     FixMojibake(resp.Result.ScreenName),
		EnterpriseID: resp.Result.EnterpriseID,
	}, nil
}
