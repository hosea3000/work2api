package trae

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckinStatusSuccess(t *testing.T) {
	var gotAuth, gotDevice, gotRegion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != EpCheckinStatus {
			t.Errorf("path=%s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotDevice = r.Header.Get("X-Device-Id")
		gotRegion = r.Header.Get("X-User-Region")
		_, _ = w.Write([]byte(`{"checked_in":true,"credits":200,"enable":true}`))
	}))
	defer srv.Close()

	c := New()
	c.UgHost = srv.URL
	checkedIn, credits, enable, err := c.CheckinStatus(context.Background(), "at-1", "dev-1")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !checkedIn || credits != 200 || !enable {
		t.Errorf("status result: %v %d %v", checkedIn, credits, enable)
	}
	if gotAuth != "Cloud-IDE-JWT at-1" || gotDevice != "dev-1" || gotRegion != "CN" {
		t.Errorf("headers: %q %q %q", gotAuth, gotDevice, gotRegion)
	}
}

func TestCheckinClaimSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != EpCheckinClaim {
			t.Errorf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"code":0,"message":"ok"}`))
	}))
	defer srv.Close()

	c := New()
	c.UgHost = srv.URL
	res, err := c.CheckinClaim(context.Background(), "at-1", "dev-1")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if !res.Success || res.Code == nil || *res.Code != 0 {
		t.Errorf("claim result: %+v", res)
	}
}

func TestCheckinClaimTooManyUsers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":9074,"message":"当前使用人数太多"}`))
	}))
	defer srv.Close()

	c := New()
	c.UgHost = srv.URL
	res, err := c.CheckinClaim(context.Background(), "at-1", "dev-1")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if res.Success || res.Code == nil || *res.Code != 9074 || !strings.Contains(res.Message, "人数太多") {
		t.Errorf("claim result: %+v", res)
	}
}

func TestCheckinClaimSchemaFallback(t *testing.T) {
	// ponytail 降级路径：响应体不含可解析 code（schema 不符）→ HTTP 200 即成功
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`ok`))
	}))
	defer srv.Close()

	c := New()
	c.UgHost = srv.URL
	res, err := c.CheckinClaim(context.Background(), "at-1", "dev-1")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if !res.Success || res.Code != nil {
		t.Errorf("claim result: %+v", res)
	}
}

func TestCheckinStatusUpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := New()
	c.UgHost = srv.URL
	if _, _, _, err := c.CheckinStatus(context.Background(), "at-1", "dev-1"); err == nil {
		t.Fatal("want error for 401")
	}
}

func TestEntUsageAggregation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != EpEntUsage {
			t.Errorf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"user_entitlement_pack_list":[
			{"entitlement_base_info":{"quota":{"credits_limit":2000}},"usage":{"credits_amount":300}},
			{"entitlement_base_info":{"quota":{"credits_limit":500}},"usage":{"credits_amount":100.5}}
		]}`))
	}))
	defer srv.Close()

	c := New()
	c.UgHost = srv.URL
	remain, limit, used, err := c.EntUsage(context.Background(), "at-1", "dev-1")
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if limit != 2500 || used != 400 || remain != 2100 {
		t.Errorf("usage: remain=%d limit=%d used=%d", remain, limit, used)
	}
}

func TestEntUsageEmptyPacks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"user_entitlement_pack_list":[]}`))
	}))
	defer srv.Close()

	c := New()
	c.UgHost = srv.URL
	remain, limit, used, err := c.EntUsage(context.Background(), "at-1", "dev-1")
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if remain != 0 || limit != 0 || used != 0 {
		t.Errorf("usage: remain=%d limit=%d used=%d", remain, limit, used)
	}
}
