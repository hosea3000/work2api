package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/yourname/work2api/internal/service"
)

func TestAPIKeyAuthRejects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/chat", APIKeyAuth(nil, &fakeAPIKeys{valid: false}), func(c *gin.Context) { c.Status(200) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/chat", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("no auth should 401, got %d", w.Code)
	}
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/chat", nil)
	req.Header.Set("Authorization", "Basic abc")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("basic auth should 401")
	}
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/chat", nil)
	req.Header.Set("Authorization", "Bearer sk-invalid")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("invalid key should 401")
	}
	if w.Body.String() == "" {
		t.Error("body should contain error")
	}
}

func TestSessionAuthRejectsAndAccepts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sessions := &fakeSessions{valid: true}
	r := gin.New()
	r.GET("/admin", SessionAuth(nil, sessions), func(c *gin.Context) { c.Status(200) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/admin", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("no cookie should 401, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/admin", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "tok"})
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("valid cookie should 200, got %d", w.Code)
	}
}

type fakeAPIKeys struct{ valid bool }

func (f *fakeAPIKeys) Create(ctx context.Context, name string) (*service.APIKeyCreateResult, error) {
	return nil, nil
}
func (f *fakeAPIKeys) List(ctx context.Context) ([]service.APIKeyView, error) { return nil, nil }
func (f *fakeAPIKeys) Delete(ctx context.Context, id string) error            { return nil }
func (f *fakeAPIKeys) Validate(ctx context.Context, raw string) (bool, error) { return f.valid, nil }

type fakeSessions struct{ valid bool }

func (f *fakeSessions) Login(ctx context.Context, username, password string) (string, bool, error) {
	return "", false, nil
}
func (f *fakeSessions) Logout(ctx context.Context, token string) error { return nil }
func (f *fakeSessions) Validate(ctx context.Context, token string) (string, bool) {
	return "admin", f.valid
}
func (f *fakeSessions) BootstrapIfEmpty(ctx context.Context, username, password string) error {
	return nil
}
