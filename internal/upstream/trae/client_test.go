package trae

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestExchangeTokenSuccess(t *testing.T) {
	futureSec := time.Now().Add(24 * time.Hour).Unix()
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Result":{"Token":"at-new","TokenExpireAt":` +
			strconv.FormatInt(futureSec*1000, 10) + `,"RefreshToken":"rt-new"}}`))
	}))
	defer srv.Close()

	c := New()
	c.OAuthHost = srv.URL
	pair, err := c.ExchangeToken("rt-old", "")
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if gotPath != EpExchange {
		t.Errorf("path=%s", gotPath)
	}
	if pair.AccessToken != "at-new" || pair.RefreshToken != "rt-new" {
		t.Errorf("pair: %+v", pair)
	}
	if pair.ExpiresAt != futureSec {
		t.Errorf("expires=%d want %d", pair.ExpiresAt, futureSec)
	}
	if gotBody["RefreshToken"] != "rt-old" || gotBody["ClientID"] != ClientID {
		t.Errorf("request body: %v", gotBody)
	}
}

func TestExchangeTokenEmptyRefresh(t *testing.T) {
	c := New()
	if _, err := c.ExchangeToken("", ""); err == nil {
		t.Fatal("want error for empty refreshToken")
	}
}

func TestExchangeTokenUpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":1001}`))
	}))
	defer srv.Close()

	c := New()
	c.OAuthHost = srv.URL
	if _, err := c.ExchangeToken("rt", ""); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("want 401 error, got %v", err)
	}
}

func TestExchangeTokenNoTokenInResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"Result":{}}`))
	}))
	defer srv.Close()

	c := New()
	c.OAuthHost = srv.URL
	if _, err := c.ExchangeToken("rt", ""); err == nil || !strings.Contains(err.Error(), "re-login") {
		t.Fatalf("want re-login error, got %v", err)
	}
}

func TestGetUserInfoSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Cloudide-Token") != "at-x" {
			t.Error("missing X-Cloudide-Token header")
		}
		_, _ = w.Write([]byte(`{"Result":{"UserID":"u1","ScreenName":"张三","EnterpriseID":"ent-9"}}`))
	}))
	defer srv.Close()

	c := New()
	c.OAuthHost = srv.URL
	info, err := c.GetUserInfo("at-x", "")
	if err != nil {
		t.Fatalf("userinfo: %v", err)
	}
	if info.UID != "u1" || info.Nickname != "张三" || info.EnterpriseID != "ent-9" {
		t.Errorf("info: %+v", info)
	}
}
