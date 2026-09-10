package service

import (
	"context"
	"strings"
	"testing"

	"github.com/yourname/work2api/internal/model"
)

// fakeAPIKeyRepo 仅实现 API Key 相关方法，其余 GatewayRepository 方法 panic 防误用。
type fakeAPIKeyRepo struct {
	keys []model.APIKey
}

func (f *fakeAPIKeyRepo) ListAPIKeys(context.Context) ([]model.APIKey, error) { return f.keys, nil }

func (f *fakeAPIKeyRepo) GetAPIKeyByID(_ context.Context, id string) (*model.APIKey, error) {
	for i := range f.keys {
		if f.keys[i].Id == id {
			return &f.keys[i], nil
		}
	}
	return nil, nil
}

func (f *fakeAPIKeyRepo) GetAPIKeyByHash(_ context.Context, hash string) (*model.APIKey, error) {
	for i := range f.keys {
		if f.keys[i].KeyHash == hash {
			return &f.keys[i], nil
		}
	}
	return nil, nil
}

func (f *fakeAPIKeyRepo) GetAPIKeyByName(_ context.Context, name string) (*model.APIKey, error) {
	for i := range f.keys {
		if strings.EqualFold(f.keys[i].Name, name) {
			return &f.keys[i], nil
		}
	}
	return nil, nil
}

func (f *fakeAPIKeyRepo) CreateAPIKey(_ context.Context, k *model.APIKey) error {
	f.keys = append(f.keys, *k)
	return nil
}

func (f *fakeAPIKeyRepo) DeleteAPIKey(_ context.Context, id string) error {
	for i := range f.keys {
		if f.keys[i].Id == id {
			f.keys = append(f.keys[:i], f.keys[i+1:]...)
			return nil
		}
	}
	return nil
}

func (f *fakeAPIKeyRepo) GetAdminByUsername(context.Context, string) (*model.AdminUser, error) {
	panic("unexpected")
}
func (f *fakeAPIKeyRepo) CountAdmins(context.Context) (int64, error) { panic("unexpected") }
func (f *fakeAPIKeyRepo) CreateAdmin(context.Context, *model.AdminUser) error {
	panic("unexpected")
}
func (f *fakeAPIKeyRepo) EnsureSchema(context.Context) error { return nil }

func TestAPIKeyCreateStoresPlaintextAndLists(t *testing.T) {
	repo := &fakeAPIKeyRepo{}
	svc := NewAPIKeyService(repo)

	created, err := svc.Create(context.Background(), "prod")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !strings.HasPrefix(created.Key, "sk-") {
		t.Errorf("key prefix: %q", created.Key)
	}
	if repo.keys[0].Key != created.Key {
		t.Errorf("plaintext not persisted: %q", repo.keys[0].Key)
	}

	list, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Key != created.Key {
		t.Errorf("list should return plaintext: %+v", list)
	}

	// 校验：明文可过，乱码不过
	if ok, _ := svc.Validate(context.Background(), created.Key); !ok {
		t.Error("created key should validate")
	}
	if ok, _ := svc.Validate(context.Background(), "sk-deadbeef"); ok {
		t.Error("unknown key should not validate")
	}
}

func TestAPIKeyDuplicateNameRejected(t *testing.T) {
	repo := &fakeAPIKeyRepo{}
	svc := NewAPIKeyService(repo)
	if _, err := svc.Create(context.Background(), "prod"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	// 完全同名
	if _, err := svc.Create(context.Background(), "prod"); err != ErrDuplicateName {
		t.Errorf("exact duplicate: got %v", err)
	}
	// 仅大小写不同
	if _, err := svc.Create(context.Background(), "Prod"); err != ErrDuplicateName {
		t.Errorf("case-insensitive duplicate: got %v", err)
	}
	if len(repo.keys) != 1 {
		t.Errorf("duplicate should not create, len=%d", len(repo.keys))
	}
}

func TestAPIKeyEmptyNameRejected(t *testing.T) {
	svc := NewAPIKeyService(&fakeAPIKeyRepo{})
	if _, err := svc.Create(context.Background(), "   "); err != ErrEmptyName {
		t.Errorf("empty name: got %v", err)
	}
}
