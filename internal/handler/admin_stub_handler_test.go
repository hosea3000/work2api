package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hosea3000/work2api/internal/service"
)

// fakeCbCreds / fakeTraeCreds 仅实现 List，其余方法由嵌入接口占位（测试不会调用）。
type fakeCbCreds struct {
	service.CodeBuddyCredentialService
	list    []service.CredentialView
	listErr error
}

func (f *fakeCbCreds) List(context.Context) ([]service.CredentialView, error) {
	return f.list, f.listErr
}

type fakeTraeCreds struct {
	service.TraeCredentialService
	list    []service.CredentialView
	listErr error
}

func (f *fakeTraeCreds) List(context.Context) ([]service.CredentialView, error) {
	return f.list, f.listErr
}

type fakeStatsService struct {
	service.StatsService
	total, success int64
	overviewErr    error
	gotStart       int64
	gotEnd         int64
}

func (f *fakeStatsService) Overview(_ context.Context, startAt, endAt int64) (int64, int64, error) {
	f.gotStart = startAt
	f.gotEnd = endAt
	return f.total, f.success, f.overviewErr
}

func doGet(t *testing.T, h gin.HandlerFunc, target string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", h)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", target, nil))
	return w
}

func TestAdminStatusMergesCredentials(t *testing.T) {
	h := &AdminStubHandler{
		Handler:   &Handler{},
		cbCreds:   &fakeCbCreds{list: []service.CredentialView{{}, {}}},
		traeCreds: &fakeTraeCreds{list: []service.CredentialView{{}}},
		stats:     &fakeStatsService{},
	}
	w := doGet(t, h.Status, "/x")
	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", w.Code)
	}
	var resp struct {
		Status      string `json:"status"`
		Credentials struct {
			Total   int `json:"total"`
			Valid   int `json:"valid"`
			Current struct {
				Status string `json:"status"`
			} `json:"current"`
		} `json:"credentials"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "healthy" {
		t.Errorf("status = %q, want healthy", resp.Status)
	}
	if resp.Credentials.Total != 3 || resp.Credentials.Valid != 3 {
		t.Errorf("credentials = (%d,%d), want (3,3)", resp.Credentials.Total, resp.Credentials.Valid)
	}
	if resp.Credentials.Current.Status != "auto_rotation" {
		t.Errorf("current status = %q, want auto_rotation", resp.Credentials.Current.Status)
	}
}

func TestAdminStatusNoCredentials(t *testing.T) {
	h := &AdminStubHandler{
		Handler:   &Handler{},
		cbCreds:   &fakeCbCreds{},
		traeCreds: &fakeTraeCreds{},
		stats:     &fakeStatsService{},
	}
	w := doGet(t, h.Status, "/x")
	var resp struct {
		Credentials struct {
			Current struct {
				Status string `json:"status"`
			} `json:"current"`
		} `json:"credentials"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Credentials.Current.Status != "no_credentials" {
		t.Errorf("current status = %q, want no_credentials", resp.Credentials.Current.Status)
	}
}

func TestStatsOverviewCounts(t *testing.T) {
	h := &AdminStubHandler{
		Handler: &Handler{},
		stats:   &fakeStatsService{total: 4, success: 3},
	}
	w := doGet(t, h.StatsOverview, "/x?start_at=100&end_at=200")
	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", w.Code)
	}
	var resp struct {
		Totals struct {
			RequestCount int      `json:"request_count"`
			SuccessRate  *float64 `json:"success_rate"`
		} `json:"totals"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Totals.RequestCount != 4 {
		t.Errorf("request_count = %d, want 4", resp.Totals.RequestCount)
	}
	if resp.Totals.SuccessRate == nil || *resp.Totals.SuccessRate != 0.75 {
		t.Errorf("success_rate = %v, want 0.75", resp.Totals.SuccessRate)
	}
	if h.stats.(*fakeStatsService).gotStart != 100 || h.stats.(*fakeStatsService).gotEnd != 200 {
		t.Errorf("range = (%d,%d), want (100,200)", h.stats.(*fakeStatsService).gotStart, h.stats.(*fakeStatsService).gotEnd)
	}
}

func TestStatsOverviewEmptyRange(t *testing.T) {
	h := &AdminStubHandler{
		Handler: &Handler{},
		stats:   &fakeStatsService{total: 0, success: 0},
	}
	w := doGet(t, h.StatsOverview, "/x?start_at=1&end_at=2")
	var resp struct {
		Totals struct {
			RequestCount int      `json:"request_count"`
			SuccessRate  *float64 `json:"success_rate"`
		} `json:"totals"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Totals.RequestCount != 0 {
		t.Errorf("request_count = %d, want 0", resp.Totals.RequestCount)
	}
	if resp.Totals.SuccessRate != nil {
		t.Errorf("success_rate = %v, want null", *resp.Totals.SuccessRate)
	}
}
