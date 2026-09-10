package codebuddy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func quotaClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, "")
	return c
}

func testSnap() CredentialSnapshot {
	return CredentialSnapshot{BearerToken: "tok-1", UserID: "user-1"}
}

func TestFetchQuotaPersonalAggregates(t *testing.T) {
	c := quotaClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/billing/meter/get-user-resource" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"Response":{"Data":{"Accounts":[
			{"Status":0,"PackageName":"A","CycleCapacitySizePrecise":"2000","CycleCapacityRemainPrecise":"1700.5"},
			{"Status":0,"PackageName":"B","CycleCapacitySize":500,"CycleCapacityRemain":100},
			{"Status":3,"PackageName":"C","CycleCapacitySize":9999,"CycleCapacityRemain":9999}
		]}}}}`))
	})
	total, remaining, err := c.FetchQuotaPersonal(context.Background(), testSnap())
	if err != nil {
		t.Fatalf("quota: %v", err)
	}
	if total != 2500 || remaining != 1800.5 {
		t.Errorf("total=%v remaining=%v want 2500 / 1800.5", total, remaining)
	}
}

func TestFetchQuotaPersonalEmptyAccounts(t *testing.T) {
	c := quotaClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"Response":{"Data":{"Accounts":[]}}}}`))
	})
	total, remaining, err := c.FetchQuotaPersonal(context.Background(), testSnap())
	if err != nil {
		t.Fatalf("quota: %v", err)
	}
	if total != 0 || remaining != 0 {
		t.Errorf("total=%v remaining=%v want 0/0", total, remaining)
	}
}

func TestFetchQuotaPersonalNon200(t *testing.T) {
	c := quotaClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	})
	if _, _, err := c.FetchQuotaPersonal(context.Background(), testSnap()); err == nil {
		t.Fatal("expected error on non-200")
	}
}

func TestFetchQuotaPersonalInvalidJSON(t *testing.T) {
	c := quotaClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	})
	if _, _, err := c.FetchQuotaPersonal(context.Background(), testSnap()); err == nil {
		t.Fatal("expected error on invalid json")
	}
}
