package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/hosea3000/work2api/internal/upstream/trae"
)

// TraeModelsService TRAE 模型列表：上游 get_detail_param 的 config_name 列表，TTL 缓存。
type TraeModelsService struct {
	client      *trae.Client
	credService TraeCredentialService
	ttl         time.Duration
	mu          sync.Mutex
	cache       []string
	cacheAt     time.Time
	hasCache    bool
}

func NewTraeModelsService(client *trae.Client, credService TraeCredentialService, ttlSeconds int) *TraeModelsService {
	if ttlSeconds <= 0 {
		ttlSeconds = 30
	}
	return &TraeModelsService{
		client:      client,
		credService: credService,
		ttl:         time.Duration(ttlSeconds) * time.Second,
	}
}

// Available 返回 TRAE 模型 id 列表。上游失败回退缓存，无缓存返回空。
func (m *TraeModelsService) Available(ctx context.Context) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.hasCache && time.Since(m.cacheAt) < m.ttl {
		return append([]string{}, m.cache...)
	}
	models := m.fetch(ctx)
	if len(models) == 0 {
		return append([]string{}, m.cache...)
	}
	m.cache = models
	m.cacheAt = time.Now()
	m.hasCache = true
	return append([]string{}, models...)
}

func (m *TraeModelsService) fetch(ctx context.Context) []string {
	entry, ok := m.credService.SelectForChat(ctx)
	if !ok {
		return nil
	}
	c := entry.Credential
	models, err := m.client.FetchModels(c.BearerToken, c.UserId, derefStr(c.MachineID), derefStr(c.DeviceID))
	if err != nil {
		if strings.Contains(err.Error(), "upstream 401") || strings.Contains(err.Error(), "upstream 403") {
			m.credService.MarkExpired(ctx, c.Id)
		}
		return nil
	}
	out := make([]string, 0, len(models))
	for _, mi := range models {
		if mi.ID != "" {
			out = append(out, mi.ID)
		}
	}
	return out
}

// Invalidate 清除缓存（凭证变更时调用）。
func (m *TraeModelsService) Invalidate() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hasCache = false
	m.cache = nil
}
