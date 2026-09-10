package repository

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hosea3000/work2api/internal/model"
	"gorm.io/gorm"
)

func newStatsTestRepo(t *testing.T) StatsRepository {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "stats.db")
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repo := NewRepository(nil, db)
	if err := NewGatewayRepository(repo).EnsureSchema(context.Background()); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	return NewStatsRepository(repo)
}

func TestStatsRepositoryCountRequests(t *testing.T) {
	repo := newStatsTestRepo(t)
	ctx := context.Background()

	records := []model.RequestRecord{
		{StartedAt: 100, Outcome: "success"},
		{StartedAt: 200, Outcome: "failure"},
		{StartedAt: 300, Outcome: "success"},
	}
	for i := range records {
		if err := repo.Record(ctx, &records[i]); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	// [100, 300) 含 100/200，不含 300
	total, success, err := repo.CountRequests(ctx, 100, 300)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 2 || success != 1 {
		t.Errorf("range [100,300) = (%d,%d), want (2,1)", total, success)
	}

	// 闭开区间：300 起点应包含 300
	total, success, err = repo.CountRequests(ctx, 300, 400)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 1 || success != 1 {
		t.Errorf("range [300,400) = (%d,%d), want (1,1)", total, success)
	}

	// 空区间
	total, success, err = repo.CountRequests(ctx, 1000, 2000)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 0 || success != 0 {
		t.Errorf("empty range = (%d,%d), want (0,0)", total, success)
	}
}
