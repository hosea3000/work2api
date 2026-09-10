package repository

import (
	"context"
	"errors"

	"github.com/hosea3000/work2api/internal/model"
	"gorm.io/gorm"
)

type GatewayRepository interface {
	// admin user
	GetAdminByUsername(ctx context.Context, username string) (*model.AdminUser, error)
	CountAdmins(ctx context.Context) (int64, error)
	CreateAdmin(ctx context.Context, u *model.AdminUser) error

	// schema
	EnsureSchema(ctx context.Context) error

	// api keys
	ListAPIKeys(ctx context.Context) ([]model.APIKey, error)
	GetAPIKeyByID(ctx context.Context, id string) (*model.APIKey, error)
	GetAPIKeyByHash(ctx context.Context, hash string) (*model.APIKey, error)
	GetAPIKeyByName(ctx context.Context, name string) (*model.APIKey, error)
	CreateAPIKey(ctx context.Context, k *model.APIKey) error
	DeleteAPIKey(ctx context.Context, id string) error
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

// EnsureSchema 确保全部表存在（服务启动自愈：DB 文件被删/全新部署无需先跑 cmd/migration）。
// AutoMigrate 幂等：存量库仅补缺失的表与列，不破坏已有数据。
func (r *gatewayRepository) EnsureSchema(ctx context.Context) error {
	if err := r.DB(ctx).AutoMigrate(
		&model.User{},
		&model.AdminUser{},
		&model.APIKey{},
		&model.CodeBuddyCredential{},
		&model.TraeCredential{},
		&model.CodeBuddyCheckinRecord{},
		&model.TraeCheckinRecord{},
		&model.PoolState{},
	); err != nil {
		return err
	}
	// API Key 名称唯一（不区分大小写）
	return r.DB(ctx).Exec(
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_api_key_name ON api_key(name COLLATE NOCASE)",
	).Error
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

func (r *gatewayRepository) GetAPIKeyByName(ctx context.Context, name string) (*model.APIKey, error) {
	var k model.APIKey
	err := r.DB(ctx).Where("name = ? COLLATE NOCASE", name).First(&k).Error
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
