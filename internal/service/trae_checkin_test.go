package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hosea3000/work2api/internal/config"
	"github.com/hosea3000/work2api/internal/model"
	"github.com/hosea3000/work2api/internal/upstream/trae"
)

func setupTraeCheckinSvc(t *testing.T, respond map[string]string, paths *[]string) (TraeCheckinService, *fakeTraeRepo) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*paths = append(*paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		for prefix, body := range respond {
			if strings.HasPrefix(r.URL.Path, prefix) {
				_, _ = w.Write([]byte(body))
				return
			}
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	c := trae.New()
	c.UgHost = srv.URL
	c.OAuthHost = srv.URL

	repo := &fakeTraeRepo{}
	creds := NewTraeCredentialService(repo, NewTraeCredentialPool(), &fakeStateRepo{}, c)
	return NewTraeCheckinService(repo, creds, c, &config.CodeBuddyConfig{}), repo
}

func traeCheckinCred() model.TraeCredential {
	device := "dev-1"
	return model.TraeCredential{Id: "t-ck", BearerToken: "at-ck", AuthSource: "web_login", Status: "active", DeviceID: &device}
}

func TestTraeManualCheckinClaimSuccess(t *testing.T) {
	var paths []string
	svc, repo := setupTraeCheckinSvc(t, map[string]string{
		"/trae/api/v2/ug/checkin_credits/status": `{"checked_in":false,"enable":true}`,
		"/trae/api/v2/ug/checkin_credits/claim":  `{"code":0,"message":"ok"}`,
		"/trae/api/v2/pay/ide_user_ent_usage":    `{"user_entitlement_pack_list":[{"entitlement_base_info":{"quota":{"credits_limit":2000}},"usage":{"credits_amount":300}}]}`,
	}, &paths)
	repo.saved = append(repo.saved, traeCheckinCred())

	res, err := svc.ManualCheckin(context.Background(), "t-ck")
	if err != nil {
		t.Fatalf("manual checkin: %v", err)
	}
	if !res["success"].(bool) {
		t.Errorf("want success, got %+v", res)
	}
	if cr, ok := res["credit"].(*float64); !ok || cr == nil || *cr != 1700 {
		t.Errorf("credit=%v want 1700", res["credit"])
	}
	if len(paths) != 3 || !strings.Contains(paths[0], "status") || !strings.Contains(paths[1], "claim") {
		t.Errorf("paths=%v", paths)
	}
	if len(repo.records) != 1 {
		t.Fatalf("checkin record not saved: %d", len(repo.records))
	}
}

func TestTraeManualCheckinAlreadyCheckedIn(t *testing.T) {
	var paths []string
	svc, repo := setupTraeCheckinSvc(t, map[string]string{
		"/trae/api/v2/ug/checkin_credits/status": `{"checked_in":true,"enable":true}`,
		"/trae/api/v2/ug/checkin_credits/claim":  `{"code":0,"message":"ok"}`,
	}, &paths)
	repo.saved = append(repo.saved, traeCheckinCred())

	res, err := svc.ManualCheckin(context.Background(), "t-ck")
	if err != nil {
		t.Fatalf("manual checkin: %v", err)
	}
	if !res["success"].(bool) {
		t.Errorf("want idempotent success, got %+v", res)
	}
	if len(paths) != 2 || !strings.Contains(paths[0], "status") || !strings.Contains(paths[1], "ent_usage") {
		t.Errorf("claim must be skipped when already checked in, paths=%v", paths)
	}
}

func TestTraeManualCheckinTooManyUsers(t *testing.T) {
	var paths []string
	svc, repo := setupTraeCheckinSvc(t, map[string]string{
		"/trae/api/v2/ug/checkin_credits/status": `{"checked_in":false,"enable":true}`,
		"/trae/api/v2/ug/checkin_credits/claim":  `{"code":9074,"message":"当前使用人数太多"}`,
	}, &paths)
	repo.saved = append(repo.saved, traeCheckinCred())

	res, err := svc.ManualCheckin(context.Background(), "t-ck")
	if err != nil {
		t.Fatalf("manual checkin: %v", err)
	}
	if res["success"].(bool) {
		t.Errorf("9074 must be failure, got %+v", res)
	}
	if code, ok := res["code"].(*int); !ok || code == nil || *code != 9074 {
		t.Errorf("code=%v want 9074", res["code"])
	}
}

func TestTraeScheduledCheckinRecordsToTraeTable(t *testing.T) {
	var paths []string
	svc, repo := setupTraeCheckinSvc(t, map[string]string{
		"/trae/api/v2/ug/checkin_credits/status": `{"checked_in":false,"enable":true}`,
		"/trae/api/v2/ug/checkin_credits/claim":  `{"code":0,"message":"ok"}`,
	}, &paths)
	repo.saved = append(repo.saved, traeCheckinCred())

	svc.RunScheduledCheckin(context.Background())

	if len(paths) != 3 {
		t.Errorf("paths=%v want status+claim+usage", paths)
	}
	if len(repo.records) != 1 {
		t.Errorf("trae checkin record not saved: %d", len(repo.records))
	}
}

func TestTraeScheduledCheckinSecondRunSkipsClaim(t *testing.T) {
	var paths []string
	var checked atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "status") {
			if checked.Load() {
				_, _ = w.Write([]byte(`{"checked_in":true,"enable":true}`))
			} else {
				_, _ = w.Write([]byte(`{"checked_in":false,"enable":true}`))
			}
			return
		}
		checked.Store(true)
		_, _ = w.Write([]byte(`{"code":0,"message":"ok"}`))
	}))
	t.Cleanup(srv.Close)

	c := trae.New()
	c.UgHost = srv.URL
	repo := &fakeTraeRepo{}
	creds := NewTraeCredentialService(repo, NewTraeCredentialPool(), &fakeStateRepo{}, c)
	svc := NewTraeCheckinService(repo, creds, c, &config.CodeBuddyConfig{})
	repo.saved = append(repo.saved, traeCheckinCred())

	svc.RunScheduledCheckin(context.Background())
	svc.RunScheduledCheckin(context.Background())

	claims := 0
	for _, p := range paths {
		if strings.Contains(p, "claim") {
			claims++
		}
	}
	if claims != 1 || len(paths) != 5 {
		t.Errorf("paths=%v claims=%d want 5 paths, 1 claim", paths, claims)
	}
}

func TestTraeCredentialTestProbe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("X-Cloudide-Token") == "at-ck" {
			_, _ = w.Write([]byte(`{"Result":{"UserID":"u-9","ScreenName":"张三"}}`))
		} else {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":1001}`))
		}
	}))
	t.Cleanup(srv.Close)

	c := trae.New()
	c.OAuthHost = srv.URL
	repo := &fakeTraeRepo{}
	svc := NewTraeCredentialService(repo, NewTraeCredentialPool(), &fakeStateRepo{}, c)
	repo.saved = append(repo.saved, traeCheckinCred())

	ok, statusCode, detail := svc.Test(context.Background(), "t-ck")
	if !ok || statusCode != 200 || !strings.Contains(detail, "u-9") {
		t.Errorf("ok=%v status=%d detail=%q", ok, statusCode, detail)
	}

	device := "dev-1"
	repo.saved = append(repo.saved, model.TraeCredential{Id: "t-dead", BearerToken: "wrong", AuthSource: "web_login", Status: "active", DeviceID: &device})
	ok, statusCode, _ = svc.Test(context.Background(), "t-dead")
	if ok || statusCode != 502 {
		t.Errorf("dead: ok=%v status=%d", ok, statusCode)
	}
	if repo.saved[1].Status != "expired" {
		t.Errorf("dead status=%s want expired", repo.saved[1].Status)
	}
}
