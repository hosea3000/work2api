package trae

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client TRAE OAuth 域客户端：ExchangeToken 与 GetUserInfo（登录闭环专用）。
type Client struct {
	HTTP *http.Client

	OAuthHost string // https://api.trae.com.cn
	ClientID  string
}

// New 生产默认客户端。
func New() *Client {
	tr := &http.Transport{
		MaxIdleConns:          20,
		MaxIdleConnsPerHost:   5,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	return &Client{
		HTTP:      &http.Client{Timeout: 30 * time.Second, Transport: tr},
		OAuthHost: OAuthHost,
		ClientID:  ClientID,
	}
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
