package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yourname/work2api/internal/model"
	"github.com/yourname/work2api/internal/repository"
)

// APIKeyService sk- API Key 管理与校验。
type APIKeyService interface {
	Create(ctx context.Context, name string) (*APIKeyCreateResult, error)
	List(ctx context.Context) ([]APIKeyView, error)
	Delete(ctx context.Context, id string) error
	Validate(ctx context.Context, rawKey string) (bool, error)
}

func NewAPIKeyService(repo repository.GatewayRepository) APIKeyService {
	return &apiKeyService{repo: repo}
}

type apiKeyService struct {
	repo repository.GatewayRepository
}

// APIKeyCreateResult 创建结果：明文仅此一次。
type APIKeyCreateResult struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Key  string `json:"key"`
}

// APIKeyView 列表视图（脱敏）。
type APIKeyView struct {
	Id        string `json:"id"`
	Name      string `json:"name"`
	KeySuffix string `json:"key_suffix"`
	Disabled  bool   `json:"disabled"`
	CreatedAt string `json:"created_at"`
}

func (s *apiKeyService) Create(ctx context.Context, name string) (*APIKeyCreateResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "unnamed"
	}
	raw, err := generateAPIKey()
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(raw))
	rec := &model.APIKey{
		Id:        uuid.NewString(),
		Name:      name,
		KeyHash:   hex.EncodeToString(sum[:]),
		KeySuffix: "..." + raw[len(raw)-4:],
	}
	if err := s.repo.CreateAPIKey(ctx, rec); err != nil {
		return nil, err
	}
	return &APIKeyCreateResult{Id: rec.Id, Name: rec.Name, Key: raw}, nil
}

func (s *apiKeyService) List(ctx context.Context) ([]APIKeyView, error) {
	keys, err := s.repo.ListAPIKeys(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]APIKeyView, 0, len(keys))
	for _, k := range keys {
		out = append(out, APIKeyView{
			Id:        k.Id,
			Name:      k.Name,
			KeySuffix: k.KeySuffix,
			Disabled:  k.Disabled,
			CreatedAt: k.CreatedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

func (s *apiKeyService) Delete(ctx context.Context, id string) error {
	return s.repo.DeleteAPIKey(ctx, id)
}

// Validate 校验明文 key；不存在/禁用/格式不符一律 false。
func (s *apiKeyService) Validate(ctx context.Context, rawKey string) (bool, error) {
	rawKey = strings.TrimSpace(rawKey)
	if !strings.HasPrefix(rawKey, "sk-") || len(rawKey) < 8 {
		return false, nil
	}
	sum := sha256.Sum256([]byte(rawKey))
	rec, err := s.repo.GetAPIKeyByHash(ctx, hex.EncodeToString(sum[:]))
	if err != nil {
		return false, err
	}
	if rec == nil || rec.Disabled {
		return false, nil
	}
	return true, nil
}

// generateAPIKey 生成 sk- 前缀 256bit 随机 key。
func generateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	return "sk-" + hex.EncodeToString(b), nil
}
