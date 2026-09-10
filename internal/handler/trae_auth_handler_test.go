package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hosea3000/work2api/internal/config"
	"github.com/hosea3000/work2api/internal/model"
	"github.com/hosea3000/work2api/internal/repository"
	"github.com/hosea3000/work2api/internal/service"
	"github.com/hosea3000/work2api/internal/upstream/trae"
)

// fakeTraeRepo 最小 TraeCredentialRepository 替身（内存）。
type fakeTraeRepo struct{ saved []model.TraeCredential }

func (f *fakeTraeRepo) List(context.Context) ([]model.TraeCredential, error) { return f.saved, nil }
func (f *fakeTraeRepo) Get(_ context.Context, id string) (*model.TraeCredential, error) {
	for i := range f.saved {
		if f.saved[i].Id == id {
			return &f.saved[i], nil
		}
	}
	return nil, nil
}
func (f *fakeTraeRepo) GetByUserId(_ context.Context, uid string) (*model.TraeCredential, error) {
	for i := range f.saved {
		if f.saved[i].UserId == uid {
			return &f.saved[i], nil
		}
	}
	return nil, nil
}
func (f *fakeTraeRepo) Create(_ context.Context, c *model.TraeCredential) error {
	f.saved = append(f.saved, *c)
	return nil
}
func (f *fakeTraeRepo) Update(_ context.Context, c *model.TraeCredential) error {
	for i := range f.saved {
		if f.saved[i].Id == c.Id {
			f.saved[i] = *c
			return nil
		}
	}
	return nil
}
func (f *fakeTraeRepo) Delete(_ context.Context, id string) error {
	for i := range f.saved {
		if f.saved[i].Id == id {
			f.saved = append(f.saved[:i], f.saved[i+1:]...)
			return nil
		}
	}
	return nil
}
func (f *fakeTraeRepo) GetCheckinRecord(context.Context, string, string) (*model.TraeCheckinRecord, error) {
	return nil, nil
}
func (f *fakeTraeRepo) SaveCheckinRecord(context.Context, *model.TraeCheckinRecord) error { return nil }
func (f *fakeTraeRepo) UpdateQuota(_ context.Context, id string, total, remaining float64) error {
	for i := range f.saved {
		if f.saved[i].Id == id {
			f.saved[i].QuotaTotal = &total
			f.saved[i].QuotaRemaining = &remaining
			return nil
		}
	}
	return nil
}

var _ repository.TraeCredentialRepository = (*fakeTraeRepo)(nil)

func setupTraeRouter(t *testing.T) (*gin.Engine, *fakeTraeRepo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	repo := &fakeTraeRepo{}
	c := trae.New()
	svc := service.NewTraeLoginService(repo, c, &config.CodeBuddyConfig{TraeCallbackPort: 18080})
	h := NewTraeAuthHandler(&Handler{}, svc)

	r.POST("/api/admin/trae/login/start", h.Start)
	r.GET("/api/admin/trae/login/result", h.Result)
	r.POST("/api/admin/trae/login/cancel", h.Cancel)
	r.POST("/api/admin/trae/login/import", h.Import)
	r.GET("/authorize", h.Authorize)
	return r, repo
}

func TestTraeStartEndpoint(t *testing.T) {
	r, _ := setupTraeRouter(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/trae/login/start", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"login_url"`) || !strings.Contains(w.Body.String(), "www.trae.cn/authorization") {
		t.Errorf("missing login_url: %s", w.Body.String())
	}
}

func TestTraeResultEndpointNotFound(t *testing.T) {
	r, _ := setupTraeRouter(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/trae/login/result?pending_id=nope", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}

func TestTraeResultRoundtrip(t *testing.T) {
	r, _ := setupTraeRouter(t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/admin/trae/login/start", nil))
	var start map[string]any
	_ = json.Unmarshal([]byte(w.Body.String()), &start)
	pid, _ := start["pending_id"].(string)

	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/api/admin/trae/login/result?pending_id="+url.QueryEscape(pid), nil))
	if w2.Code != http.StatusOK || !strings.Contains(w2.Body.String(), `"pending"`) {
		t.Errorf("result: %d %s", w2.Code, w2.Body.String())
	}
}

func TestTraeImportEndpointValidation(t *testing.T) {
	r, _ := setupTraeRouter(t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/admin/trae/login/import", strings.NewReader(`{}`)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/admin/trae/login/import",
		strings.NewReader(`{"callback_url":"http://x/authorize?foo=1"}`)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid callback want 400, got %d", w.Code)
	}
}

func TestTraeAuthorizeEndpointRejectsUnknown(t *testing.T) {
	r, repo := setupTraeRouter(t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/authorize?refreshToken=rt&userInfo=%7B%7D", nil))
	if w.Code < 400 {
		t.Errorf("unknown callback should error, got %d", w.Code)
	}
	if strings.Contains(w.Header().Get("Content-Type"), "json") {
		t.Error("authorize should render HTML for browser, not JSON")
	}
	if len(repo.saved) != 0 {
		t.Error("side effect on unknown callback")
	}
}
