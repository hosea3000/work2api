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
	"github.com/yourname/work2api/internal/config"
	"github.com/yourname/work2api/internal/model"
	"github.com/yourname/work2api/internal/repository"
	"github.com/yourname/work2api/internal/service"
	"github.com/yourname/work2api/internal/upstream/trae"
)

// fakeRepo 最小仓储替身：只实现 CreateCredential，其余 panic。
type fakeRepo struct {
	saved *[]model.Credential
}

func (f *fakeRepo) CreateCredential(_ context.Context, c *model.Credential) error {
	*f.saved = append(*f.saved, *c)
	return nil
}

func (f *fakeRepo) stub() { panic("unexpected repo call") }

func (f *fakeRepo) GetAdminByUsername(context.Context, string) (*model.AdminUser, error) {
	f.stub()
	return nil, nil
}
func (f *fakeRepo) CountAdmins(context.Context) (int64, error) { f.stub(); return 0, nil }
func (f *fakeRepo) CreateAdmin(context.Context, *model.AdminUser) error {
	f.stub()
	return nil
}
func (f *fakeRepo) ListCredentials(context.Context) ([]model.Credential, error) {
	return *f.saved, nil
}
func (f *fakeRepo) MigrateCredentialColumns(context.Context) error { return nil }
func (f *fakeRepo) GetCredential(_ context.Context, id string) (*model.Credential, error) {
	for i := range *f.saved {
		if (*f.saved)[i].Id == id {
			return &(*f.saved)[i], nil
		}
	}
	return nil, nil
}
func (f *fakeRepo) GetCredentialByUserId(_ context.Context, userId string) (*model.Credential, error) {
	for i := range *f.saved {
		if (*f.saved)[i].UserId == userId {
			return &(*f.saved)[i], nil
		}
	}
	return nil, nil
}
func (f *fakeRepo) GetCredentialByAccountUid(_ context.Context, accountUid string) (*model.Credential, error) {
	for i := range *f.saved {
		if (*f.saved)[i].AccountUid != nil && *(*f.saved)[i].AccountUid == accountUid {
			return &(*f.saved)[i], nil
		}
	}
	return nil, nil
}
func (f *fakeRepo) UpdateCredential(_ context.Context, c *model.Credential) error {
	for i := range *f.saved {
		if (*f.saved)[i].Id == c.Id {
			(*f.saved)[i] = *c
			return nil
		}
	}
	return nil
}
func (f *fakeRepo) DeleteCredential(context.Context, string) error {
	f.stub()
	return nil
}
func (f *fakeRepo) ListAPIKeys(context.Context) ([]model.APIKey, error) {
	f.stub()
	return nil, nil
}
func (f *fakeRepo) GetAPIKeyByID(context.Context, string) (*model.APIKey, error) {
	f.stub()
	return nil, nil
}
func (f *fakeRepo) GetAPIKeyByHash(context.Context, string) (*model.APIKey, error) {
	f.stub()
	return nil, nil
}
func (f *fakeRepo) CreateAPIKey(context.Context, *model.APIKey) error {
	f.stub()
	return nil
}
func (f *fakeRepo) DeleteAPIKey(context.Context, string) error {
	f.stub()
	return nil
}
func (f *fakeRepo) GetCheckinRecord(context.Context, string, string) (*model.CheckinRecord, error) {
	f.stub()
	return nil, nil
}
func (f *fakeRepo) SaveCheckinRecord(context.Context, *model.CheckinRecord) error {
	f.stub()
	return nil
}

func setupTraeRouter(t *testing.T) (*gin.Engine, *[]model.Credential) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	saved := &[]model.Credential{}
	repo := &fakeRepo{}
	repo.saved = saved
	c := trae.New()
	svc := service.NewTraeLoginService(repo, nil, c, nil, &config.CodeBuddyConfig{TraeCallbackPort: 18080})
	h := NewTraeAuthHandler(&Handler{}, svc)

	r.POST("/api/admin/trae/login/start", h.Start)
	r.GET("/api/admin/trae/login/result", h.Result)
	r.POST("/api/admin/trae/login/cancel", h.Cancel)
	r.POST("/api/admin/trae/login/import", h.Import)
	r.GET("/authorize", h.Authorize)
	return r, saved
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
	_ = jsonUnmarshal(w.Body.String(), &start)
	pid, _ := start["pending_id"].(string)

	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/api/admin/trae/login/result?pending_id="+url.QueryEscape(pid), nil))
	if w2.Code != http.StatusOK || !strings.Contains(w2.Body.String(), `"pending"`) {
		t.Errorf("result: %d %s", w2.Code, w2.Body.String())
	}
}

func TestTraeImportEndpointValidation(t *testing.T) {
	r, _ := setupTraeRouter(t)
	// 缺参数 → 400
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/admin/trae/login/import", strings.NewReader(`{}`)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
	// 无效回调 → 400
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/admin/trae/login/import",
		strings.NewReader(`{"callback_url":"http://x/authorize?foo=1"}`)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid callback want 400, got %d", w.Code)
	}
}

func TestTraeAuthorizeEndpointRejectsUnknown(t *testing.T) {
	r, saved := setupTraeRouter(t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/authorize?refreshToken=rt&userInfo=%7B%7D", nil))
	// 无匹配 pending → 错误页（4xx HTML），无入库
	if w.Code < 400 {
		t.Errorf("unknown callback should error, got %d", w.Code)
	}
	if strings.Contains(w.Header().Get("Content-Type"), "json") {
		t.Error("authorize should render HTML for browser, not JSON")
	}
	if len(*saved) != 0 {
		t.Error("side effect on unknown callback")
	}
}

func jsonUnmarshal(s string, v any) error {
	return json.Unmarshal([]byte(s), v)
}

var _ repository.GatewayRepository = (*fakeRepo)(nil)
