package codebuddy

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// CredentialSnapshot 是头生成器需要的最小凭证视图。
type CredentialSnapshot struct {
	BearerToken        string
	UserID             string
	AccountUID         string
	Domain             string
	EnterpriseID       string
	DepartmentFullName string
}

// ConversationIDs 客户端可透传的会话标识；空值时随机生成。
type ConversationIDs struct {
	ConversationID          string
	ConversationRequestID   string
	ConversationMessageID   string
	RequestID               string
}

var safeDomainRe = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)

// NodeRuntimeVersion / OpenAIJSPackageVersion 与参考实现对齐，CLI 版本可配置。
const (
	NodeRuntimeVersion     = "v24.11.1"
	OpenAIJSPackageVersion = "6.25.0"
)

// GenerateHeaders 生成 CodeBuddy CLI 伪装头集（对齐 codebuddy_api_client.generate_codebuddy_headers）。
func GenerateHeaders(cred CredentialSnapshot, ids ConversationIDs, cliVersion string) (map[string]string, error) {
	if cred.BearerToken == "" {
		return nil, fmt.Errorf("codebuddy credential missing bearer_token")
	}
	effectiveUserID := cred.AccountUID
	if effectiveUserID == "" {
		effectiveUserID = cred.UserID
	}
	if effectiveUserID == "" {
		return nil, fmt.Errorf("codebuddy credential missing user_id")
	}
	if cliVersion == "" {
		cliVersion = "2.107.0"
	}

	host := HostFromEndpoint(defaultEndpoint)
	domain := host
	if cred.Domain != "" && safeDomainRe.MatchString(cred.Domain) {
		domain = cred.Domain
	}

	headers := map[string]string{
		"Host":                        host,
		"Accept":                      "application/json",
		"Content-Type":                "application/json",
		"X-Requested-With":            "XMLHttpRequest",
		"x-stainless-arch":            stainlessArch(),
		"x-stainless-lang":            "js",
		"x-stainless-os":              stainlessOS(),
		"x-stainless-package-version": OpenAIJSPackageVersion,
		"x-stainless-retry-count":     "0",
		"x-stainless-runtime":         "node",
		"x-stainless-runtime-version": NodeRuntimeVersion,
		"X-Conversation-ID":           orNewUUID(ids.ConversationID),
		"X-Conversation-Request-ID":   orHex16(ids.ConversationRequestID),
		"X-Conversation-Message-ID":   orNewUUID(ids.ConversationMessageID),
		"X-Request-ID":                orNewUUID(ids.RequestID),
		"X-Agent-Intent":              "craft",
		"X-Agent-Purpose":             "conversation",
		"X-IDE-Type":                  "CLI",
		"X-IDE-Name":                  "CLI",
		"X-IDE-Version":               cliVersion,
		"Authorization":               "Bearer " + cred.BearerToken,
		"X-Domain":                    domain,
		"X-Private-Data":              "false",
		"X-CodeBuddy-Request":         "1",
		"X-Product":                   "SaaS",
		"X-User-Id":                   effectiveUserID,
	}
	if cred.EnterpriseID != "" {
		headers["X-Enterprise-Id"] = cred.EnterpriseID
		headers["X-Tenant-Id"] = cred.EnterpriseID
	}
	if cred.DepartmentFullName != "" {
		if strings.ContainsFunc(cred.DepartmentFullName, func(r rune) bool { return r < 32 || r == 127 }) {
			return nil, fmt.Errorf("codebuddy credential contains invalid department_full_name")
		}
		headers["X-Department-Info"] = url.PathEscape(cred.DepartmentFullName)
	}
	return headers, nil
}

// GenerateIDEConfigHeaders 为 /v3/config 模型接口生成 CodeBuddyIDE 变体头集。
func GenerateIDEConfigHeaders(cred CredentialSnapshot, cliVersion string) (map[string]string, error) {
	headers, err := GenerateHeaders(cred, ConversationIDs{}, cliVersion)
	if err != nil {
		return nil, err
	}
	host := HostFromEndpoint(defaultEndpoint)
	if cliVersion == "" {
		cliVersion = "2.107.0"
	}
	headers["Host"] = host
	headers["X-Domain"] = host
	headers["Accept"] = "application/json"
	headers["X-IDE-Type"] = "CodeBuddyIDE"
	headers["X-IDE-Name"] = "CodeBuddyIDE"
	headers["X-IDE-Version"] = cliVersion
	headers["X-Product-Version"] = cliVersion
	return headers, nil
}

func orNewUUID(v string) string {
	if v != "" {
		return v
	}
	return uuid.NewString()
}

func orHex16(v string) string {
	if v != "" {
		return v
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func newHex32() string {
	return orHex16("")
}

// AuthStartHeaders 认证启动 /state 头集（对齐 get_auth_start_headers）：
// X-No-* 哨兵头替代 Authorization，无 b3 头。
func AuthStartHeaders(cliVersion string) map[string]string {
	if cliVersion == "" {
		cliVersion = "2.107.0"
	}
	host := HostFromEndpoint(defaultEndpoint)
	return map[string]string{
		"Host":                    host,
		"Accept":                  "application/json, text/plain, */*",
		"Content-Type":            "application/json",
		"Cache-Control":           "no-cache",
		"Pragma":                  "no-cache",
		"X-Requested-With":        "XMLHttpRequest",
		"X-Domain":                host,
		"X-No-Authorization":      "true",
		"X-No-User-Id":            "true",
		"X-No-Enterprise-Id":      "true",
		"X-No-Department-Info":    "true",
		"User-Agent":              "CLI/" + cliVersion + " CodeBuddy/" + cliVersion,
		"X-Product":               "SaaS",
		"X-Request-ID":            newHex32(),
	}
}

// AuthPollHeaders 认证轮询头集（对齐 get_auth_poll_headers）：b3 链路追踪伪装。
func AuthPollHeaders(cliVersion string) map[string]string {
	if cliVersion == "" {
		cliVersion = "2.107.0"
	}
	host := HostFromEndpoint(defaultEndpoint)
	traceID := newHex32()
	spanID := newHex16()
	return map[string]string{
		"Host":                 host,
		"Accept":               "application/json, text/plain, */*",
		"Cache-Control":        "no-cache",
		"Pragma":               "no-cache",
		"X-Requested-With":     "XMLHttpRequest",
		"X-Request-ID":         traceID,
		"b3":                   traceID + "-" + spanID + "-1-",
		"X-B3-TraceId":         traceID,
		"X-B3-ParentSpanId":    "",
		"X-B3-SpanId":          spanID,
		"X-B3-Sampled":         "1",
		"X-No-Authorization":   "true",
		"X-No-User-Id":         "true",
		"X-No-Enterprise-Id":   "true",
		"X-No-Department-Info": "true",
		"X-Domain":             host,
		"User-Agent":           "CLI/" + cliVersion + " CodeBuddy/" + cliVersion,
		"X-Product":            "SaaS",
	}
}

// AuthRefreshHeaders 凭证刷新头集（对齐 _auth_headers + X-Refresh-Token）：
// 轮询头移除 X-No-Authorization 后带真实 Bearer 与 domain。
func AuthRefreshHeaders(cliVersion string, accessToken, domain, refreshToken string) map[string]string {
	headers := AuthPollHeaders(cliVersion)
	for _, k := range []string{"X-No-Authorization", "X-No-User-Id", "X-No-Enterprise-Id", "X-No-Department-Info"} {
		delete(headers, k)
	}
	headers["Authorization"] = "Bearer " + accessToken
	if domain != "" {
		headers["X-Domain"] = domain
	}
	headers["X-Refresh-Token"] = refreshToken
	headers["X-Auth-Refresh-Source"] = "plugin"
	return headers
}

func newHex16() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
