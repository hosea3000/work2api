package codebuddy

import (
	"strings"
	"testing"
)

func TestAuthStartHeaders(t *testing.T) {
	h := AuthStartHeaders("2.107.0")
	for _, k := range []string{"X-No-Authorization", "X-No-User-Id", "X-No-Enterprise-Id", "X-No-Department-Info"} {
		if h[k] != "true" {
			t.Errorf("missing sentinel %s", k)
		}
	}
	if _, has := h["Authorization"]; has {
		t.Error("start headers must not carry Authorization")
	}
	if _, has := h["b3"]; has {
		t.Error("start headers must not carry b3")
	}
	if h["User-Agent"] != "CLI/2.107.0 CodeBuddy/2.107.0" {
		t.Errorf("user agent wrong: %q", h["User-Agent"])
	}
}

func TestAuthPollHeaders(t *testing.T) {
	h := AuthPollHeaders("2.107.0")
	traceID := h["X-B3-TraceId"]
	spanID := h["X-B3-SpanId"]
	if traceID == "" || spanID == "" {
		t.Fatal("b3 ids required")
	}
	if h["b3"] != traceID+"-"+spanID+"-1-" {
		t.Errorf("b3 format wrong: %q", h["b3"])
	}
	if h["X-B3-Sampled"] != "1" || h["X-B3-ParentSpanId"] != "" {
		t.Errorf("b3 sampled/parent wrong: %v", h)
	}
	if len(traceID) != 32 || len(spanID) != 16 {
		t.Errorf("trace/span id lengths wrong: %d/%d", len(traceID), len(spanID))
	}
}

func TestAuthRefreshHeaders(t *testing.T) {
	h := AuthRefreshHeaders("2.107.0", "tok123", "d.example.com", "rt456")
	if h["Authorization"] != "Bearer tok123" {
		t.Error("refresh needs Authorization")
	}
	if h["X-Refresh-Token"] != "rt456" || h["X-Auth-Refresh-Source"] != "plugin" {
		t.Error("refresh headers missing")
	}
	if h["X-Domain"] != "d.example.com" {
		t.Error("domain override missing")
	}
	for _, k := range []string{"X-No-Authorization", "X-No-User-Id"} {
		if _, has := h[k]; has {
			t.Errorf("refresh must drop %s", k)
		}
	}
}

func TestIsSafeExternalAuthURL(t *testing.T) {
	valid := []string{
		"https://team.example.com/login?state=x",
		"http://localhost:8080/auth",
	}
	invalid := []string{
		"", "   ",
		"https://user:pass@example.com/x", // userinfo
		"ftp://example.com/x",
		"/relative/path",
		"https://example.com/\x01",
		" https://example.com ", // 前后空格
		"https://exa\nmple.com",
	}
	for _, u := range valid {
		if !IsSafeExternalAuthURL(u) {
			t.Errorf("expected valid: %q", u)
		}
	}
	for _, u := range invalid {
		if IsSafeExternalAuthURL(u) {
			t.Errorf("expected invalid: %q", u)
		}
	}
}

func TestBusinessAuthErrorMapping(t *testing.T) {
	cases := map[int]string{
		12005: "企业许可证没有可用席位",
		11212: "CodeBuddy 许可证已过期",
		11216: "CodeBuddy 试用授权已过期",
		10081: "当前 IP 被 CodeBuddy 访问策略限制",
	}
	for code, desc := range cases {
		e := BusinessAuthError(code)
		if e == nil || e.HTTPStatus != 403 || e.Description != desc {
			t.Errorf("code %d mapping wrong: %+v", code, e)
		}
	}
	if BusinessAuthError(0) != nil || BusinessAuthError(99999) != nil {
		t.Error("unknown codes must return nil")
	}
}

func TestParseTokenData(t *testing.T) {
	td := parseTokenData(map[string]any{
		"accessToken":      "at",
		"expiresIn":        float64(86400),
		"expiresAt":        float64(1788950910000), // 毫秒 → 归一化
		"refreshToken":     "rt",
		"refreshExpiresAt": float64(1789000000),
		"domain":           "d.example.com",
		"enterpriseId":     "e1",
	})
	if td == nil || td.AccessToken != "at" || td.RefreshToken != "rt" {
		t.Fatalf("parse wrong: %+v", td)
	}
	if *td.ExpiresAt != 1788950910 {
		t.Errorf("ms normalization wrong: %v", *td.ExpiresAt)
	}
	if *td.ExpiresIn != 86400 || *td.RefreshExpiresAt != 1789000000 {
		t.Errorf("expires fields wrong: %v %v", td.ExpiresIn, td.RefreshExpiresAt)
	}
	if td.EnterpriseID != "e1" || td.Domain != "d.example.com" {
		t.Errorf("domain/enterprise wrong: %v", td)
	}
	if td.TokenType != "Bearer" {
		t.Errorf("default token type wrong: %q", td.TokenType)
	}
	if parseTokenData(nil) != nil || parseTokenData(map[string]any{}) != nil {
		t.Error("empty data must yield nil")
	}
}

func TestParseAccountFiltering(t *testing.T) {
	enabled := parseAccount(map[string]any{"uid": "u1", "pluginEnabled": true, "type": "personal"})
	if enabled == nil || enabled.UID != "u1" || !enabled.PluginEnabled {
		t.Errorf("enabled account parse wrong: %+v", enabled)
	}
	// 缺 uid → nil
	if parseAccount(map[string]any{"pluginEnabled": true}) != nil {
		t.Error("missing uid must yield nil")
	}
}

func TestEpochOrPtr(t *testing.T) {
	if epochOrPtr(nil) != nil || epochOrPtr(float64(0)) != nil || epochOrPtr(-5) != nil {
		t.Error("invalid epochs must be nil")
	}
	if p := epochOrPtr(float64(1788950910)); p == nil || *p != 1788950910 {
		t.Error("seconds epoch passthrough wrong")
	}
	if p := epochOrPtr(float64(1788950910123)); p == nil || *p != 1788950910 {
		t.Error("ms normalization wrong")
	}
}

func TestRefreshParseResponse(t *testing.T) {
	c := NewClient(defaultEndpoint, "2.107.0")
	td, err := c.parseRefreshResponse([]byte(`{"code":0,"data":{"accessToken":"new","refreshToken":"","expiresAt":1789000000}}`))
	if err != nil || td == nil || td.AccessToken != "new" {
		t.Fatalf("refresh parse wrong: %+v %+v", td, err)
	}
	// ip_restricted
	_, err = c.parseRefreshResponse([]byte(`{"code":10081,"msg":"x"}`))
	if ae, ok := err.(*AuthError); !ok || ae.Name != "ip_restricted" {
		t.Errorf("ip restricted mapping wrong: %v", err)
	}
	// code≠0
	_, err = c.parseRefreshResponse([]byte(`{"code":42,"msg":"x"}`))
	if ae, ok := err.(*AuthError); !ok || ae.Name != "invalid_response" {
		t.Errorf("invalid response mapping wrong: %v", err)
	}
	// 非 JSON
	_, err = c.parseRefreshResponse([]byte(`<html>`))
	if err == nil || !strings.Contains(err.Error(), "刷新") {
		t.Errorf("non-json should error: %v", err)
	}
}
