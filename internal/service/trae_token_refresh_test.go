package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yourname/work2api/internal/model"
	"github.com/yourname/work2api/internal/upstream/trae"
)

func mkTraeRefreshCred(id string, expiresAt int64) *model.TraeCredential {
	rt := "rt-" + id
	return &model.TraeCredential{
		Id:           id,
		BearerToken:  "at-" + id,
		AuthSource:   "web_login",
		Status:       "active",
		RefreshToken: &rt,
		ExpiresAt:    &expiresAt,
	}
}

func TestShouldTraeRefreshMatrix(t *testing.T) {
	now := time.Now().Unix()
	otherSrc := mkTraeRefreshCred("t2", now+3600)
	otherSrc.AuthSource = "oauth"
	noRT := mkTraeRefreshCred("t3", now+3600)
	noRT.RefreshToken = nil
	noExp := mkTraeRefreshCred("t4", now+3600)
	noExp.ExpiresAt = nil

	cases := []struct {
		name string
		cred *model.TraeCredential
		want bool
	}{
		{"临期(差1h)→刷", mkTraeRefreshCred("t1", now+3600), true},
		{"已过期→刷(临期)", mkTraeRefreshCred("t1", now-10), true},
		{"离过期尚远→不刷", mkTraeRefreshCred("t1", now+72*3600), false},
		{"非web_login→不刷", otherSrc, false},
		{"无refresh_token→不刷", noRT, false},
		{"无expires_at→不刷", noExp, false},
	}
	for _, tc := range cases {
		if got := ShouldTraeRefresh(tc.cred, now); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
	if ShouldTraeRefresh(nil, now) {
		t.Error("nil credential must not refresh")
	}
}

func setupTraeRefreshSvc(t *testing.T, exchangeResp string, status int) (*TraeTokenRefreshService, *fakeTraeRepo) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(exchangeResp))
	}))
	t.Cleanup(srv.Close)

	c := trae.New()
	c.OAuthHost = srv.URL
	repo := &fakeTraeRepo{}
	svc := NewTraeCredentialService(repo, NewTraeCredentialPool(), &fakeStateRepo{}, c)
	refresh := NewTraeTokenRefreshService(svc, c)
	refresh.jitterMinMs, refresh.jitterMaxMs = 0, 0
	return refresh, repo
}

func TestTraeRefreshScanWritesBack(t *testing.T) {
	futureSec := time.Now().Add(48 * time.Hour).Unix()
	svc, repo := setupTraeRefreshSvc(t,
		`{"Result":{"Token":"at-new","RefreshToken":"rt-new","TokenExpireAt":`+i64Str(futureSec*1000)+`}}`, 200)

	repo.saved = append(repo.saved, *mkTraeRefreshCred("t-scan", time.Now().Unix()+3600))
	svc.scanOnce(context.Background())

	if len(repo.saved) != 1 {
		t.Fatal("credential missing")
	}
	got := repo.saved[0]
	if got.BearerToken != "at-new" {
		t.Errorf("bearer_token=%s", got.BearerToken)
	}
	if got.RefreshToken == nil || *got.RefreshToken != "rt-new" {
		t.Errorf("refresh_token=%v", got.RefreshToken)
	}
	if got.ExpiresAt == nil || *got.ExpiresAt != futureSec {
		t.Errorf("expires_at=%v want %d", got.ExpiresAt, futureSec)
	}
	if got.LastRefreshAt == nil {
		t.Error("last_refresh_at not set")
	}
}

func TestTraeRefreshKeepsOldRTWhenAbsent(t *testing.T) {
	futureSec := time.Now().Add(48 * time.Hour).Unix()
	svc, repo := setupTraeRefreshSvc(t,
		`{"Result":{"Token":"at-new","TokenExpireAt":`+i64Str(futureSec*1000)+`}}`, 200)

	repo.saved = append(repo.saved, *mkTraeRefreshCred("t-keep", time.Now().Unix()+3600))
	svc.scanOnce(context.Background())

	got := repo.saved[0]
	if got.RefreshToken == nil || *got.RefreshToken != "rt-t-keep" {
		t.Errorf("old refresh_token should be kept, got %v", got.RefreshToken)
	}
	if got.BearerToken != "at-new" {
		t.Errorf("bearer_token=%s", got.BearerToken)
	}
}

func TestTraeRefreshMarksExpiredOnRejection(t *testing.T) {
	svc, repo := setupTraeRefreshSvc(t, `{"error":"invalid"}`, http.StatusUnauthorized)

	repo.saved = append(repo.saved, *mkTraeRefreshCred("t-dead", time.Now().Unix()+3600))
	svc.scanOnce(context.Background())

	if repo.saved[0].Status != "expired" {
		t.Errorf("status=%s want expired", repo.saved[0].Status)
	}
	if repo.saved[0].BearerToken != "at-t-dead" {
		t.Errorf("bearer_token should be untouched, got %s", repo.saved[0].BearerToken)
	}
}

func TestTraeRefreshScanSkipsFresh(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"Result":{"Token":"x"}}`))
	}))
	defer srv.Close()

	c := trae.New()
	c.OAuthHost = srv.URL
	repo := &fakeTraeRepo{}
	svc := NewTraeCredentialService(repo, NewTraeCredentialPool(), &fakeStateRepo{}, c)
	refresh := NewTraeTokenRefreshService(svc, c)
	refresh.jitterMinMs, refresh.jitterMaxMs = 0, 0

	repo.saved = append(repo.saved,
		*mkTraeRefreshCred("t-fresh", time.Now().Unix()+72*3600),
		*mkTraeRefreshCred("t-due", time.Now().Unix()+3600),
	)
	refresh.scanOnce(context.Background())

	if requests != 1 {
		t.Errorf("upstream requests=%d want 1 (only due)", requests)
	}
}

func i64Str(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}
