package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hosea3000/work2api/internal/config"
	"github.com/hosea3000/work2api/internal/model"
	"github.com/hosea3000/work2api/internal/repository"
	"github.com/hosea3000/work2api/internal/upstream/codebuddy"
)

const providerCodeBuddy = "codebuddy"

// CodeBuddyCredentialService codebuddy 凭证子系统（表、池、当前/轮换均独立）。
type CodeBuddyCredentialService interface {
	List(ctx context.Context) ([]CredentialView, error)
	Current(ctx context.Context) (*CredentialView, error)
	Add(ctx context.Context, bearerToken string) (*CredentialView, error)
	Delete(ctx context.Context, id string) error
	Select(ctx context.Context, id string) (*CredentialView, bool, error)
	ToggleRotation(ctx context.Context) (bool, *CredentialView, error)
	SelectByToken(ctx context.Context) (Selection, bool)
	RotationEnabled() bool
	Test(ctx context.Context, id string) (bool, int, string)
	MarkExpired(ctx context.Context, id string)
	GetByID(ctx context.Context, id string) (*model.CodeBuddyCredential, error)
	PoolReload(ctx context.Context)
	AddOAuth(ctx context.Context, td *codebuddy.TokenData, account *codebuddy.Account) (*CredentialView, error)
	UpdateCredential(ctx context.Context, c *model.CodeBuddyCredential) error
}

func NewCodeBuddyCredentialService(repo repository.CodeBuddyCredentialRepository, pool *CredentialPool, stateRepo repository.PoolStateRepository, conf *config.CodeBuddyConfig, upstream *codebuddy.Client) CodeBuddyCredentialService {
	return &codeBuddyCredentialService{repo: repo, pool: pool, stateRepo: stateRepo, conf: conf, upstream: upstream}
}

type codeBuddyCredentialService struct {
	repo      repository.CodeBuddyCredentialRepository
	pool      *CredentialPool
	stateRepo repository.PoolStateRepository
	conf      *config.CodeBuddyConfig
	upstream  *codebuddy.Client
}

func (s *codeBuddyCredentialService) List(ctx context.Context) ([]CredentialView, error) {
	creds, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CredentialView, 0, len(creds))
	for _, c := range creds {
		out = append(out, codeBuddyView(c))
	}
	return out, nil
}

func (s *codeBuddyCredentialService) Current(ctx context.Context) (*CredentialView, error) {
	if entry, ok := s.pool.Current(); ok {
		if c, err := s.repo.Get(ctx, entry.Credential.Id); err == nil && c != nil {
			v := codeBuddyView(*c)
			return &v, nil
		}
	}
	creds, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range creds {
		if c.Status == "active" {
			v := codeBuddyView(c)
			return &v, nil
		}
	}
	return nil, nil
}

// Add 手动添加凭证：解析 JWT 提取 user_id，无效则拒绝。
func (s *codeBuddyCredentialService) Add(ctx context.Context, bearerToken string) (*CredentialView, error) {
	userID, err := ExtractUserIDFromJWT(bearerToken)
	if err != nil {
		return nil, ErrInvalidToken
	}
	cred := &model.CodeBuddyCredential{
		Id:          uuid.NewString(),
		BearerToken: bearerToken,
		UserId:      userID,
		AuthSource:  "manual",
		Status:      "active",
	}
	applyJWTIdentity(cred, bearerToken)
	if domain, entID, ok := ExtractIssuerInfo(bearerToken); ok {
		if domain != "" {
			cred.Domain = &domain
		}
		if entID != "" {
			cred.EnterpriseId = &entID
		}
	}
	if err := s.repo.Create(ctx, cred); err != nil {
		return nil, err
	}
	s.refreshPool(ctx)
	v := codeBuddyView(*cred)
	return &v, nil
}

func (s *codeBuddyCredentialService) Delete(ctx context.Context, id string) error {
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
func (s *codeBuddyCredentialService) Select(ctx context.Context, id string) (*CredentialView, bool, error) {
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
	v := codeBuddyView(*cred)
	return &v, true, nil
}

func (s *codeBuddyCredentialService) ToggleRotation(ctx context.Context) (bool, *CredentialView, error) {
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

func (s *codeBuddyCredentialService) SelectByToken(ctx context.Context) (Selection, bool) {
	return s.pool.Select()
}

func (s *codeBuddyCredentialService) RotationEnabled() bool {
	return s.pool.AutoRotationEnabled()
}

func (s *codeBuddyCredentialService) Test(ctx context.Context, id string) (bool, int, string) {
	cred, err := s.repo.Get(ctx, id)
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

func (s *codeBuddyCredentialService) MarkExpired(ctx context.Context, id string) {
	if cred, err := s.repo.Get(ctx, id); err == nil && cred != nil {
		cred.Status = "expired"
		_ = s.repo.Update(ctx, cred)
	}
	s.pool.MarkExpired(id)
	s.clearCurrentIf(ctx, id)
}

func (s *codeBuddyCredentialService) GetByID(ctx context.Context, id string) (*model.CodeBuddyCredential, error) {
	return s.repo.Get(ctx, id)
}

func (s *codeBuddyCredentialService) PoolReload(ctx context.Context) {
	s.refreshPool(ctx)
}

func (s *codeBuddyCredentialService) UpdateCredential(ctx context.Context, c *model.CodeBuddyCredential) error {
	if err := s.repo.Update(ctx, c); err != nil {
		return err
	}
	s.refreshPool(ctx)
	return nil
}

// AddOAuth OAuth 认证成功后的入库路径。
func (s *codeBuddyCredentialService) AddOAuth(ctx context.Context, td *codebuddy.TokenData, account *codebuddy.Account) (*CredentialView, error) {
	if td == nil || td.AccessToken == "" {
		return nil, ErrInvalidToken
	}
	now := time.Now().Unix()
	cred := &model.CodeBuddyCredential{
		Id:               uuid.NewString(),
		BearerToken:      td.AccessToken,
		UserId:           oauthUserID(td.AccessToken, accountUID(account)),
		AuthSource:       "oauth",
		Status:           "active",
		ExpiresAt:        td.ExpiresAt,
		ExpiresIn:        td.ExpiresIn,
		RefreshToken:     stringPtr(td.RefreshToken),
		RefreshExpiresAt: td.RefreshExpiresAt,
		SessionState:     stringPtr(td.SessionState),
		Scope:            stringPtr(td.Scope),
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
		if account.Type == "personal" {
			cred.EnterpriseId = nil
		} else if account.EnterpriseID != "" {
			cred.EnterpriseId = &account.EnterpriseID
		}
		if account.UID != "" {
			var existing *model.CodeBuddyCredential
			if e, err := s.repo.GetByUserId(ctx, "uid_"+account.UID); err == nil && e != nil {
				existing = e
			} else if e, err := s.repo.GetByAccountUid(ctx, account.UID); err == nil && e != nil {
				existing = e
			}
			if existing != nil {
				return s.updateExistingOAuth(ctx, existing, cred)
			}
		}
	}
	applyJWTIdentity(cred, td.AccessToken)
	if err := s.repo.Create(ctx, cred); err != nil {
		return nil, err
	}
	s.refreshPool(ctx)
	v := codeBuddyView(*cred)
	return &v, nil
}

func (s *codeBuddyCredentialService) updateExistingOAuth(ctx context.Context, existing, cred *model.CodeBuddyCredential) (*CredentialView, error) {
	existing.BearerToken = cred.BearerToken
	existing.Status = "active"
	existing.ExpiresAt = cred.ExpiresAt
	existing.ExpiresIn = cred.ExpiresIn
	existing.RefreshToken = cred.RefreshToken
	existing.RefreshExpiresAt = cred.RefreshExpiresAt
	existing.SessionState = cred.SessionState
	existing.Scope = cred.Scope
	existing.AccountUid = cred.AccountUid
	existing.Domain = cred.Domain
	existing.EnterpriseId = cred.EnterpriseId
	applyJWTIdentity(existing, cred.BearerToken)
	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, err
	}
	s.refreshPool(ctx)
	v := codeBuddyView(*existing)
	return &v, nil
}

func (s *codeBuddyCredentialService) refreshPool(ctx context.Context) {
	creds, err := s.repo.List(ctx)
	if err != nil {
		return
	}
	state, _ := s.stateRepo.Get(ctx, providerCodeBuddy)
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

func (s *codeBuddyCredentialService) saveState(ctx context.Context, auto bool, currentID *string) {
	state, _ := s.stateRepo.Get(ctx, providerCodeBuddy)
	if state == nil {
		state = &model.PoolState{Provider: providerCodeBuddy}
	}
	state.AutoRotation = auto
	state.CurrentCredentialId = currentID
	_ = s.stateRepo.Upsert(ctx, state)
}

func (s *codeBuddyCredentialService) clearCurrentIf(ctx context.Context, id string) {
	state, _ := s.stateRepo.Get(ctx, providerCodeBuddy)
	if state != nil && state.CurrentCredentialId != nil && *state.CurrentCredentialId == id {
		state.CurrentCredentialId = nil
		_ = s.stateRepo.Upsert(ctx, state)
	}
}

func accountUID(account *codebuddy.Account) string {
	if account == nil {
		return ""
	}
	return account.UID
}

func stringPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
