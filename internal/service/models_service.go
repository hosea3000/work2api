package service

import (
	"context"
	"sync"
	"time"

	"github.com/yourname/work2api/internal/upstream/codebuddy"
)

// ModelsService 模型列表：配置模型 ∪ 上游实际模型（有序去重），TTL 缓存。
type ModelsService struct {
	client       *codebuddy.Client
	credService  CredentialService
	configured   []string
	ttl          time.Duration
	mu           sync.Mutex
	cache        []string
	cacheAt      time.Time
	hasCache     bool
}

func NewModelsService(client *codebuddy.Client, credService CredentialService, configured []string, ttlSeconds int) *ModelsService {
	if ttlSeconds <= 0 {
		ttlSeconds = 30
	}
	return &ModelsService{
		client:      client,
		credService: credService,
		configured:  configured,
		ttl:         time.Duration(ttlSeconds) * time.Second,
	}
}

// Available 返回有序去重的模型并集。上游失败回退缓存，再回退配置模型。
func (m *ModelsService) Available(ctx context.Context) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.hasCache && time.Since(m.cacheAt) < m.ttl {
		return append([]string{}, m.cache...)
	}
	actual := m.fetchActual(ctx)
	merged := orderedUnion(m.configured, actual)
	m.cache = merged
	m.cacheAt = time.Now()
	m.hasCache = true
	return append([]string{}, merged...)
}

func (m *ModelsService) fetchActual(ctx context.Context) []string {
	sel, ok := m.credService.SelectByToken(ctx)
	if !ok {
		return nil
	}
	models, err := m.client.FetchModels(ctx, sel.Entry.Snapshot)
	if err != nil {
		// 401/403 摘除凭证
		if ue, isUE := err.(*codebuddy.UpstreamError); isUE && ue.CredInvalid {
			m.credService.MarkExpired(ctx, sel.Entry.Credential.Id)
		}
		return nil
	}
	return models
}

func orderedUnion(configured, actual []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, m := range configured {
		add(m)
	}
	for _, m := range actual {
		add(m)
	}
	return out
}

// Invalidate 清除缓存（凭证变更时调用）。
func (m *ModelsService) Invalidate() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hasCache = false
	m.cache = nil
}
