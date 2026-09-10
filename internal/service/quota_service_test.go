package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hosea3000/work2api/internal/model"
	"github.com/hosea3000/work2api/internal/upstream/codebuddy"
	"github.com/hosea3000/work2api/internal/upstream/trae"
)

func cbQuotaService(t *testing.T, repo *fakeCbRepo, srv *httptest.Server) QuotaService {
	t.Helper()
	return NewQuotaService(repo, &fakeTraeRepo{}, codebuddy.NewClient(srv.URL, ""), trae.New())
}

func TestQuotaServiceRefreshCodeBuddy(t *testing.T) {
	repo := &fakeCbRepo{saved: []model.CodeBuddyCredential{newCbCred("cb1", "active")}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"Response":{"Data":{"Accounts":[{"Status":0,"CycleCapacitySize":2000,"CycleCapacityRemain":1700}]}}}}`))
	}))
	defer srv.Close()

	q, err := cbQuotaService(t, repo, srv).Refresh(context.Background(), "codebuddy", "cb1")
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if q.Total != 2000 || q.Remaining != 1700 {
		t.Errorf("quota=%+v want 2000/1700", q)
	}
	if repo.saved[0].QuotaTotal == nil || *repo.saved[0].QuotaTotal != 2000 || *repo.saved[0].QuotaRemaining != 1700 {
		t.Errorf("repo not updated: %+v", repo.saved[0])
	}
}

func TestQuotaServiceRefreshFailureKeepsOldValue(t *testing.T) {
	old := 100.0
	oldRemain := 50.0
	cred := newCbCred("cb1", "active")
	cred.QuotaTotal = &old
	cred.QuotaRemaining = &oldRemain
	repo := &fakeCbRepo{saved: []model.CodeBuddyCredential{cred}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if _, err := cbQuotaService(t, repo, srv).Refresh(context.Background(), "codebuddy", "cb1"); err == nil {
		t.Fatal("expected error")
	}
	if *repo.saved[0].QuotaTotal != old || *repo.saved[0].QuotaRemaining != oldRemain {
		t.Errorf("old value overwritten: %+v", repo.saved[0])
	}
}

func TestQuotaServiceRefreshEnterpriseSkipped(t *testing.T) {
	ent := "ent-1"
	cred := newCbCred("cb1", "active")
	cred.EnterpriseId = &ent
	repo := &fakeCbRepo{saved: []model.CodeBuddyCredential{cred}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("enterprise credential must not probe upstream")
	}))
	defer srv.Close()

	_, err := cbQuotaService(t, repo, srv).Refresh(context.Background(), "codebuddy", "cb1")
	if !errors.Is(err, ErrQuotaSkipped) {
		t.Fatalf("err=%v want ErrQuotaSkipped", err)
	}
	if repo.saved[0].QuotaTotal != nil {
		t.Errorf("enterprise quota should stay NULL: %+v", repo.saved[0])
	}
}

func TestQuotaServiceRefreshTrae(t *testing.T) {
	repo := &fakeTraeRepo{saved: []model.TraeCredential{newTraeCred("tr1", "active")}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"user_entitlement_pack_list":[{"entitlement_base_info":{"quota":{"credits_limit":2000}},"usage":{"credits_amount":300}}]}`))
	}))
	defer srv.Close()

	client := trae.New()
	client.UgHost = srv.URL
	svc := NewQuotaService(&fakeCbRepo{}, repo, codebuddy.NewClient("http://127.0.0.1:0", ""), client)

	q, err := svc.Refresh(context.Background(), "trae", "tr1")
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if q.Total != 2000 || q.Remaining != 1700 {
		t.Errorf("quota=%+v want 2000/1700", q)
	}
	if repo.saved[0].QuotaRemaining == nil || *repo.saved[0].QuotaRemaining != 1700 {
		t.Errorf("repo not updated: %+v", repo.saved[0])
	}
}
