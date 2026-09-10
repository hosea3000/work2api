package repository

import (
	"context"
	"errors"
	"time"

	"github.com/yourname/work2api/internal/model"
	"gorm.io/gorm"
)

// CodeBuddyCredentialRepository codebuddy_credential + codebuddy_checkin_record。
type CodeBuddyCredentialRepository interface {
	List(ctx context.Context) ([]model.CodeBuddyCredential, error)
	Get(ctx context.Context, id string) (*model.CodeBuddyCredential, error)
	GetByUserId(ctx context.Context, userId string) (*model.CodeBuddyCredential, error)
	GetByAccountUid(ctx context.Context, accountUid string) (*model.CodeBuddyCredential, error)
	Create(ctx context.Context, c *model.CodeBuddyCredential) error
	Update(ctx context.Context, c *model.CodeBuddyCredential) error
	Delete(ctx context.Context, id string) error
	GetCheckinRecord(ctx context.Context, credentialId, date string) (*model.CodeBuddyCheckinRecord, error)
	SaveCheckinRecord(ctx context.Context, r *model.CodeBuddyCheckinRecord) error
}

func NewCodeBuddyCredentialRepository(r *Repository) CodeBuddyCredentialRepository {
	return &codeBuddyCredentialRepository{Repository: r}
}

type codeBuddyCredentialRepository struct{ *Repository }

func (r *codeBuddyCredentialRepository) List(ctx context.Context) ([]model.CodeBuddyCredential, error) {
	var list []model.CodeBuddyCredential
	err := r.DB(ctx).Order("created_at ASC").Find(&list).Error
	return list, err
}

func (r *codeBuddyCredentialRepository) Get(ctx context.Context, id string) (*model.CodeBuddyCredential, error) {
	var c model.CodeBuddyCredential
	err := r.DB(ctx).Where("id = ?", id).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *codeBuddyCredentialRepository) GetByUserId(ctx context.Context, userId string) (*model.CodeBuddyCredential, error) {
	var c model.CodeBuddyCredential
	err := r.DB(ctx).Where("user_id = ?", userId).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *codeBuddyCredentialRepository) GetByAccountUid(ctx context.Context, accountUid string) (*model.CodeBuddyCredential, error) {
	var c model.CodeBuddyCredential
	err := r.DB(ctx).Where("account_uid = ?", accountUid).Order("created_at DESC").First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *codeBuddyCredentialRepository) Create(ctx context.Context, c *model.CodeBuddyCredential) error {
	return r.DB(ctx).Create(c).Error
}

func (r *codeBuddyCredentialRepository) Update(ctx context.Context, c *model.CodeBuddyCredential) error {
	return r.DB(ctx).Save(c).Error
}

func (r *codeBuddyCredentialRepository) Delete(ctx context.Context, id string) error {
	return r.DB(ctx).Where("id = ?", id).Delete(&model.CodeBuddyCredential{}).Error
}

func (r *codeBuddyCredentialRepository) GetCheckinRecord(ctx context.Context, credentialId, date string) (*model.CodeBuddyCheckinRecord, error) {
	var rec model.CodeBuddyCheckinRecord
	err := r.DB(ctx).Where("credential_id = ? AND checkin_date = ?", credentialId, date).First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (r *codeBuddyCredentialRepository) SaveCheckinRecord(ctx context.Context, rec *model.CodeBuddyCheckinRecord) error {
	var existing model.CodeBuddyCheckinRecord
	err := r.DB(ctx).Where("credential_id = ? AND checkin_date = ?", rec.CredentialId, rec.CheckinDate).First(&existing).Error
	if err == nil {
		rec.Id = existing.Id
		rec.CreatedAt = existing.CreatedAt
		return r.DB(ctx).Save(rec).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now()
	}
	return r.DB(ctx).Create(rec).Error
}

// TraeCredentialRepository trae_credential + trae_checkin_record。
type TraeCredentialRepository interface {
	List(ctx context.Context) ([]model.TraeCredential, error)
	Get(ctx context.Context, id string) (*model.TraeCredential, error)
	GetByUserId(ctx context.Context, userId string) (*model.TraeCredential, error)
	Create(ctx context.Context, c *model.TraeCredential) error
	Update(ctx context.Context, c *model.TraeCredential) error
	Delete(ctx context.Context, id string) error
	GetCheckinRecord(ctx context.Context, credentialId, date string) (*model.TraeCheckinRecord, error)
	SaveCheckinRecord(ctx context.Context, r *model.TraeCheckinRecord) error
}

func NewTraeCredentialRepository(r *Repository) TraeCredentialRepository {
	return &traeCredentialRepository{Repository: r}
}

type traeCredentialRepository struct{ *Repository }

func (r *traeCredentialRepository) List(ctx context.Context) ([]model.TraeCredential, error) {
	var list []model.TraeCredential
	err := r.DB(ctx).Order("created_at ASC").Find(&list).Error
	return list, err
}

func (r *traeCredentialRepository) Get(ctx context.Context, id string) (*model.TraeCredential, error) {
	var c model.TraeCredential
	err := r.DB(ctx).Where("id = ?", id).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *traeCredentialRepository) GetByUserId(ctx context.Context, userId string) (*model.TraeCredential, error) {
	var c model.TraeCredential
	err := r.DB(ctx).Where("user_id = ?", userId).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *traeCredentialRepository) Create(ctx context.Context, c *model.TraeCredential) error {
	return r.DB(ctx).Create(c).Error
}

func (r *traeCredentialRepository) Update(ctx context.Context, c *model.TraeCredential) error {
	return r.DB(ctx).Save(c).Error
}

func (r *traeCredentialRepository) Delete(ctx context.Context, id string) error {
	return r.DB(ctx).Where("id = ?", id).Delete(&model.TraeCredential{}).Error
}

func (r *traeCredentialRepository) GetCheckinRecord(ctx context.Context, credentialId, date string) (*model.TraeCheckinRecord, error) {
	var rec model.TraeCheckinRecord
	err := r.DB(ctx).Where("credential_id = ? AND checkin_date = ?", credentialId, date).First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (r *traeCredentialRepository) SaveCheckinRecord(ctx context.Context, rec *model.TraeCheckinRecord) error {
	var existing model.TraeCheckinRecord
	err := r.DB(ctx).Where("credential_id = ? AND checkin_date = ?", rec.CredentialId, rec.CheckinDate).First(&existing).Error
	if err == nil {
		rec.Id = existing.Id
		rec.CreatedAt = existing.CreatedAt
		return r.DB(ctx).Save(rec).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now()
	}
	return r.DB(ctx).Create(rec).Error
}

// PoolStateRepository pool_state 表（每 provider 一行）。
type PoolStateRepository interface {
	Get(ctx context.Context, provider string) (*model.PoolState, error)
	Upsert(ctx context.Context, state *model.PoolState) error
}

func NewPoolStateRepository(r *Repository) PoolStateRepository {
	return &poolStateRepository{Repository: r}
}

type poolStateRepository struct{ *Repository }

func (r *poolStateRepository) Get(ctx context.Context, provider string) (*model.PoolState, error) {
	var s model.PoolState
	err := r.DB(ctx).Where("provider = ?", provider).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *poolStateRepository) Upsert(ctx context.Context, state *model.PoolState) error {
	state.UpdatedAt = time.Now()
	return r.DB(ctx).Save(state).Error
}
