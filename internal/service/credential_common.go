package service

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hosea3000/work2api/internal/model"
)

var (
	ErrCredentialNotFound = errors.New("credential not found")
	ErrInvalidToken       = errors.New("invalid bearer token")
	ErrNoCredential       = errors.New("no available credential")
)

// CredentialView 前端可见的脱敏凭证视图（两 provider 共用）。
type CredentialView struct {
	Id          string  `json:"id"`
	UserId      string  `json:"user_id"`
	TokenSuffix string  `json:"token_suffix"`
	Status      string  `json:"status"`
	AuthSource  string  `json:"auth_source"`
	Provider    string  `json:"provider"`
	Enterprise  *string `json:"enterprise_id,omitempty"`
	CreatedAt   string  `json:"created_at"`
	ExpiresAt   *int64  `json:"expires_at,omitempty"`
	// 用户身份展示字段
	Nickname          *string `json:"nickname,omitempty"`
	PreferredUsername *string `json:"preferred_username,omitempty"`
	Email             *string `json:"email,omitempty"`
	// 额度探测结果（NULL = 未探测）
	QuotaTotal     *float64 `json:"quota_total,omitempty"`
	QuotaRemaining *float64 `json:"quota_remaining,omitempty"`
}

func codeBuddyView(c model.CodeBuddyCredential) CredentialView {
	return CredentialView{
		Id:                c.Id,
		UserId:            c.UserId,
		TokenSuffix:       tokenSuffix(c.BearerToken),
		Status:            c.Status,
		AuthSource:        c.AuthSource,
		Provider:          "codebuddy",
		Enterprise:        c.EnterpriseId,
		CreatedAt:         c.CreatedAt.Format(time.RFC3339),
		ExpiresAt:         c.ExpiresAt,
		Nickname:          c.Nickname,
		PreferredUsername: c.PreferredUsername,
		Email:             c.Email,
		QuotaTotal:        c.QuotaTotal,
		QuotaRemaining:    c.QuotaRemaining,
	}
}

func traeView(c model.TraeCredential) CredentialView {
	return CredentialView{
		Id:             c.Id,
		UserId:         c.UserId,
		TokenSuffix:    tokenSuffix(c.BearerToken),
		Status:         c.Status,
		AuthSource:     c.AuthSource,
		Provider:       "trae",
		CreatedAt:      c.CreatedAt.Format(time.RFC3339),
		ExpiresAt:      c.ExpiresAt,
		Nickname:       c.Nickname,
		Email:          c.Email,
		QuotaTotal:     c.QuotaTotal,
		QuotaRemaining: c.QuotaRemaining,
	}
}

func tokenSuffix(tok string) string {
	if len(tok) <= 8 {
		return strings.Repeat("*", len(tok))
	}
	return "..." + tok[len(tok)-8:]
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// applyJWTIdentity 从 JWT payload 提取用户身份展示字段（codebuddy 用）。
func applyJWTIdentity(cred *model.CodeBuddyCredential, bearerToken string) {
	payload, err := jwtPayload(bearerToken)
	if err != nil {
		return
	}
	if nickname, _ := payload["nickname"].(string); nickname != "" {
		cred.Nickname = stringPtr(nickname)
	}
	if pu, _ := payload["preferred_username"].(string); pu != "" {
		cred.PreferredUsername = stringPtr(pu)
	}
	if email, _ := payload["email"].(string); email != "" {
		cred.Email = stringPtr(email)
	}
	if cred.Nickname == nil && cred.PreferredUsername != nil {
		cred.Nickname = cred.PreferredUsername
	}
}

// oauthUserID OAuth 凭证的稳定 user_id：优先 uid_<account_uid>，缺失时退回 oauth_<token 哈希>。
func oauthUserID(accessToken, accountUID string) string {
	if accountUID != "" {
		return "uid_" + accountUID
	}
	return "oauth_" + shortHash(accessToken)
}

// shortHash 取 token 的 SHA-256 前 12 位。
func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}

// ExtractUserIDFromJWT 解析 JWT payload 的 sub 字段；失败报错。
func ExtractUserIDFromJWT(token string) (string, error) {
	payload, err := jwtPayload(token)
	if err != nil {
		return "", err
	}
	if sub, ok := payload["sub"].(string); ok && sub != "" {
		return sub, nil
	}
	return "", fmt.Errorf("jwt payload missing sub")
}

// ExtractIssuerInfo 从 JWT iss 提取 domain 与 sso-<enterprise_id>。
func ExtractIssuerInfo(token string) (domain, enterpriseID string, ok bool) {
	payload, err := jwtPayload(token)
	if err != nil {
		return "", "", false
	}
	iss, _ := payload["iss"].(string)
	if iss == "" {
		return "", "", false
	}
	parts := strings.SplitN(iss, "://", 2)
	rest := parts[len(parts)-1]
	host := rest
	if idx := strings.Index(rest, "/"); idx >= 0 {
		host = rest[:idx]
		rest = rest[idx:]
	} else {
		rest = ""
	}
	host = strings.Split(host, ":")[0]
	if p := strings.Trim(rest, "/"); p != "" {
		segs := strings.Split(p, "/")
		last := segs[len(segs)-1]
		if strings.HasPrefix(last, "sso-") && len(last) > len("sso-") {
			enterpriseID = last[len("sso-"):]
		}
	}
	return host, enterpriseID, true
}

func jwtPayload(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("not a jwt")
	}
	payloadPart := parts[1]
	if m := len(payloadPart) % 4; m != 0 {
		payloadPart += strings.Repeat("=", 4-m)
	}
	raw, err := base64.URLEncoding.DecodeString(payloadPart)
	if err != nil {
		raw, err = base64.RawURLEncoding.DecodeString(strings.TrimRight(payloadPart, "="))
		if err != nil {
			return nil, err
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}
