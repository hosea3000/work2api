package trae

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func mustBuildCallback(t *testing.T, refreshToken, userInfoJSON, userJwtJSON string) string {
	t.Helper()
	v := url.Values{}
	v.Set("refreshToken", refreshToken)
	if userInfoJSON != "" {
		v.Set("userInfo", userInfoJSON)
	}
	if userJwtJSON != "" {
		v.Set("userJwt", userJwtJSON)
	}
	return "http://127.0.0.1:18080/authorize?" + v.Encode()
}

func TestBuildLoginURL(t *testing.T) {
	u := BuildLoginURL("m1234567890abcdef1234567890abcdef", "d1234567890abcdef1234567890abcdef", "http://127.0.0.1:18080/authorize")
	if !strings.HasPrefix(u, ConsoleHost+"/authorization?") {
		t.Fatalf("unexpected base: %s", u)
	}
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := parsed.Query()
	if q.Get("client_id") != ClientID || q.Get("auth_from") != "solo" {
		t.Errorf("missing core params: %v", q)
	}
	if q.Get("auth_callback_url") != "http://127.0.0.1:18080/authorize" {
		t.Errorf("callback not encoded: %q", q.Get("auth_callback_url"))
	}
	if q.Get("machine_id") != "m1234567890abcdef1234567890abcdef" || q.Get("device_id") != "d1234567890abcdef1234567890abcdef" {
		t.Errorf("machine/device missing: %v", q)
	}
	if len(q.Get("login_trace_id")) != TraceIDHexLen {
		t.Errorf("trace len=%d", len(q.Get("login_trace_id")))
	}
}

func TestMachineTraceIDStable(t *testing.T) {
	a := MachineTraceID("mmmm", "dddd")
	b := MachineTraceID("mmmm", "dddd")
	if a != b || len(a) != TraceIDHexLen {
		t.Errorf("trace unstable or wrong len: %q vs %q", a, b)
	}
	if MachineTraceID("m", "d") != strings.Repeat("0", TraceIDHexLen-2)+"md" {
		t.Error("short input should be zero-padded")
	}
}

func TestParseCallbackWithRefreshToken(t *testing.T) {
	userInfo := `{"UserID":"u123","ScreenName":"Alice","TenantID":"ent-1"}`
	userJwt := `{"Token":"at-xyz","RefreshToken":"rt-fallback","TokenExpireAt":1786847930141}`
	info, err := ParseCallback(mustBuildCallback(t, "rt-main", userInfo, userJwt))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if info.RefreshToken != "rt-main" {
		t.Errorf("refreshToken=%q", info.RefreshToken)
	}
	if info.AccessToken != "" {
		t.Errorf("accessToken should be empty when refreshToken present")
	}
	if info.UID != "u123" || info.Nickname != "Alice" || info.EnterpriseID != "ent-1" {
		t.Errorf("userInfo fields: %+v", info)
	}
}

func TestParseCallbackFallbackToUserJwt(t *testing.T) {
	userJwt := `{"Token":"at-from-jwt","RefreshToken":"rt-from-jwt","TokenExpireAt":1786847930141}`
	info, err := ParseCallback(mustBuildCallback(t, "", `{"UserID":"u9","ScreenName":"Bob"}`, userJwt))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if info.RefreshToken != "rt-from-jwt" {
		t.Errorf("fallback refreshToken=%q", info.RefreshToken)
	}
	if info.ExpiresAt != 0 {
		t.Errorf("expiresAt should be 0 pre-exchange, got %d", info.ExpiresAt)
	}
}

func TestParseCallbackNoRefreshButHasJwtToken(t *testing.T) {
	userJwt := `{"Token":"at-direct","TokenExpireAt":1786847930141}`
	info, err := ParseCallback(mustBuildCallback(t, "", `{"UserID":"u1","ScreenName":"S"}`, userJwt))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if info.AccessToken != "at-direct" {
		t.Errorf("accessToken=%q", info.AccessToken)
	}
	if info.ExpiresAt != 1786847930 {
		t.Errorf("millis normalize: got %d", info.ExpiresAt)
	}
}

func TestParseCallbackMissingAllTokens(t *testing.T) {
	cb := mustBuildCallback(t, "", `{"UserID":"u1"}`, `{"RefreshToken":""}`)
	if _, err := ParseCallback(cb); err == nil {
		t.Fatal("want error when both refreshToken and userJwt.Token missing")
	}
}

func TestParseCallbackEmpty(t *testing.T) {
	if _, err := ParseCallback(""); err == nil {
		t.Fatal("want error for empty url")
	}
}

func TestParseCallbackGarbledUserInfo(t *testing.T) {
	info, err := ParseCallback(mustBuildCallback(t, "rt", "not-a-json", ""))
	if err != nil {
		t.Fatalf("garbled userInfo should not error: %v", err)
	}
	if info.UID != "" || info.Nickname != "" {
		t.Errorf("garbled userInfo should yield empty fields: %+v", info)
	}
	if info.RefreshToken != "rt" {
		t.Errorf("refreshToken lost: %+v", info)
	}
}

func TestExpireAtFromExchange(t *testing.T) {
	now := time.Unix(1700000000, 0)
	if got := ExpireAtFromExchange(1786847930141, 0, now); got != 1786847930 {
		t.Errorf("millis future: got %d", got)
	}
	if got := ExpireAtFromExchange(1000, 1209600, now); got != now.Add(1209600*time.Second).Unix() {
		t.Errorf("past expire fallback: got %d", got)
	}
	if got := ExpireAtFromExchange(0, 3600, now); got != now.Add(time.Hour).Unix() {
		t.Errorf("duration only: got %d", got)
	}
	if got := ExpireAtFromExchange(0, 0, now); got != 0 {
		t.Errorf("no info should be 0, got %d", got)
	}
}

func TestFixMojibake(t *testing.T) {
	// ASCII 昵称原样保留
	if got := FixMojibake("Alice"); got != "Alice" {
		t.Errorf("ascii changed: %q", got)
	}
	// 正常 CJK 原样保留
	if got := FixMojibake("张三"); got != "张三" {
		t.Errorf("cjk changed: %q", got)
	}
	// 乱码（非法 UTF-8 语义的高位字节）→ 回退 用户+尾部
	garbled := string([]byte{0xc3, 0x93, 0xc3, 0xbb, 0xc2, 0xa7}) // Óû§ Latin-1 化
	got := FixMojibake(garbled)
	if got == garbled || !strings.HasPrefix(got, "用户") {
		t.Errorf("mojibake not fixed: %q", got)
	}
}
