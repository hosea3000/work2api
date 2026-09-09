package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yourname/work2api/internal/config"
	"github.com/yourname/work2api/internal/model"
	"github.com/yourname/work2api/internal/repository"
	"github.com/yourname/work2api/internal/upstream/codebuddy"
)

var (
	ErrCredentialNotFound = errors.New("credential not found")
	ErrInvalidToken       = errors.New("invalid bearer token")
	ErrNoCredential       = errors.New("no available codebuddy credential")
)

// CredentialService 凭证管理服务。
type CredentialService interface {
	List(ctx context.Context) ([]CredentialView, error)
	Current(ctx context.Context) (*CredentialView, error)
	Add(ctx context.Context, bearerToken string) (*CredentialView, error)
	Delete(ctx context.Context, id string) error
	Select(ctx context.Context, id string) (*CredentialView, bool, error)
	ToggleRotation(ctx context.Context) (bool, *CredentialView, error)
	SelectByToken(ctx context.Context) (Selection, bool)
	RotationEnabled() bool
	Test(ctx context.Context, id string) (ok bool, statusCode int, detail string)
	MarkExpired(ctx context.Context, id string)
	GetByID(ctx context.Context, id string) (*model.Credential, error)
	PoolReload(ctx context.Context)
	AddOAuth(ctx context.Context, td *codebuddy.TokenData, account *codebuddy.Account) (*CredentialView, error)
	UpdateCredential(ctx context.Context, c *model.Credential) error
}

// codebuddySnapshot 别名，避免直接暴露上游类型于接口。
type codebuddySnapshot = PoolEntry

func NewCredentialService(repo repository.GatewayRepository, pool *CredentialPool, conf *config.CodeBuddyConfig, upstream *codebuddy.Client) CredentialService {
	return &credentialService{repo: repo, pool: pool, conf: conf, upstream: upstream}
}

type credentialService struct {
	repo     repository.GatewayRepository
	pool     *CredentialPool
	conf     *config.CodeBuddyConfig
	upstream *codebuddy.Client
}

// CredentialView 前端可见的脱敏凭证视图。
type CredentialView struct {
	Id          string  `json:"id"`
	UserId      string  `json:"user_id"`
	TokenSuffix string  `json:"token_suffix"`
	Status      string  `json:"status"`
	AuthSource  string  `json:"auth_source"`
	Enterprise  *string `json:"enterprise_id,omitempty"`
	CreatedAt   string  `json:"created_at"`
	ExpiresAt   *int64  `json:"expires_at,omitempty"`
}

func (s *credentialService) view(c model.Credential) CredentialView {
	return CredentialView{
		Id:          c.Id,
		UserId:      c.UserId,
		TokenSuffix: tokenSuffix(c.BearerToken),
		Status:      c.Status,
		AuthSource:  c.AuthSource,
		Enterprise:  c.EnterpriseId,
		CreatedAt:   c.CreatedAt.Format(time.RFC3339),
		ExpiresAt:   c.ExpiresAt,
	}
}

func tokenSuffix(tok string) string {
	if len(tok) <= 8 {
		return strings.Repeat("*", len(tok))
	}
	return "..." + tok[len(tok)-8:]
}

func (s *credentialService) List(ctx context.Context) ([]CredentialView, error) {
	creds, err := s.repo.ListCredentials(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CredentialView, 0, len(creds))
	for _, c := range creds {
		out = append(out, s.view(c))
	}
	return out, nil
}

func (s *credentialService) Current(ctx context.Context) (*CredentialView, error) {
	if entry, ok := s.pool.Current(); ok {
		c, err := s.repo.GetCredential(ctx, entry.Credential.Id)
		if err == nil && c != nil {
			v := s.view(*c)
			return &v, nil
		}
	}
	creds, err := s.repo.ListCredentials(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range creds {
		if c.Status == "active" {
			v := s.view(c)
			return &v, nil
		}
	}
	return nil, nil
}

// Add 手动添加凭证：解析 JWT 提取 user_id，无效则拒绝。
func (s *credentialService) Add(ctx context.Context, bearerToken string) (*CredentialView, error) {
	bearerToken = strings.TrimSpace(bearerToken)
	if bearerToken == "" {
		return nil, ErrInvalidToken
	}
	userID, err := ExtractUserIDFromJWT(bearerToken)
	if err != nil {
		return nil, ErrInvalidToken
	}
	// 参考实现行为：相同 user_id 允许并存，由文件名随机后缀区分 → 这里用新 UUID id 并存
	cred := &model.Credential{
		Id:          uuid.NewString(),
		BearerToken: bearerToken,
		UserId:      userID,
		AuthSource:  "manual",
		Status:      "active",
	}
	// 尝试从 JWT 提取 issuer 信息补 domain / enterprise_id
	if domain, entID, ok := ExtractIssuerInfo(bearerToken); ok {
		if domain != "" {
			cred.Domain = &domain
		}
		if entID != "" {
			cred.EnterpriseId = &entID
		}
	}
	if err := s.repo.CreateCredential(ctx, cred); err != nil {
		return nil, err
	}
	s.refreshPool(ctx)
	v := s.view(*cred)
	return &v, nil
}

func (s *credentialService) Delete(ctx context.Context, id string) error {
	cred, err := s.repo.GetCredential(ctx, id)
	if err != nil {
		return err
	}
	if cred == nil {
		return ErrCredentialNotFound
	}
	if err := s.repo.DeleteCredential(ctx, id); err != nil {
		return err
	}
	s.pool.MarkExpired(id) // 从内存池移除（等价语义：删除当前凭证后重置指针）
	return nil
}

// Select 手动选择当前凭证；选择后关闭自动轮换。
func (s *credentialService) Select(ctx context.Context, id string) (*CredentialView, bool, error) {
	cred, err := s.repo.GetCredential(ctx, id)
	if err != nil {
		return nil, false, err
	}
	if cred == nil {
		return nil, false, ErrCredentialNotFound
	}
	if cred.Status != "active" {
		return nil, false, ErrNoCredential
	}
	s.pool.SetAutoRotation(false)
	// 确保在池内且被选中
	s.refreshPool(ctx)
	if _, ok := s.pool.SelectByID(id); !ok {
		return nil, false, ErrNoCredential
	}
	if err := s.forceSelect(id); err != nil {
		return nil, false, err
	}
	v := s.view(*cred)
	return &v, true, nil
}

func (s *credentialService) forceSelect(id string) error {
	// pool.SelectByID 只校验存在；轮换指针推进由 autoRotation=false 时的 Select 语义保证。
	// 为手动选择建立"固定当前"语义，直接把指针置到该凭证。
	return s.pool.SelectCurrent(id)
}

func (s *credentialService) ToggleRotation(ctx context.Context) (bool, *CredentialView, error) {
	enabled := !s.pool.AutoRotationEnabled()
	s.pool.SetAutoRotation(enabled)
	cur, err := s.Current(ctx)
	return enabled, cur, err
}

// SelectByToken 供聊天执行路径调用：原子选择凭证快照。
func (s *credentialService) SelectByToken(ctx context.Context) (Selection, bool) {
	sel, ok := s.pool.Select()
	return sel, ok
}

// RotationEnabled 当前轮换开关状态。
func (s *credentialService) RotationEnabled() bool {
	return s.pool.AutoRotationEnabled()
}

// Test 用凭证请求上游模型列表做连通性测试。
func (s *credentialService) Test(ctx context.Context, id string) (bool, int, string) {
	cred, err := s.repo.GetCredential(ctx, id)
	if err != nil || cred == nil {
		return false, 404, "credential not found"
	}
	entry := toPoolEntry(*cred)
	models, err := s.upstream.FetchModels(ctx, entry.Snapshot)
	if err != nil {
		if ue, isUE := err.(*codebuddy.UpstreamError); isUE {
			if ue.CredInvalid {
				s.MarkExpired(ctx, id)
			}
			return false, ue.StatusCode, ue.ErrType
		}
		return false, 502, "transport_error"
	}
	return true, 200, fmt.Sprintf("%d models available", len(models))
}

// MarkExpired 上游 401/403 时摘除：写库 + 内存摘除。
func (s *credentialService) MarkExpired(ctx context.Context, id string) {
	if cred, err := s.repo.GetCredential(ctx, id); err == nil && cred != nil {
		cred.Status = "expired"
		_ = s.repo.UpdateCredential(ctx, cred)
	}
	s.pool.MarkExpired(id)
}

func (s *credentialService) GetByID(ctx context.Context, id string) (*model.Credential, error) {
	return s.repo.GetCredential(ctx, id)
}

func (s *credentialService) refreshPool(ctx context.Context) {
	creds, err := s.repo.ListCredentials(ctx)
	if err != nil {
		return
	}
	s.pool.Refresh(creds)
}

// PoolReload 供启动时加载。
func (s *credentialService) PoolReload(ctx context.Context) {
	s.refreshPool(ctx)
}

// UpdateCredential 原位更新凭证（刷新器写入新 token 用），并刷新内存池。
func (s *credentialService) UpdateCredential(ctx context.Context, c *model.Credential) error {
	if err := s.repo.UpdateCredential(ctx, c); err != nil {
		return err
	}
	s.refreshPool(ctx)
	return nil
}

// AddOAuth OAuth 认证成功后的入库路径：
// auth_source=oauth；domain/enterprise_id 取 token 响应（非 JWT iss）；
// account_uid 取当前账号；expires_at 缺失时用 created_at + expires_in 推算。
func (s *credentialService) AddOAuth(ctx context.Context, td *codebuddy.TokenData, account *codebuddy.Account) (*CredentialView, error) {
	if td == nil || td.AccessToken == "" {
		return nil, ErrInvalidToken
	}
	now := time.Now().Unix()
	cred := &model.Credential{
		Id:          uuid.NewString(),
		BearerToken: td.AccessToken,
		UserId:      "oauth_" + shortHash(td.AccessToken),
		AuthSource:  "oauth",
		Status:      "active",
		ExpiresAt:   td.ExpiresAt,
		ExpiresIn:   td.ExpiresIn,
		RefreshToken: stringPtr(td.RefreshToken),
		RefreshExpiresAt: td.RefreshExpiresAt,
		SessionState: stringPtr(td.SessionState),
		Scope:       stringPtr(td.Scope),
	}
	if td.ExpiresAt == nil && td.ExpiresIn != nil {
		exp := now + *td.ExpiresIn
		cred.ExpiresAt = &exp
	}
	if td.Domain != "" {
		cred.Domain = &td.Domain
	}
	if td.EnterpriseID != "" {
		cred.EnterpriseId = &td.EnterpriseID
	}
	if account != nil {
		cred.AccountUid = &account.UID
		// 个人账号清空 enterprise_id（对齐参考实现 build_credential_data）
		if account.Type == "personal" {
			cred.EnterpriseId = nil
		} else if account.EnterpriseID != "" {
			cred.EnterpriseId = &account.EnterpriseID
		}
	}
	if err := s.repo.CreateCredential(ctx, cred); err != nil {
		return nil, err
	}
	s.refreshPool(ctx)
	v := s.view(*cred)
	return &v, nil
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// shortHash 取 token 的 SHA-256 前 12 位作 oauth user_id 后缀。
func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}

// ExtractUserIDFromJWT 解析 JWT payload 的 sub 字段；失败报错（拒绝入库）。
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

// ExtractIssuerInfo 从 JWT iss 提取 domain 与 sso-<enterprise_id>（对齐 _extract_issuer_info）。
func ExtractIssuerInfo(token string) (domain, enterpriseID string, ok bool) {
	payload, err := jwtPayload(token)
	if err != nil {
		return "", "", false
	}
	iss, _ := payload["iss"].(string)
	if iss == "" {
		return "", "", false
	}
	// 解析 URL：domain 为 hostname；path 最后一段以 sso- 开头则截取后缀
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
		// 尝试 RawURLEncoding（无 padding）
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

var _ = rand.Read
