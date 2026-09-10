package service

import (
	"context"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/yourname/work2api/internal/model"
	"github.com/yourname/work2api/internal/upstream/trae"
)

// ShouldTraeRefresh TRAE 凭证刷新判定：web_login && refreshToken 非空 && 临期 24h。
// 与 codebuddy 的 ShouldRefresh 相互独立，两套扫描互不触碰（各自表）。
func ShouldTraeRefresh(cred *model.TraeCredential, now int64) bool {
	if cred == nil || cred.AuthSource != "web_login" {
		return false
	}
	if cred.RefreshToken == nil || *cred.RefreshToken == "" {
		return false
	}
	if cred.ExpiresAt == nil {
		return false
	}
	return now >= *cred.ExpiresAt-RefreshWindowSeconds
}

// TraeTokenRefreshService TRAE 凭证每小时刷新扫描（镜像 TokenRefreshService，独立判定）。
type TraeTokenRefreshService struct {
	creds  TraeCredentialService
	client *trae.Client
	rand   *rand.Rand
	mu     sync.Mutex
	// 刷新前抖动范围（防风控）；测试可置 0 跳过等待。
	jitterMinMs int64
	jitterMaxMs int64
}

func NewTraeTokenRefreshService(creds TraeCredentialService, client *trae.Client) *TraeTokenRefreshService {
	return &TraeTokenRefreshService{
		creds:       creds,
		client:      client,
		rand:        rand.New(rand.NewSource(time.Now().UnixNano())),
		jitterMinMs: 5 * 1000,
		jitterMaxMs: 20 * 1000,
	}
}

// RunLoop 阻塞运行：首轮立即扫描，之后每小时一轮（对齐 codebuddy 刷新节奏）。
func (s *TraeTokenRefreshService) RunLoop(ctx context.Context) {
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

// scanOnce 单轮扫描：遍历 provider=trae 凭证，带抖动逐个临期刷新。
func (s *TraeTokenRefreshService) scanOnce(ctx context.Context) {
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
		if !ShouldTraeRefresh(cred, now) {
			continue
		}
		// 抖动 5~20s（防风控，对齐 codebuddy 刷新节奏）
		delay := time.Duration(s.jitterMinMs+int64(s.rand.Float64()*float64(s.jitterMaxMs-s.jitterMinMs))) * time.Millisecond
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		}
		s.refreshCredential(ctx, cred)
	}
}

// refreshCredential 经 ExchangeToken 刷新单个凭证并原位落库。
// 失败路径不改写凭证字段（旧 refreshToken 保留可重试）；refresh_failed 即标记过期。
func (s *TraeTokenRefreshService) refreshCredential(ctx context.Context, cred *model.TraeCredential) {
	// host 传空 → 用 Client 默认 OAuthHost；cred.Domain 是 "trae.cn"（域名非 URL），不能当 host
	pair, err := s.client.ExchangeToken(derefStr(cred.RefreshToken), "")
	if err != nil {
		// refreshToken 被上游拒绝（明确 refresh_failed / 401/403）→ 凭证死亡，走"重新登录"链路
		msg := err.Error()
		if strings.Contains(msg, "refresh_failed") || strings.Contains(msg, "re-login") ||
			strings.Contains(msg, "upstream 401") || strings.Contains(msg, "upstream 403") {
			log.Printf("[trae-refresh] credential %s refreshToken rejected, marking expired", cred.Id)
			s.creds.MarkExpired(ctx, cred.Id)
			return
		}
		log.Printf("[trae-refresh] credential %s refresh failed: %v", cred.Id, err)
		return
	}
	cred.BearerToken = pair.AccessToken
	// refreshToken 轮换：响应缺新值时沿用旧值
	if pair.RefreshToken != "" {
		cred.RefreshToken = stringPtr(pair.RefreshToken)
	}
	cred.ExpiresAt = &pair.ExpiresAt
	now := time.Now().Unix()
	cred.LastRefreshAt = &now
	if err := s.creds.UpdateCredential(ctx, cred); err != nil {
		log.Printf("[trae-refresh] credential %s persist failed: %v", cred.Id, err)
		return
	}
	log.Printf("[trae-refresh] credential %s refreshed ok", cred.Id)
}
