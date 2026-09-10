package service

import (
	"context"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/hosea3000/work2api/internal/model"
	"github.com/hosea3000/work2api/internal/upstream/codebuddy"
)

// RefreshWindowSeconds 临期窗口：expires_at − 24h 内触发刷新（对齐 REFRESH_WINDOW_SECONDS）。
const RefreshWindowSeconds int64 = 86400

// TokenRefreshService OAuth 凭证每小时刷新扫描（方案 C：最小实现）。
type TokenRefreshService struct {
	creds  CodeBuddyCredentialService
	client *codebuddy.Client
	rand   *rand.Rand
	mu     sync.Mutex
}

func NewTokenRefreshService(creds CodeBuddyCredentialService, client *codebuddy.Client) *TokenRefreshService {
	return &TokenRefreshService{
		creds:  creds,
		client: client,
		rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// ShouldRefresh 判定是否需要刷新（对齐 _should_refresh + _can_refresh_access_token）。
func ShouldRefresh(cred *credentialForRefresh, now int64) bool {
	if cred == nil || cred.AuthSource != "oauth" {
		return false
	}
	if cred.RefreshToken == nil || *cred.RefreshToken == "" {
		return false
	}
	if cred.RefreshExpiresAt != nil && now >= *cred.RefreshExpiresAt {
		return false // refresh_token 已过期，刷不了
	}
	if cred.ExpiresAt == nil {
		return false // 无 expires_at 无法判定临期
	}
	return now >= *cred.ExpiresAt-RefreshWindowSeconds
}

// credentialForRefresh 刷新判定所需字段视图。
type credentialForRefresh struct {
	AuthSource       string
	RefreshToken     *string
	RefreshExpiresAt *int64
	ExpiresAt        *int64
}

// credFull 刷新执行所需的完整凭证视图（gorm 模型 + 原位更新用）。
type credFull = model.CodeBuddyCredential

// RunLoop 阻塞运行：首轮立即扫描，之后每小时一轮。
func (s *TokenRefreshService) RunLoop(ctx context.Context) {
	s.scanOnce(ctx)
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.scanOnce(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// scanOnce 单轮扫描：遍历 oauth 凭证，带抖动逐个刷新。
func (s *TokenRefreshService) scanOnce(ctx context.Context) {
	all, err := s.creds.List(ctx)
	if err != nil {
		return
	}
	now := time.Now().Unix()
	for _, view := range all {
		select {
		case <-ctx.Done():
			return
		default:
		}
		cred, err := s.creds.GetByID(ctx, view.Id)
		if err != nil || cred == nil {
			continue
		}
		cand := &credentialForRefresh{
			AuthSource:       cred.AuthSource,
			RefreshToken:     cred.RefreshToken,
			RefreshExpiresAt: cred.RefreshExpiresAt,
			ExpiresAt:        cred.ExpiresAt,
		}
		if !ShouldRefresh(cand, now) {
			continue
		}
		// 抖动 5~20s（防风控，复用签到节奏）
		delay := time.Duration(5*1000+int64(s.rand.Float64()*15*1000)) * time.Millisecond
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		}
		s.refreshCredential(ctx, cred)
	}
}

// refreshCredential 执行单个凭证刷新并落库。
func (s *TokenRefreshService) refreshCredential(ctx context.Context, cred *credFull) {
	domain := derefStr(cred.Domain)
	td, err := s.client.RefreshToken(ctx, cred.BearerToken, domain, derefStr(cred.RefreshToken))
	if err != nil {
		if ae, ok := err.(*codebuddy.AuthError); ok && ae.Name == "unauthorized" {
			log.Printf("[token-refresh] credential %s rejected by upstream, marking expired", cred.Id)
			s.creds.MarkExpired(ctx, cred.Id)
			return
		}
		log.Printf("[token-refresh] credential %s refresh failed: %v", cred.Id, err)
		return
	}
	// 原位更新：新 token/有效期；响应缺新 refresh_token 时沿用旧的
	cred.BearerToken = td.AccessToken
	cred.ExpiresAt = td.ExpiresAt
	cred.ExpiresIn = td.ExpiresIn
	if td.RefreshToken != "" {
		cred.RefreshToken = stringPtr(td.RefreshToken)
	}
	if td.RefreshExpiresAt != nil {
		cred.RefreshExpiresAt = td.RefreshExpiresAt
	}
	now := time.Now().Unix()
	cred.LastRefreshAt = &now
	if err := s.creds.UpdateCredential(ctx, cred); err != nil {
		log.Printf("[token-refresh] credential %s persist failed: %v", cred.Id, err)
		return
	}
	s.creds.PoolReload(ctx)
	log.Printf("[token-refresh] credential %s refreshed ok", cred.Id)
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
