package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/hosea3000/work2api/internal/config"
	"github.com/hosea3000/work2api/internal/model"
	"github.com/hosea3000/work2api/internal/upstream/codebuddy"
	"github.com/hosea3000/work2api/internal/upstream/trae"
)

func mkTraeSvc(t *testing.T, port int) (*TraeLoginService, *trae.Client, *fakeTraeRepo) {
	t.Helper()
	repo := &fakeTraeRepo{}
	c := trae.New()
	svc := NewTraeLoginService(repo, c, &config.CodeBuddyConfig{TraeCallbackPort: port})
	return svc, c, repo
}

func traeCallback(rt, uid string, trace string) string {
	v := url.Values{}
	v.Set("refreshToken", rt)
	v.Set("userInfo", fmt.Sprintf(`{"UserID":%q,"ScreenName":"测试用户"}`, uid))
	v.Set("userJwt", `{"Token":"at","RefreshToken":""}`)
	u := "http://127.0.0.1:18080/authorize?" + v.Encode()
	if trace != "" {
		u += "&loginTraceID=" + trace
	}
	return u
}

func TestTraeStartAndResult(t *testing.T) {
	svc, _, _ := mkTraeSvc(t, 18080)
	res, err := svc.Start()
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if res.LoginURL == "" || res.PendingID == "" {
		t.Fatalf("missing fields: %+v", res)
	}
	if !strings.Contains(res.LoginURL, "www.trae.cn/authorization") {
		t.Errorf("login url host wrong: %s", res.LoginURL)
	}
	if !strings.Contains(res.CallbackURL, "127.0.0.1:18080/authorize") {
		t.Errorf("callback url wrong: %s", res.CallbackURL)
	}
	r, ok := svc.Result(res.PendingID)
	if !ok || r.State != TraePendingActive {
		t.Errorf("expected active pending, got %+v ok=%v", r, ok)
	}
	if _, ok := svc.Result("nonexistent"); ok {
		t.Error("unknown pending should 404")
	}
}

func TestTraeCancel(t *testing.T) {
	svc, _, _ := mkTraeSvc(t, 18080)
	res, _ := svc.Start()
	if !svc.Cancel(res.PendingID) {
		t.Fatal("cancel should find pending")
	}
	if _, ok := svc.Result(res.PendingID); ok {
		t.Error("canceled pending should be gone")
	}
	if err := svc.HandleAuthorize(context.Background(), traeCallback("rt", "u1", trae.MachineTraceID("x", "y"))); err == nil {
		t.Fatal("callback after cancel must be rejected")
	}
}

func TestTraeHandleAuthorizeSuccess(t *testing.T) {
	exchanged := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case trae.EpExchange:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			exchanged, _ = body["RefreshToken"].(string)
			_, _ = w.Write([]byte(`{"Result":{"Token":"at-new","TokenExpireAt":9999999999999,"RefreshToken":"rt-rotated"}}`))
		case trae.EpUserInfo:
			_, _ = w.Write([]byte(`{"Result":{"UserID":"uid-1","ScreenName":"小明","EnterpriseID":""}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	svc, client, repo := mkTraeSvc(t, 18080)
	client.OAuthHost = srv.URL

	res, _ := svc.Start()
	svc.mu.Lock()
	pl := svc.logins[res.PendingID]
	traceID := trae.MachineTraceID(pl.MachineID, pl.DeviceID)
	machine, device := pl.MachineID, pl.DeviceID
	svc.mu.Unlock()

	if err := svc.HandleAuthorize(context.Background(), traeCallback("rt-main", "", traceID)); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if exchanged != "rt-main" {
		t.Errorf("exchange used rt %q", exchanged)
	}
	r, ok := svc.Result(res.PendingID)
	if !ok || r.State != TraePendingSuccess || r.UID != "uid-1" || r.Nickname != "小明" {
		t.Errorf("result: %+v ok=%v", r, ok)
	}
	if len(repo.saved) != 1 {
		t.Fatalf("saved %d creds", len(repo.saved))
	}
	c := repo.saved[0]
	if c.AuthSource != "web_login" {
		t.Errorf("auth_source: %s", c.AuthSource)
	}
	if c.UserId != "uid-1" {
		t.Errorf("uid: %q", c.UserId)
	}
	if c.MachineID == nil || *c.MachineID != machine || c.DeviceID == nil || *c.DeviceID != device {
		t.Errorf("machine/device not persisted: %v %v", c.MachineID, c.DeviceID)
	}
	if c.RefreshToken == nil || *c.RefreshToken != "rt-rotated" {
		t.Error("rotated refresh token not persisted")
	}
	if c.ExpiresAt == nil || *c.ExpiresAt != 9999999999 {
		t.Errorf("expires: %v", c.ExpiresAt)
	}
}

func TestTraeHandleAuthorizeExchangeFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	svc, client, repo := mkTraeSvc(t, 18080)
	client.OAuthHost = srv.URL
	res, _ := svc.Start()

	svc.mu.Lock()
	pl := svc.logins[res.PendingID]
	traceID := trae.MachineTraceID(pl.MachineID, pl.DeviceID)
	svc.mu.Unlock()

	if err := svc.HandleAuthorize(context.Background(), traeCallback("rt-bad", "", traceID)); err == nil {
		t.Fatal("want error on exchange failure")
	}
	r, _ := svc.Result(res.PendingID)
	if r.State != TraePendingFailed || r.Error == "" {
		t.Errorf("pending should be failed: %+v", r)
	}
	if len(repo.saved) != 0 {
		t.Error("no credential should be saved on failure")
	}
}

func TestTraeHandleAuthorizeUnknownTrace(t *testing.T) {
	svc, client, repo := mkTraeSvc(t, 18080)
	client.OAuthHost = "http://127.0.0.1:1"
	if err := svc.HandleAuthorize(context.Background(), traeCallback("rt", "u1", "0000000000000000")); err == nil {
		t.Fatal("unknown trace must be rejected")
	}
	if len(repo.saved) != 0 {
		t.Error("side effect on unknown callback")
	}
}

func TestTraeImport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == trae.EpExchange {
			_, _ = w.Write([]byte(`{"Result":{"Token":"at-imp","TokenExpireAt":9999999999999,"RefreshToken":"rt-imp"}}`))
		} else {
			_, _ = w.Write([]byte(`{"Result":{"UserID":"uid-9","ScreenName":"粘贴"}}`))
		}
	}))
	defer srv.Close()

	svc, client, repo := mkTraeSvc(t, 18080)
	client.OAuthHost = srv.URL

	cred, err := svc.Import(context.Background(), traeCallback("rt-paste", "", ""))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if cred.UserId != "uid-9" {
		t.Errorf("cred: %+v", cred)
	}
	if cred.MachineID == nil || len(*cred.MachineID) != 32 {
		t.Errorf("machine id should be generated hex32: %v", cred.MachineID)
	}
	if len(repo.saved) != 1 {
		t.Errorf("saved %d", len(repo.saved))
	}
	if _, err := svc.Import(context.Background(), "http://x/authorize?foo=1"); err == nil {
		t.Fatal("invalid callback should fail")
	}
}

func TestTraeResultExpires(t *testing.T) {
	svc, _, _ := mkTraeSvc(t, 18080)
	res, _ := svc.Start()
	svc.mu.Lock()
	svc.logins[res.PendingID].CreatedAt = svc.logins[res.PendingID].CreatedAt.Add(-11 * time.Minute)
	svc.mu.Unlock()
	if _, ok := svc.Result(res.PendingID); ok {
		t.Error("expired pending should be pruned")
	}
}

func TestOAuthAddDeduplicatesSameAccountUid(t *testing.T) {
	repo := &fakeCbRepo{}
	conf := &config.CodeBuddyConfig{RotationCount: 1}
	svc := NewCodeBuddyCredentialService(repo, NewCredentialPool(conf), &fakeStateRepo{}, conf, nil)

	td1 := &codebuddy.TokenData{AccessToken: "at-first", RefreshToken: "rt-first", Domain: "tencent.com"}
	acc := &codebuddy.Account{UID: "acc-1", Type: "personal"}
	if _, err := svc.AddOAuth(context.Background(), td1, acc); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if len(repo.saved) != 1 {
		t.Fatalf("first add saved %d", len(repo.saved))
	}
	firstID := repo.saved[0].Id
	if repo.saved[0].UserId != "uid_acc-1" {
		t.Errorf("user_id should be uid_acc-1, got %q", repo.saved[0].UserId)
	}

	td2 := &codebuddy.TokenData{AccessToken: "at-second", RefreshToken: "rt-second", Domain: "tencent.com"}
	v, err := svc.AddOAuth(context.Background(), td2, acc)
	if err != nil {
		t.Fatalf("second add: %v", err)
	}
	if len(repo.saved) != 1 {
		t.Fatalf("re-auth created new credential (total %d)", len(repo.saved))
	}
	if v.Id != firstID {
		t.Errorf("should reuse id %s, got %s", firstID, v.Id)
	}
	if repo.saved[0].BearerToken != "at-second" {
		t.Errorf("token not updated in place: %q", repo.saved[0].BearerToken)
	}

	if _, err := svc.AddOAuth(context.Background(), &codebuddy.TokenData{AccessToken: "at-x"}, nil); err != nil {
		t.Fatalf("fallback add: %v", err)
	}
	if len(repo.saved) != 2 {
		t.Fatalf("fallback should create new (total %d)", len(repo.saved))
	}
}

func TestTraeImportDeduplicatesSameUid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == trae.EpExchange {
			_, _ = w.Write([]byte(`{"Result":{"Token":"at-2","TokenExpireAt":9999999999999,"RefreshToken":"rt-2"}}`))
		} else {
			_, _ = w.Write([]byte(`{"Result":{"UserID":"uid-dup","ScreenName":"复登"}}`))
		}
	}))
	defer srv.Close()

	svc, client, repo := mkTraeSvc(t, 18080)
	client.OAuthHost = srv.URL

	if _, err := svc.Import(context.Background(), traeCallback("rt-1", "", "")); err != nil {
		t.Fatalf("first import: %v", err)
	}
	if len(repo.saved) != 1 {
		t.Fatalf("first import saved %d", len(repo.saved))
	}
	firstID := repo.saved[0].Id

	cred, err := svc.Import(context.Background(), traeCallback("rt-again", "", ""))
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if len(repo.saved) != 1 {
		t.Fatalf("duplicate login created new credential (total %d)", len(repo.saved))
	}
	if cred.Id != firstID {
		t.Errorf("should reuse existing id %s, got %s", firstID, cred.Id)
	}
	if *cred.RefreshToken != "rt-2" || cred.BearerToken != "at-2" {
		t.Errorf("token not refreshed in place: %+v", cred)
	}
}

var _ = model.TraeCredential{}
