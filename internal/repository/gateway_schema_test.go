package repository

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestEnsureSchemaOnFreshDB 全新/删库启动时 EnsureSchema 建出全部新表。
func TestEnsureSchemaOnFreshDB(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "fresh.db")
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repo := NewGatewayRepository(NewRepository(nil, db))
	ctx := context.Background()

	if err := repo.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}

	tables := []string{
		"codebuddy_credential", "trae_credential",
		"codebuddy_checkin_record", "trae_checkin_record",
		"pool_state", "api_key", "admin_user",
	}
	for _, tbl := range tables {
		var n int64
		if err := db.Raw("SELECT count(*) FROM sqlite_master WHERE type='table' AND name = ?", tbl).Scan(&n).Error; err != nil {
			t.Fatalf("check %s: %v", tbl, err)
		}
		if n != 1 {
			t.Errorf("table %s missing", tbl)
		}
	}
	// 旧表不再创建
	var old int64
	_ = db.Raw("SELECT count(*) FROM sqlite_master WHERE type='table' AND name = 'credential'").Scan(&old).Error
	if old != 0 {
		t.Errorf("legacy credential table should not exist")
	}
}
