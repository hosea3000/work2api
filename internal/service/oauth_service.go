package service

import (
	"context"

	"github.com/yourname/work2api/internal/upstream/codebuddy"
)

// OAuthPollResult 轮询判别结果。
type OAuthPollResult struct {
	Kind       string // "pending" | "success" | "error"
	Stage      string // token | account | accounts
	Code       *int
	ErrorName  string
	ErrorDesc  string
	HTTPStatus int
}

// OAuthService 设备授权流编排。
type OAuthService interface {
	Start(ctx context.Context) (map[string]any, error)
	Poll(ctx context.Context, state string) (*OAuthPollResult, bool)
	Cancel(state string) (bool, int)
}

// StartLimitChecker start 端点需要的 store 能力（429 响应带 Retry-After）。
type StartLimitChecker interface {
	BeginStart() (string, error)
	FinishStart(reservation, state string) bool
	CancelStart(reservation string)
	ValidateOwner(state string) bool
	Consume(state string) bool
}

func NewOAuthService(client *codebuddy.Client, creds CodeBuddyCredentialService, models *ModelsService, store *AuthStateStore) OAuthService {
	return &oauthService{client: client, creds: creds, models: models, store: store}
}

type oauthService struct {
	client *codebuddy.Client
	creds  CodeBuddyCredentialService
	models *ModelsService
	store  *AuthStateStore
}

// Start 启动认证。上游失败映射为 {success:false}（200），限流返回 AuthStartLimitError。
func (s *oauthService) Start(ctx context.Context) (map[string]any, error) {
	reservation, err := s.store.BeginStart()
	if err != nil {
		return nil, err
	}
	state, startErr := s.client.StartAuth(ctx)
	if startErr != nil {
		s.store.CancelStart(reservation)
		return map[string]any{
			"success": false,
			"error":   "auth_start_failed",
			"message": "认证启动失败，请稍后重试",
		}, nil
	}
	if !s.store.FinishStart(reservation, state.State) {
		return map[string]any{
			"success": false,
			"error":   "duplicate_auth_state",
			"message": "认证服务返回了重复的 auth_state，请稍后重试",
		}, nil
	}
	return map[string]any{
		"success":                    true,
		"method":                     "codebuddy_real_auth",
		"auth_state":                 state.State,
		"verification_uri_complete":  state.AuthURL,
		"verification_uri":           s.client.Endpoint,
		"expires_in":                 authStateTTLSeconds,
		"interval":                   5,
		"status":                     "awaiting_login",
		"instructions":               "请点击链接完成CodeBuddy登录",
		"message":                    "请使用提供的链接登录CodeBuddy",
		"platform":                   "CLI",
	}, nil
}

// Poll 三段瀑布：token → account → accounts。
// 第二个返回值表示 state 归属校验通过（false → handler 返回 403）。
func (s *oauthService) Poll(ctx context.Context, state string) (*OAuthPollResult, bool) {
	if !s.store.ValidateOwner(state) {
		return nil, false
	}

	// ① token
	td, pending, err := s.client.PollToken(ctx, state)
	if err != nil {
		return authErrToResult(err), true
	}
	if pending {
		code := 11217
		return &OAuthPollResult{
			Kind: "pending", Stage: "token", Code: &code,
			ErrorDesc: "等待用户完成登录",
		}, true
	}

	// ② 当前账号
	acc, pending, err := s.client.PollAccount(ctx, state, td)
	if err != nil {
		return authErrToResult(err), true
	}
	if pending {
		code := 12151
		return &OAuthPollResult{
			Kind: "pending", Stage: "account", Code: &code,
			ErrorDesc: "等待账号信息准备完成",
		}, true
	}

	// ③ 账号列表（仅取首个启用账号的完整信息来源；入库本身用 PollAccount 的当前账号）
	accounts, err := s.client.PollAccounts(ctx, td)
	if err != nil {
		// 参考实现：账号列表不可用时按 pending 处理（等待账号列表准备完成）
		return &OAuthPollResult{Kind: "pending", Stage: "accounts", ErrorDesc: "等待账号列表准备完成"}, true
	}
	_ = accounts // 一期不做多账号切换；account_uid 已从 PollAccount 取得

	// 原子消费（防重放）→ 入库
	if !s.store.Consume(state) {
		return &OAuthPollResult{
			Kind: "error", ErrorName: "auth_error",
			ErrorDesc: "认证状态已失效", HTTPStatus: 409,
		}, true
	}
	if _, addErr := s.creds.AddOAuth(ctx, td, acc); addErr != nil {
		return &OAuthPollResult{
			Kind: "error", ErrorName: "auth_error",
			ErrorDesc: "凭证保存失败，请重新认证", HTTPStatus: 500,
		}, true
	}
	s.models.Invalidate()
	return &OAuthPollResult{Kind: "success"}, true
}

// Cancel 取消：原子消费并确认。
// 返回 (ok, httpStatus)。
func (s *oauthService) Cancel(state string) (bool, int) {
	if state == "" {
		return false, 400
	}
	if !s.store.Consume(state) {
		return false, 403
	}
	return true, 200
}

func authErrToResult(err error) *OAuthPollResult {
	if ae, ok := err.(*codebuddy.AuthError); ok {
		return &OAuthPollResult{
			Kind: "error", ErrorName: ae.Name, ErrorDesc: ae.Description,
			HTTPStatus: ae.HTTPStatus, Code: ae.Code,
		}
	}
	return &OAuthPollResult{
		Kind: "error", ErrorName: "auth_unavailable",
		ErrorDesc: "认证状态查询失败", HTTPStatus: 503,
	}
}
