package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourname/work2api/internal/model"
	"github.com/yourname/work2api/internal/repository"
	"github.com/yourname/work2api/internal/upstream/trae"
)

const providerTrae = "trae"

// TraeCredentialService trae 凭证子系统（表、池、当前/轮换均独立）。
type TraeCredentialService interface {
	List(ctx context.Context) ([]CredentialView, error)
	Current(ctx context.Context) (*CredentialView, error)
	Delete(ctx context.Context, id string) error
	Select(ctx context.Context, id string) (*CredentialView, bool, error)
	ToggleRotation(ctx context.Context) (bool, *CredentialView, error)
	SelectForChat(ctx context.Context) (TraePoolEntry, bool)
	RotationEnabled() bool
	Test(ctx context.Context, id string) (bool, int, string)
	MarkExpired(ctx context.Context, id string)
	GetByID(ctx context.Context, id string) (*model.TraeCredential, error)
	PoolReload(ctx context.Context)
	UpdateCredential(ctx context.Context, c *model.TraeCredential) error
	Cooldown(ctx context.Context, id string, until int64)
}

func NewTraeCredentialService(repo repository.TraeCredentialRepository, pool *TraeCredentialPool, stateRepo repository.PoolStateRepository, client *trae.Client) TraeCredentialService {
	return &traeCredentialService{repo: repo, pool: pool, stateRepo: stateRepo, client: client}
}

type traeCredentialService struct {
	repo      repository.TraeCredentialRepository
	pool      *TraeCredentialPool
	stateRepo repository.PoolStateRepository
	client    *trae.Client
}

func (s *traeCredentialService) List(ctx context.Context) ([]CredentialView, error) {
	creds, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CredentialView, 0, len(creds))
	for _, c := range creds {
		out = append(out, traeView(c))
	}
	return out, nil
}

func (s *traeCredentialService) Current(ctx context.Context) (*CredentialView, error) {
	if entry, ok := s.pool.Current(); ok {
		if c, err := s.repo.Get(ctx, entry.Credential.Id); err == nil && c != nil {
			v := traeView(*c)
			return &v, nil
		}
	}
	creds, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range creds {
		if c.Status == "active" {
			v := traeView(c)
			return &v, nil
		}
	}
	return nil, nil
}

func (s *traeCredentialService) Delete(ctx context.Context, id string) error {
	cred, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if cred == nil {
		return ErrCredentialNotFound
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.pool.MarkExpired(id)
	s.clearCurrentIf(ctx, id)
	return nil
}

// Select 手动选择当前凭证；选择后关闭自动轮换并持久化。
func (s *traeCredentialService) Select(ctx context.Context, id string) (*CredentialView, bool, error) {
	cred, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, false, err
	}
	if cred == nil {
		return nil, false, ErrCredentialNotFound
	}
	if cred.Status != "active" {
		return nil, false, ErrNoCredential
	}
	s.refreshPool(ctx)
	if _, ok := s.pool.SelectByID(id); !ok {
		return nil, false, ErrNoCredential
	}
	if err := s.pool.SelectCurrent(id); err != nil {
		return nil, false, err
	}
	s.pool.SetAutoRotation(false)
	s.saveState(ctx, false, &id)
	v := traeView(*cred)
	return &v, true, nil
}

func (s *traeCredentialService) ToggleRotation(ctx context.Context) (bool, *CredentialView, error) {
	enabled := !s.pool.AutoRotationEnabled()
	s.pool.SetAutoRotation(enabled)
	cur, err := s.Current(ctx)
	id := ""
	if cur != nil {
		id = cur.Id
	}
	s.saveState(ctx, enabled, stringPtrOrNil(id))
	return enabled, cur, err
}

func (s *traeCredentialService) SelectForChat(ctx context.Context) (TraePoolEntry, bool) {
	return s.pool.Select()
}

func (s *traeCredentialService) RotationEnabled() bool {
	return s.pool.AutoRotationEnabled()
}

func (s *traeCredentialService) Test(ctx context.Context, id string) (bool, int, string) {
	cred, err := s.repo.Get(ctx, id)
	if err != nil || cred == nil {
		return false, 404, "credential not found"
	}
	info, err := s.client.GetUserInfo(cred.BearerToken, "")
	if err != nil {
		if strings.Contains(err.Error(), "upstream 401") || strings.Contains(err.Error(), "upstream 403") {
			s.MarkExpired(ctx, id)
		}
		return false, 502, "trae_probe_failed: " + err.Error()
	}
	return true, 200, fmt.Sprintf("uid=%s connected", info.UID)
}

func (s *traeCredentialService) MarkExpired(ctx context.Context, id string) {
	if cred, err := s.repo.Get(ctx, id); err == nil && cred != nil {
		cred.Status = "expired"
		_ = s.repo.Update(ctx, cred)
	}
	s.pool.MarkExpired(id)
	s.clearCurrentIf(ctx, id)
}

func (s *traeCredentialService) GetByID(ctx context.Context, id string) (*model.TraeCredential, error) {
	return s.repo.Get(ctx, id)
}

func (s *traeCredentialService) PoolReload(ctx context.Context) {
	s.refreshPool(ctx)
}

func (s *traeCredentialService) UpdateCredential(ctx context.Context, c *model.TraeCredential) error {
	if err := s.repo.Update(ctx, c); err != nil {
		return err
	}
	s.refreshPool(ctx)
	return nil
}

func (s *traeCredentialService) Cooldown(ctx context.Context, id string, until int64) {
	s.pool.Cooldown(id, until)
}

func (s *traeCredentialService) refreshPool(ctx context.Context) {
	creds, err := s.repo.List(ctx)
	if err != nil {
		return
	}
	state, _ := s.stateRepo.Get(ctx, providerTrae)
	preferID := ""
	auto := true
	if state != nil {
		auto = state.AutoRotation
		if state.CurrentCredentialId != nil {
			preferID = *state.CurrentCredentialId
		}
	}
	s.pool.RefreshWithCurrent(creds, preferID)
	s.pool.SetAutoRotation(auto)
}

func (s *traeCredentialService) saveState(ctx context.Context, auto bool, currentID *string) {
	state, _ := s.stateRepo.Get(ctx, providerTrae)
	if state == nil {
		state = &model.PoolState{Provider: providerTrae}
	}
	state.AutoRotation = auto
	state.CurrentCredentialId = currentID
	_ = s.stateRepo.Upsert(ctx, state)
}

func (s *traeCredentialService) clearCurrentIf(ctx context.Context, id string) {
	state, _ := s.stateRepo.Get(ctx, providerTrae)
	if state != nil && state.CurrentCredentialId != nil && *state.CurrentCredentialId == id {
		state.CurrentCredentialId = nil
		_ = s.stateRepo.Upsert(ctx, state)
	}
}
