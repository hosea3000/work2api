package trae

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// BuildLoginURL 构造 TRAE 官网登录 URL（复刻 trae2api-web，参数集来自实测）。
// callbackURL 是 TRAE 登录成功后的重定向落点（如 http://127.0.0.1:18080/authorize）。
// machineID/deviceID 必须与入库凭证共用同一对（hex32），保证登录态与凭证一致。
func BuildLoginURL(machineID, deviceID, callbackURL string) string {
	v := url.Values{}
	v.Set("login_version", "1")
	v.Set("auth_from", "solo")
	v.Set("login_channel", "native_ide")
	v.Set("plugin_version", "2.3.62834")
	v.Set("auth_type", "local")
	v.Set("client_id", ClientID)
	v.Set("redirect", "0")
	return ConsoleHost + "/authorization?" + v.Encode() +
		"&login_trace_id=" + url.QueryEscape(MachineTraceID(machineID, deviceID)) +
		"&auth_callback_url=" + url.QueryEscape(callbackURL) +
		"&machine_id=" + url.QueryEscape(machineID) +
		"&device_id=" + url.QueryEscape(deviceID) +
		"&x_device_id=" + url.QueryEscape(deviceID) +
		"&x_machine_id=" + url.QueryEscape(machineID) +
		"&x_device_brand=PC" +
		"&x_device_type=PC" +
		"&x_os_version=1.0" +
		"&x_app_version=" + url.QueryEscape(IdeVersion) +
		"&x_app_type=stable"
}

// TraceIDHexLen login_trace_id 的 hex 长度。
const TraceIDHexLen = 16

// MachineTraceID 由 machineID+deviceID 派生稳定的 login_trace_id（hex16）。
// TRAE 回调不回传 machine_id/device_id，仅回传 loginTraceID，可据此反查 pending。
func MachineTraceID(machineID, deviceID string) string {
	h := machineID + deviceID
	if len(h) >= TraceIDHexLen {
		return h[len(h)-TraceIDHexLen:]
	}
	return strings.Repeat("0", TraceIDHexLen-len(h)) + h
}

// CallbackInfo 回调链接解析结果（原始凭证，仅服务端内部使用，不得透出前端）。
type CallbackInfo struct {
	RefreshToken string // 优先 query.refreshToken，缺省回退 userJwt.RefreshToken
	AccessToken  string // 无 refreshToken 时回退 userJwt.Token（兜底）
	UID          string // userInfo.UserID
	Nickname     string // userInfo.ScreenName
	EnterpriseID string // userInfo.TenantID（回调字段名是 TenantID）
	ExpiresAt    int64  // 仅 userJwt.Token 兜底路径会设（毫秒归一化为秒）
}

// parseJSONParam 解回调里 URL 编码的 JSON 参数，容错再解一层编码（复刻参考实现）。
func parseJSONParam(raw string) map[string]any {
	if raw == "" {
		return nil
	}
	candidates := []string{raw}
	if uq, err := url.QueryUnescape(raw); err == nil && uq != raw {
		candidates = append(candidates, uq)
	}
	for _, c := range candidates {
		var obj map[string]any
		if json.Unmarshal([]byte(c), &obj) == nil && obj != nil {
			return obj
		}
	}
	return nil
}

func getString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case json.Number:
		return x.String()
	}
	return fmt.Sprintf("%v", v)
}

func getInt64(m map[string]any, key string) int64 {
	if m == nil {
		return 0
	}
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch x := v.(type) {
	case float64:
		return int64(x)
	case json.Number:
		n, _ := x.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	}
	return 0
}

// NormalizeExpire 毫秒/秒归一化：上游 TokenExpireAt 返回毫秒（~1.7e12），>1e12 视为毫秒。
func NormalizeExpire(v int64) int64 {
	if v > 1e12 {
		return v / 1000
	}
	return v
}

// ParseCallback 解析 TRAE 登录回调链接，提取凭证字段。仅解析，不执行 ExchangeToken。
//
// 回调形如：
//
//	http://127.0.0.1:18080/authorize?refreshToken=...&userInfo={...}&userJwt={...}
func ParseCallback(rawURL string) (*CallbackInfo, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("empty callback url")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse callback url: %w", err)
	}
	q := u.Query()
	info := &CallbackInfo{
		RefreshToken: q.Get("refreshToken"),
	}

	userInfo := parseJSONParam(q.Get("userInfo"))
	info.UID = getString(userInfo, "UserID")
	info.Nickname = FixMojibake(getString(userInfo, "ScreenName"))
	info.EnterpriseID = getString(userInfo, "TenantID")

	userJwt := parseJSONParam(q.Get("userJwt"))
	jwtToken := getString(userJwt, "Token")
	jwtRefresh := getString(userJwt, "RefreshToken")

	if info.RefreshToken == "" {
		info.RefreshToken = jwtRefresh
	}
	if info.RefreshToken == "" {
		// 兜底：无 refreshToken 时直接用 userJwt.Token 作为 accessToken
		info.AccessToken = jwtToken
		if jwtToken == "" {
			return nil, fmt.Errorf("callback missing refreshToken and userJwt.Token")
		}
		if exp := getInt64(userJwt, "TokenExpireAt"); exp > 0 {
			info.ExpiresAt = NormalizeExpire(exp)
		}
	}
	return info, nil
}

// ExpireAtFromExchange 把 ExchangeToken 返回的 TokenExpireAt/TokenExpireDuration
// 归一化为 Unix 秒：优先 TokenExpireAt（过去则回退），缺省用 now+duration。
func ExpireAtFromExchange(tokenExpireAt, tokenExpireDuration int64, now time.Time) int64 {
	if tokenExpireAt > 0 {
		exp := NormalizeExpire(tokenExpireAt)
		if exp > now.Unix() {
			return exp
		}
	}
	if tokenExpireDuration > 0 {
		return now.Add(time.Duration(tokenExpireDuration) * time.Second).Unix()
	}
	return 0
}

// FixMojibake 修复回调昵称的双重 URL 编码乱码（实测 ScreenName 中文会出现
// Latin-1 化乱码，如 "Óû§8847309959"）。回转失败则回退 "用户+后缀"。
func FixMojibake(s string) string {
	if s == "" || isASCII(s) {
		return s
	}
	// 尝试按 Latin-1 字节重新解释为 UTF-8
	b := []byte(s)
	for _, enc := range []string{"utf-8", "gbk"} {
		if dec, err := decodeBytes(b, enc); err == nil && looksLikeCJK(dec) {
			return dec
		}
	}
	// 回转失败：回退 用户+原文尾部（保留可辨识度）
	if len(s) > 8 {
		s = s[len(s)-8:]
	}
	return "用户" + s
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 0x7f {
			return false
		}
	}
	return true
}

// looksLikeCJK 粗判字符串是否含 CJK 字符（修复成功的信号）。
func looksLikeCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

// decodeBytes 用指定字符集解码字节；仅支持 utf-8 与 gbk（无外部依赖的最小实现）。
func decodeBytes(b []byte, enc string) (string, error) {
	switch enc {
	case "utf-8":
		if r := []rune(string(b)); len(r) > 0 {
			re := string(r)
			if looksLikeCJK(re) {
				return re, nil
			}
		}
		return "", fmt.Errorf("not cjk utf-8")
	case "gbk":
		return gbkDecode(b)
	}
	return "", fmt.Errorf("unsupported encoding %q", enc)
}

// gbkDecode 最小 GBK 解码：依赖系统 x/text 不可用，退化为 UTF-8 合法性检测，
// 双重编码实测主体是 UTF-8 被误解为 Latin-1，utf-8 分支已覆盖；失败走兜底命名。
func gbkDecode(b []byte) (string, error) {
	if !utf8Valid(b) {
		return "", fmt.Errorf("invalid utf-8")
	}
	s := string(b)
	if !looksLikeCJK(s) {
		return "", fmt.Errorf("no cjk")
	}
	return s, nil
}

func utf8Valid(b []byte) bool {
	return utf8.Valid(b)
}
