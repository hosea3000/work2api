package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/yourname/work2api/internal/model"
	"gorm.io/gorm"
)

type GatewayRepository interface {
	// admin user
	GetAdminByUsername(ctx context.Context, username string) (*model.AdminUser, error)
	CountAdmins(ctx context.Context) (int64, error)
	CreateAdmin(ctx context.Context, u *model.AdminUser) error

	// credentials
	ListCredentials(ctx context.Context) ([]model.Credential, error)
	MigrateCredentialColumns(ctx context.Context) error
	GetCredential(ctx context.Context, id string) (*model.Credential, error)
	GetCredentialByUserId(ctx context.Context, userId string) (*model.Credential, error)
	GetCredentialByAccountUid(ctx context.Context, accountUid string) (*model.Credential, error)
	CreateCredential(ctx context.Context, c *model.Credential) error
	UpdateCredential(ctx context.Context, c *model.Credential) error
	DeleteCredential(ctx context.Context, id string) error

	// api keys
	ListAPIKeys(ctx context.Context) ([]model.APIKey, error)
	GetAPIKeyByID(ctx context.Context, id string) (*model.APIKey, error)
	GetAPIKeyByHash(ctx context.Context, hash string) (*model.APIKey, error)
	CreateAPIKey(ctx context.Context, k *model.APIKey) error
	DeleteAPIKey(ctx context.Context, id string) error

	// checkin
	GetCheckinRecord(ctx context.Context, credentialId, date string) (*model.CheckinRecord, error)
	SaveCheckinRecord(ctx context.Context, r *model.CheckinRecord) error
}

func NewGatewayRepository(r *Repository) GatewayRepository {
	return &gatewayRepository{Repository: r}
}

type gatewayRepository struct {
	*Repository
}

func (r *gatewayRepository) GetAdminByUsername(ctx context.Context, username string) (*model.AdminUser, error) {
	var u model.AdminUser
	err := r.DB(ctx).Where("username = ?", username).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *gatewayRepository) CountAdmins(ctx context.Context) (int64, error) {
	var n int64
	err := r.DB(ctx).Model(&model.AdminUser{}).Count(&n).Error
	return n, err
}

func (r *gatewayRepository) CreateAdmin(ctx context.Context, u *model.AdminUser) error {
	return r.DB(ctx).Create(u).Error
}

func (r *gatewayRepository) ListCredentials(ctx context.Context) ([]model.Credential, error) {
	var list []model.Credential
	err := r.DB(ctx).Order("created_at ASC").Find(&list).Error
	return list, err
}

// MigrateCredentialColumns 幂等补齐 credential 表新增列（provider/machine_id/device_id）。
// 独立迁移进程（cmd/migration）之外的常规启动也会执行，兼容存量库。
func (r *gatewayRepository) MigrateCredentialColumns(ctx context.Context) error {
	cols := map[string]string{
		"provider":   "TEXT NOT NULL DEFAULT 'codebuddy'",
		"machine_id": "TEXT",
		"device_id":  "TEXT",
	}
	for name, def := range cols {
		var count int64
		err := r.DB(ctx).Raw(
			"SELECT count(*) FROM pragma_table_info('credential') WHERE name = ?", name,
		).Scan(&count).Error
		if err != nil {
			return err
		}
		if count == 0 {
			if err := r.DB(ctx).Exec(
				fmt.Sprintf("ALTER TABLE credential ADD COLUMN %s %s", name, def),
			).Error; err != nil {
				return err
			}
		}
	}
	// provider 列的索引（IF NOT EXISTS 语义，SQLite 支持）
	return r.DB(ctx).Exec(
		"CREATE INDEX IF NOT EXISTS idx_credential_provider ON credential(provider)",
	).Error
}

func (r *gatewayRepository) GetCredential(ctx context.Context, id string) (*model.Credential, error) {
	var c model.Credential
	err := r.DB(ctx).Where("id = ?", id).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *gatewayRepository) GetCredentialByUserId(ctx context.Context, userId string) (*model.Credential, error) {
	var c model.Credential
	err := r.DB(ctx).Where("user_id = ?", userId).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetCredentialByAccountUid 按上游账号 uid 查凭证（存量 oauth 记录的 user_id 是
// token 哈希，重认证去重需回退按 account_uid 列匹配）。
func (r *gatewayRepository) GetCredentialByAccountUid(ctx context.Context, accountUid string) (*model.Credential, error) {
	var c model.Credential
	err := r.DB(ctx).Where("account_uid = ?", accountUid).Order("created_at DESC").First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *gatewayRepository) CreateCredential(ctx context.Context, c *model.Credential) error {
	return r.DB(ctx).Create(c).Error
}

func (r *gatewayRepository) UpdateCredential(ctx context.Context, c *model.Credential) error {
	return r.DB(ctx).Save(c).Error
}

func (r *gatewayRepository) DeleteCredential(ctx context.Context, id string) error {
	return r.DB(ctx).Where("id = ?", id).Delete(&model.Credential{}).Error
}

func (r *gatewayRepository) ListAPIKeys(ctx context.Context) ([]model.APIKey, error) {
	var list []model.APIKey
	err := r.DB(ctx).Order("created_at DESC").Find(&list).Error
	return list, err
}

func (r *gatewayRepository) GetAPIKeyByID(ctx context.Context, id string) (*model.APIKey, error) {
	var k model.APIKey
	err := r.DB(ctx).Where("id = ?", id).First(&k).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &k, nil
}

func (r *gatewayRepository) GetAPIKeyByHash(ctx context.Context, hash string) (*model.APIKey, error) {
	var k model.APIKey
	err := r.DB(ctx).Where("key_hash = ?", hash).First(&k).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &k, nil
}

func (r *gatewayRepository) CreateAPIKey(ctx context.Context, k *model.APIKey) error {
	return r.DB(ctx).Create(k).Error
}

func (r *gatewayRepository) DeleteAPIKey(ctx context.Context, id string) error {
	return r.DB(ctx).Where("id = ?", id).Delete(&model.APIKey{}).Error
}

func (r *gatewayRepository) GetCheckinRecord(ctx context.Context, credentialId, date string) (*model.CheckinRecord, error) {
	var rec model.CheckinRecord
	err := r.DB(ctx).Where("credential_id = ? AND checkin_date = ?", credentialId, date).First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (r *gatewayRepository) SaveCheckinRecord(ctx context.Context, rec *model.CheckinRecord) error {
	var existing model.CheckinRecord
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
