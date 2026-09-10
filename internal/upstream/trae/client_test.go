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

func TestSOLOHeaders(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "http://x", nil)
	SOLOHeaders(req, "at-1", "u-1", "m-1", "d-1", true)
	checks := map[string]string{
		"Authorization":        "Cloud-IDE-JWT at-1",
		"X-Cloudide-Token":     "at-1",
		"X-Ide-Token":          "at-1",
		"X-Uid":                "u-1",
		"X-Machine-Id":         "m-1",
		"X-Device-Id":          "d-1",
		"X-App-Id":             AppID,
		"X-Ide-Version":        IdeVersion,
		"X-Ide-Version-Code":   IdeVersionCode,
		"Request-Traffic-Type": "prod",
		"Accept":               "text/event-stream",
	}
	for k, want := range checks {
		if got := req.Header.Get(k); got != want {
			t.Errorf("%s=%q want %q", k, got, want)
		}
	}
}

func TestFetchModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != EpModels {
			t.Errorf("path=%s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Cloud-IDE-JWT at-1" {
			t.Error("missing SOLO auth header")
		}
		_, _ = w.Write([]byte(`{"config_info_list":[
			{"config_name":"glm-5.2","display_config":{"display_name":"GLM-5.2"}},
			{"config_name":"glm-5.3","display_config":{"display_name":"GLM-5.3"}},
			{"config_name":""}
		]}`))
	}))
	defer srv.Close()

	c := New()
	c.AgentHost = srv.URL
	models, err := c.FetchModels("at-1", "u-1", "m-1", "d-1")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(models) != 2 || models[0].ID != "glm-5.2" || models[1].Name != "GLM-5.3" {
		t.Errorf("models=%+v", models)
	}
}

func TestFetchModelsEmptyErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"config_info_list":[]}`))
	}))
	defer srv.Close()
	c := New()
	c.AgentHost = srv.URL
	if _, err := c.FetchModels("at", "u", "m", "d"); err == nil {
		t.Fatal("want error for empty model list")
	}
}
