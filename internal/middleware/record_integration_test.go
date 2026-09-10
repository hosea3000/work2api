package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hosea3000/work2api/internal/repository"
	"github.com/hosea3000/work2api/internal/service"
	"gorm.io/gorm"
)

// TestRecordRequestEndToEnd 验证「请求 → 落库 → 统计」整条链路。
func TestRecordRequestEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsn := filepath.Join(t.TempDir(), "e2e.db")
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	base := repository.NewRepository(nil, db)
	if err := repository.NewGatewayRepository(base).EnsureSchema(context.Background()); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	stats := service.NewStatsService(repository.NewStatsRepository(base))

	r := gin.New()
	r.POST("/chat", RecordRequest(nil, stats), func(c *gin.Context) { c.Status(http.StatusOK) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/chat", nil))

	// 覆盖 0..now+1 的区间应包含刚记录的请求
	total, success, err := stats.Overview(context.Background(), 0, 1<<62)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if total != 1 || success != 1 {
		t.Errorf("overview = (%d,%d), want (1,1)", total, success)
	}
}
