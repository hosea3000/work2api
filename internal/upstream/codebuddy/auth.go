package codebuddy

import (
	"context"
	"encoding/json"

	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// AuthState 启动认证结果。
type AuthState struct {
	State   string
	AuthURL string
}

// TokenData 上游 token 响应字段（对齐 _token_data 映射）。
type TokenData struct {
	AccessToken      string
	TokenType        string
	ExpiresIn        *int64
	ExpiresAt        *int64
	RefreshToken     string
	RefreshExpiresIn *int64
	RefreshExpiresAt *int64
	SessionState     string
	Scope            string
	Domain           string
	EnterpriseID     string
	Raw              map[string]any
}

// Account 上游账号。
type Account struct {
	UID           string
	AccountID     string
	Type          string
	Nickname      string
	EnterpriseID  string
	PluginEnabled bool
}

// authEnvelope CodeBuddy 业务信封。
type authEnvelope struct {
	Code *int           `json:"code"`
	Msg  string         `json:"msg"`
	Data map[string]any `json:"data"`
}

// AuthError 认证流程业务错误（受控描述，不透传上游响应体）。
type AuthError struct {
	Name        string // license_seat_limit / license_expired / trial_expired / ip_restricted / invalid_auth_response / auth_unavailable / auth_error / unauthorized / invalid_response
	Description string
	HTTPStatus  int
	Code        *int
}

func (e *AuthError) Error() string { return e.Description }

// authBusinessErrorDetails 与参考实现 AUTH_ERROR_DETAILS 一致。
var authBusinessErrorDetails = map[int]struct {
	name string
	desc string
}{
	12005: {"license_seat_limit", "企业许可证没有可用席位"},
	11212: {"license_expired", "CodeBuddy 许可证已过期"},
	11216: {"trial_expired", "CodeBuddy 试用授权已过期"},
	10081: {"ip_restricted", "当前 IP 被 CodeBuddy 访问策略限制"},
}

// BusinessAuthError 将上游业务码映射为受控错误；非已知码返回 nil。
func BusinessAuthError(code int) *AuthError {
	if d, ok := authBusinessErrorDetails[code]; ok {
		return &AuthError{Name: d.name, Description: d.desc, HTTPStatus: 403, Code: &code}
	}
	return nil
}

// IsSafeExternalAuthURL 校验认证 URL：无用户信息、无控制字符的绝对 HTTP(S) URL。
func IsSafeExternalAuthURL(v string) bool {
	if v == "" || v != strings.TrimSpace(v) {
		return false
	}
	for _, r := range v {
		if r < 32 || r == 127 {
			return false
		}
	}
	u, err := url.Parse(v)
	if err != nil {
		return false
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return false
	}
	return true
}

// StartAuth 启动认证：POST /v2/plugin/auth/state?platform=CLI。
func (c *Client) StartAuth(ctx context.Context) (*AuthState, error) {
	headers := AuthStartHeaders(c.CLIVersion)
	body, err := c.doAuthJSON(ctx, http.MethodPost, c.Endpoint+"/v2/plugin/auth/state?platform=CLI", headers, map[string]any{})
	if err != nil {
		return nil, err
	}
	var env authEnvelope
	if err := json.Unmarshal(body, &env); err != nil || env.Code == nil || *env.Code != 0 || env.Data == nil {
		return nil, &AuthError{Name: "auth_start_failed", Description: "无法启动认证流程", HTTPStatus: 502}
	}
	state, _ := env.Data["state"].(string)
	authURL, _ := env.Data["authUrl"].(string)
	if state == "" || !IsSafeExternalAuthURL(authURL) {
		return nil, &AuthError{Name: "auth_start_failed", Description: "无法启动认证流程", HTTPStatus: 502}
	}
	return &AuthState{State: state, AuthURL: authURL}, nil
}

// PollToken 轮询①：GET /v2/plugin/auth/token?state=X。
// 返回 (tokenData, pending, err)。pending=true 表示 code=11217 等待登录。
func (c *Client) PollToken(ctx context.Context, state string) (*TokenData, bool, error) {
	headers := AuthPollHeaders(c.CLIVersion)
	body, err := c.doAuthJSON(ctx, http.MethodGet, c.Endpoint+"/v2/plugin/auth/token?state="+url.QueryEscape(state), headers, nil)
	if err != nil {
		return nil, false, &AuthError{Name: "auth_unavailable", Description: "认证服务暂时不可用", HTTPStatus: 503}
	}
	var env authEnvelope
	if err := json.Unmarshal(body, &env); err != nil || env.Code == nil {
		return nil, false, &AuthError{Name: "invalid_auth_response", Description: "认证服务返回无效响应", HTTPStatus: 502}
	}
	if *env.Code == 11217 {
		return nil, true, nil
	}
	if be := BusinessAuthError(*env.Code); be != nil {
		return nil, false, be
	}
	if *env.Code != 0 {
		return nil, false, &AuthError{Name: "invalid_auth_response", Description: "认证服务返回未知状态", HTTPStatus: 502, Code: env.Code}
	}
	td := parseTokenData(env.Data)
	if td == nil {
		return nil, false, &AuthError{Name: "invalid_auth_response", Description: "认证响应缺少访问令牌", HTTPStatus: 502}
	}
	return td, false, nil
}

// PollAccount 轮询②：GET /v2/plugin/login/account?state=X。
// 返回 (account, pending, err)。pending=true 表示 code=12151。
func (c *Client) PollAccount(ctx context.Context, state string, td *TokenData) (*Account, bool, error) {
	headers := AuthRefreshHeaders(c.CLIVersion, td.AccessToken, td.Domain, "") // _auth_headers 语义：Bearer + X-Domain，无 X-Refresh-Token
	delete(headers, "X-Refresh-Token")
	delete(headers, "X-Auth-Refresh-Source")
	body, err := c.doAuthJSON(ctx, http.MethodGet, c.Endpoint+"/v2/plugin/login/account?state="+url.QueryEscape(state), headers, nil)
	if err != nil {
		return nil, false, &AuthError{Name: "auth_unavailable", Description: "账号服务暂时不可用", HTTPStatus: 503}
	}
	var env authEnvelope
	if err := json.Unmarshal(body, &env); err != nil || env.Code == nil {
		return nil, false, &AuthError{Name: "invalid_auth_response", Description: "账号服务返回无效响应", HTTPStatus: 502}
	}
	if *env.Code == 12151 {
		return nil, true, nil
	}
	if be := BusinessAuthError(*env.Code); be != nil {
		return nil, false, be
	}
	if *env.Code != 0 || env.Data == nil {
		return nil, false, &AuthError{Name: "invalid_auth_response", Description: "账号服务返回无效账号", HTTPStatus: 502}
	}
	acc := parseAccount(env.Data)
	if acc == nil || acc.UID == "" {
		return nil, false, &AuthError{Name: "invalid_auth_response", Description: "账号服务返回无效账号", HTTPStatus: 502}
	}
	return acc, false, nil
}

// PollAccounts 轮询③：GET /v2/plugin/accounts，过滤 pluginEnabled==true。
func (c *Client) PollAccounts(ctx context.Context, td *TokenData) ([]Account, error) {
	headers := AuthRefreshHeaders(c.CLIVersion, td.AccessToken, td.Domain, "")
	delete(headers, "X-Refresh-Token")
	delete(headers, "X-Auth-Refresh-Source")
	body, err := c.doAuthJSON(ctx, http.MethodGet, c.Endpoint+"/v2/plugin/accounts", headers, nil)
	if err != nil {
		return nil, &AuthError{Name: "auth_unavailable", Description: "账号服务暂时不可用", HTTPStatus: 503}
	}
	var env authEnvelope
	if err := json.Unmarshal(body, &env); err != nil || env.Code == nil {
		return nil, &AuthError{Name: "invalid_auth_response", Description: "账号列表返回无效响应", HTTPStatus: 502}
	}
	if be := BusinessAuthError(*env.Code); be != nil {
		return nil, be
	}
	if *env.Code != 0 || env.Data == nil {
		return nil, &AuthError{Name: "invalid_auth_response", Description: "账号列表返回无效数据", HTTPStatus: 502}
	}
	rawAccounts, _ := env.Data["accounts"].([]any)
	if rawAccounts == nil {
		return nil, &AuthError{Name: "invalid_auth_response", Description: "账号列表返回无效数据", HTTPStatus: 502}
	}
	var out []Account
	for _, item := range rawAccounts {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if enabled, _ := m["pluginEnabled"].(bool); enabled {
			if acc := parseAccount(m); acc != nil {
				out = append(out, *acc)
			}
		}
	}
	return out, nil
}

// RefreshToken 刷新 OAuth 凭证：POST /v2/plugin/auth/token/refresh。
// 401/403 → unauthorized 错误（调用方摘除凭证）。
func (c *Client) RefreshToken(ctx context.Context, accessToken, domain, refreshToken string) (*TokenData, error) {
	if refreshToken == "" {
		return nil, &AuthError{Name: "refresh_unavailable", Description: "凭证缺少 refresh_token", HTTPStatus: 400}
	}
	headers := AuthRefreshHeaders(c.CLIVersion, accessToken, domain, refreshToken)
	body, err := c.doAuthJSON(ctx, http.MethodPost, c.Endpoint+"/v2/plugin/auth/token/refresh", headers, map[string]any{})
	if err != nil {
		return nil, &AuthError{Name: "refresh_failed", Description: "刷新请求失败", HTTPStatus: 503}
	}
	// 上游 401/403 需区分于业务信封：doAuthJSON 不预读状态码，这里重发判定
	// 简化：doAuthJSON 已将非 200 一并读体返回，由 HTTP 状态判定
	return c.parseRefreshResponse(body)
}

func (c *Client) parseRefreshResponse(raw []byte) (*TokenData, error) {
	var env authEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, &AuthError{Name: "invalid_response", Description: "刷新响应无效", HTTPStatus: 502}
	}
	if env.Code != nil && *env.Code == 10081 {
		return nil, &AuthError{Name: "ip_restricted", Description: "当前 IP 被 CodeBuddy 访问策略限制", HTTPStatus: 403, Code: env.Code}
	}
	if env.Code == nil || *env.Code != 0 {
		return nil, &AuthError{Name: "invalid_response", Description: "刷新响应无效", HTTPStatus: 502}
	}
	td := parseTokenData(env.Data)
	if td == nil {
		return nil, &AuthError{Name: "invalid_response", Description: "刷新响应缺少访问令牌", HTTPStatus: 502}
	}
	return td, nil
}

// doAuthJSON 执行认证 JSON 请求并返回响应体；非 200 时按状态映射错误。
func (c *Client) doAuthJSON(ctx context.Context, method, rawURL string, headers map[string]string, payload any) ([]byte, error) {
	resp, err := c.doJSON(ctx, method, rawURL, headers, payload, 30*time.Second)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, &AuthError{Name: "unauthorized", Description: "凭证已被上游拒绝", HTTPStatus: resp.StatusCode}
	}
	if resp.StatusCode >= 500 {
		return nil, &AuthError{Name: "auth_unavailable", Description: "认证服务暂时不可用", HTTPStatus: 503}
	}
	if resp.StatusCode != 200 {
		return nil, &AuthError{Name: "invalid_response", Description: "认证服务响应无效", HTTPStatus: 502}
	}
	return body, nil
}

// parseTokenData 从 data 提取 TokenData（对齐 _token_data）。
func parseTokenData(data map[string]any) *TokenData {
	if data == nil {
		return nil
	}
	accessToken, _ := data["accessToken"].(string)
	if accessToken == "" {
		return nil
	}
	td := &TokenData{
		AccessToken:  accessToken,
		TokenType:    stringOr(data["tokenType"], "Bearer"),
		RefreshToken: stringOr(data["refreshToken"], ""),
		SessionState: stringOr(data["sessionState"], ""),
		Scope:        stringOr(data["scope"], ""),
		Domain:       stringOr(data["domain"], ""),
		Raw:          data,
	}
	if v, ok := data["enterpriseId"].(string); ok && v != "" {
		td.EnterpriseID = v
	} else if v, ok := data["enterprise_id"].(string); ok && v != "" {
		td.EnterpriseID = v
	}
	td.ExpiresIn = epochOrPtr(data["expiresIn"])
	td.ExpiresAt = epochOrPtr(data["expiresAt"])
	td.RefreshExpiresIn = epochOrPtr(data["refreshExpiresIn"])
	td.RefreshExpiresAt = epochOrPtr(data["refreshExpiresAt"])
	return td
}

// parseAccount 从 data 提取账号。
func parseAccount(data map[string]any) *Account {
	if data == nil {
		return nil
	}
	uid, _ := data["uid"].(string)
	if uid == "" {
		return nil
	}
	acc := &Account{
		UID:           uid,
		Type:          stringOr(data["type"], ""),
		Nickname:      stringOr(data["nickname"], ""),
		PluginEnabled: boolOrN(data["pluginEnabled"]),
	}
	if id, ok := data["account_id"].(string); ok {
		acc.AccountID = id
	} else if id, ok := data["accountId"].(string); ok {
		acc.AccountID = id
	}
	if ent, ok := data["enterpriseId"].(string); ok && ent != "" {
		acc.EnterpriseID = ent
	} else if ent, ok := data["enterprise_id"].(string); ok && ent != "" {
		acc.EnterpriseID = ent
	}
	return acc
}

func stringOr(v any, def string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}

func boolOrN(v any) bool {
	b, _ := v.(bool)
	return b
}

// epochOrPtr 归一化时间戳：数字且 >0；≥1000 亿按毫秒除 1000（对齐 _normalize_epoch）。
func epochOrPtr(v any) *int64 {
	f, ok := v.(float64)
	if !ok || f <= 0 {
		return nil
	}
	n := int64(f)
	if n >= 100_000_000_000 {
		n /= 1000
	}
	return &n
}
