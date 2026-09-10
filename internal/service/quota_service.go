package service

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/hosea3000/work2api/internal/repository"
	"github.com/hosea3000/work2api/internal/upstream/codebuddy"
	"github.com/hosea3000/work2api/internal/upstream/trae"
)

// quotaScanInterval 额度扫描间隔（产品决定写死 1 小时）。
const quotaScanInterval = time.Hour

// Quota 额度快照（仅总额与剩余）。
type Quota struct {
	Total     float64 `json:"total"`
	Remaining float64 `json:"remaining"`
}

// ErrQuotaSkipped 该凭证按设计跳过额度探测（企业凭证、非个人额度）。
var ErrQuotaSkipped = errors.New("quota probe skipped")

// QuotaService 额度探测：每小时扫描 + 手动刷新，结果写回凭证行。
type QuotaService interface {
	Refresh(ctx context.Context, provider, credentialID string) (Quota, error)
	RunLoop(ctx context.Context)
}

func NewQuotaService(
	cbRepo repository.CodeBuddyCredentialRepository,
	traeRepo repository.TraeCredentialRepository,
	cbClient *codebuddy.Client,
	traeClient *trae.Client,
) QuotaService {
	return &quotaService{cbRepo: cbRepo, traeRepo: traeRepo, cbClient: cbClient, traeClient: traeClient}
}

type quotaService struct {
	cbRepo     repository.CodeBuddyCredentialRepository
	traeRepo   repository.TraeCredentialRepository
	cbClient   *codebuddy.Client
	traeClient *trae.Client
}

// Refresh 对指定凭证执行一次额度探测并写回。探测失败时调用方保留旧值。
func (s *quotaService) Refresh(ctx context.Context, provider, credentialID string) (Quota, error) {
	switch provider {
	case providerCodeBuddy:
		cred, err := s.cbRepo.Get(ctx, credentialID)
		if err != nil {
			return Quota{}, err
		}
		if cred == nil {
			return Quota{}, ErrCredentialNotFound
		}
		if cred.EnterpriseId != nil && *cred.EnterpriseId != "" {
			return Quota{}, ErrQuotaSkipped
		}
		total, remaining, err := s.cbClient.FetchQuotaPersonal(ctx, toPoolEntry(*cred).Snapshot)
		if err != nil {
			return Quota{}, err
		}
		if err := s.cbRepo.UpdateQuota(ctx, credentialID, total, remaining); err != nil {
			return Quota{}, err
		}
		return Quota{Total: total, Remaining: remaining}, nil
	case "trae":
		cred, err := s.traeRepo.Get(ctx, credentialID)
		if err != nil {
			return Quota{}, err
		}
		if cred == nil {
			return Quota{}, ErrCredentialNotFound
		}
		remain, limit, _, err := s.traeClient.EntUsage(ctx, cred.BearerToken, derefStr(cred.DeviceID))
		if err != nil {
			return Quota{}, err
		}
		total, remaining := float64(limit), float64(remain)
		if err := s.traeRepo.UpdateQuota(ctx, credentialID, total, remaining); err != nil {
			return Quota{}, err
		}
		return Quota{Total: total, Remaining: remaining}, nil
	default:
		return Quota{}, ErrCredentialNotFound
	}
}

// RunLoop 首轮立即扫描，之后每小时一轮。
func (s *quotaService) RunLoop(ctx context.Context) {
	s.scanOnce(ctx)
	ticker := time.NewTicker(quotaScanInterval)
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

// scanOnce 遍历两 provider 的 active 凭证逐个探测；失败仅记日志，保留旧值。
func (s *quotaService) scanOnce(ctx context.Context) {
	cbList, err := s.cbRepo.List(ctx)
	if err != nil {
		log.Printf("[quota] list codebuddy failed: %v", err)
	}
	for _, c := range cbList {
		if c.Status != "active" {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		s.probeAndLog(ctx, providerCodeBuddy, c.Id)
	}
	traeList, err := s.traeRepo.List(ctx)
	if err != nil {
		log.Printf("[quota] list trae failed: %v", err)
	}
	for _, c := range traeList {
		if c.Status != "active" {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		s.probeAndLog(ctx, "trae", c.Id)
	}
}

func (s *quotaService) probeAndLog(ctx context.Context, provider, id string) {
	if _, err := s.Refresh(ctx, provider, id); err != nil && !errors.Is(err, ErrQuotaSkipped) {
		log.Printf("[quota] %s %s probe failed: %v", provider, id, err)
	}
}
