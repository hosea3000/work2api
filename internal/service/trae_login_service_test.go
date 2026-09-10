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

	"github.com/yourname/work2api/internal/config"
	"github.com/yourname/work2api/internal/model"
	"github.com/yourname/work2api/internal/repository"
	"github.com/yourname/work2api/internal/upstream/codebuddy"
	"github.com/yourname/work2api/internal/upstream/trae"
)

// fakeCredRepo 其余方法 panic 防意外调用。
func (f *fakeCredRepo) stub() { panic("unexpected repo call") }

func (f *fakeCredRepo) GetAdminByUsername(context.Context, string) (*model.AdminUser, error) {
	f.stub()
	return nil, nil
}
func (f *fakeCredRepo) CountAdmins(context.Context) (int64, error) { f.stub(); return 0, nil }
func (f *fakeCredRepo) CreateAdmin(context.Context, *model.AdminUser) error {
	panic("unexpected repo call")
}
func (f *fakeCredRepo) ListCredentials(context.Context) ([]model.Credential, error) {
	return *f.saved, nil
}
func (f *fakeCredRepo) MigrateCredentialColumns(context.Context) error { return nil }
func (f *fakeCredRepo) GetCredential(_ context.Context, id string) (*model.Credential, error) {
	for i := range *f.saved {
		if (*f.saved)[i].Id == id {
			return &(*f.saved)[i], nil
		}
	}
	return nil, nil
}
func (f *fakeCredRepo) GetCredentialByUserId(_ context.Context, userId string) (*model.Credential, error) {
	for i := range *f.saved {
		if (*f.saved)[i].UserId == userId {
			return &(*f.saved)[i], nil
		}
	}
	return nil, nil
}
func (f *fakeCredRepo) GetCredentialByAccountUid(_ context.Context, accountUid string) (*model.Credential, error) {
	for i := range *f.saved {
		if (*f.saved)[i].AccountUid != nil && *(*f.saved)[i].AccountUid == accountUid {
			return &(*f.saved)[i], nil
		}
	}
	return nil, nil
}
func (f *fakeCredRepo) UpdateCredential(_ context.Context, c *model.Credential) error {
	for i := range *f.saved {
		if (*f.saved)[i].Id == c.Id {
			(*f.saved)[i] = *c
			return nil
		}
	}
	return nil
}
func (f *fakeCredRepo) DeleteCredential(context.Context, string) error { panic("unexpected repo call") }
func (f *fakeCredRepo) ListAPIKeys(context.Context) ([]model.APIKey, error) {
	f.stub()
	return nil, nil
}
func (f *fakeCredRepo) GetAPIKeyByID(context.Context, string) (*model.APIKey, error) {
	f.stub()
	return nil, nil
}
func (f *fakeCredRepo) GetAPIKeyByHash(context.Context, string) (*model.APIKey, error) {
	f.stub()
	return nil, nil
}
func (f *fakeCredRepo) CreateAPIKey(context.Context, *model.APIKey) error {
	panic("unexpected repo call")
}
func (f *fakeCredRepo) DeleteAPIKey(context.Context, string) error { panic("unexpected repo call") }
func (f *fakeCredRepo) GetCheckinRecord(context.Context, string, string) (*model.CheckinRecord, error) {
	f.stub()
	return nil, nil
}
func (f *fakeCredRepo) SaveCheckinRecord(context.Context, *model.CheckinRecord) error {
	panic("unexpected repo call")
}

// fakeCredRepo 仅实现登录服务用到的 CreateCredential（其余 panic 防意外调用）。
type fakeCredRepo struct {
	saved *[]model.Credential
}

func (f *fakeCredRepo) CreateCredential(_ context.Context, c *model.Credential) error {
	*f.saved = append(*f.saved, *c)
	return nil
}

func mkTraeSvc(t *testing.T, port int) (*TraeLoginService, *trae.Client, *[]model.Credential) {
	t.Helper()
	saved := &[]model.Credential{}
	repo := &fakeCredRepo{saved: saved}
	c := trae.New()
	svc := NewTraeLoginService(repo, nil, c, nil, &config.CodeBuddyConfig{TraeCallbackPort: port})
	return svc, c, saved
}

var _ repository.GatewayRepository = (*fakeCredRepo)(nil)

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
	// 回调到达：无 pending → 拒绝
	if err := svc.HandleAuthorize(context.Background(), traeCallback("rt", "u1", trae.MachineTraceID("x", "y"))); err == nil {
		t.Fatal("callback after cancel must be rejected")
	}
}

func TestTraeHandleAuthorizeSuccess(t *testing.T) {
	// 上游假服务器：ExchangeToken + GetUserInfo
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

	svc, client, saved := mkTraeSvc(t, 18080)
	client.OAuthHost = srv.URL

	res, _ := svc.Start()
	trace := trae.MachineTraceID("", "")
	_ = trace
	// 从 pending 反推 trace
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
	if len(*saved) != 1 {
		t.Fatalf("saved %d creds", len(*saved))
	}
	c := (*saved)[0]
	if c.Provider != "trae" || c.AuthSource != "web_login" {
		t.Errorf("provider/auth_source: %s/%s", c.Provider, c.AuthSource)
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

	svc, client, saved := mkTraeSvc(t, 18080)
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
	if len(*saved) != 0 {
		t.Error("no credential should be saved on failure")
	}
}

func TestTraeHandleAuthorizeUnknownTrace(t *testing.T) {
	svc, client, saved := mkTraeSvc(t, 18080)
	client.OAuthHost = "http://127.0.0.1:1" // 不可达，任何意外调用都会失败
	if err := svc.HandleAuthorize(context.Background(), traeCallback("rt", "u1", "0000000000000000")); err == nil {
		t.Fatal("unknown trace must be rejected")
	}
	if len(*saved) != 0 {
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

	svc, client, saved := mkTraeSvc(t, 18080)
	client.OAuthHost = srv.URL

	cred, err := svc.Import(context.Background(), traeCallback("rt-paste", "", ""))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if cred.Provider != "trae" || cred.UserId != "uid-9" {
		t.Errorf("cred: %+v", cred)
	}
	if cred.MachineID == nil || len(*cred.MachineID) != 32 {
		t.Errorf("machine id should be generated hex32: %v", cred.MachineID)
	}
	if len(*saved) != 1 {
		t.Errorf("saved %d", len(*saved))
	}
	// 无效回调 URL
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
	// 同 account_uid 重复 OAuth 认证 → 原位更新（复用 uid_<account_uid> 稳定 key），不新增
	saved := &[]model.Credential{}
	repo := &fakeCredRepo{saved: saved}
	conf := &config.CodeBuddyConfig{RotationCount: 1}
	svc := &credentialService{repo: repo, pool: NewCredentialPool(conf)}

	td1 := &codebuddy.TokenData{AccessToken: "at-first", RefreshToken: "rt-first", Domain: "tencent.com"}
	acc := &codebuddy.Account{UID: "acc-1", Type: "personal"}
	if _, err := svc.AddOAuth(context.Background(), td1, acc); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if len(*saved) != 1 {
		t.Fatalf("first add saved %d", len(*saved))
	}
	firstID := (*saved)[0].Id
	if (*saved)[0].UserId != "uid_acc-1" {
		t.Errorf("user_id should be uid_acc-1, got %q", (*saved)[0].UserId)
	}

	// 重新认证：token 换新（旧实现按 token 哈希会另建一条）
	td2 := &codebuddy.TokenData{AccessToken: "at-second", RefreshToken: "rt-second", Domain: "tencent.com"}
	v, err := svc.AddOAuth(context.Background(), td2, acc)
	if err != nil {
		t.Fatalf("second add: %v", err)
	}
	if len(*saved) != 1 {
		t.Fatalf("re-auth created new credential (total %d)", len(*saved))
	}
	if v.Id != firstID {
		t.Errorf("should reuse id %s, got %s", firstID, v.Id)
	}
	if (*saved)[0].BearerToken != "at-second" {
		t.Errorf("token not updated in place: %q", (*saved)[0].BearerToken)
	}
	if (*saved)[0].UserId != "uid_acc-1" {
		t.Errorf("user_id drifted: %q", (*saved)[0].UserId)
	}

	// 无账号信息（acc=nil）→ 退回 token 哈希行为，直接新建
	if _, err := svc.AddOAuth(context.Background(), &codebuddy.TokenData{AccessToken: "at-x"}, nil); err != nil {
		t.Fatalf("fallback add: %v", err)
	}
	if len(*saved) != 2 {
		t.Fatalf("fallback should create new (total %d)", len(*saved))
	}
}

func TestTraeImportDeduplicatesSameUid(t *testing.T) {
	// 同 uid 重复登录 → 原位更新，不新增
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == trae.EpExchange {
			_, _ = w.Write([]byte(`{"Result":{"Token":"at-2","TokenExpireAt":9999999999999,"RefreshToken":"rt-2"}}`))
		} else {
			_, _ = w.Write([]byte(`{"Result":{"UserID":"uid-dup","ScreenName":"复登"}}`))
		}
	}))
	defer srv.Close()

	svc, client, saved := mkTraeSvc(t, 18080)
	client.OAuthHost = srv.URL

	if _, err := svc.Import(context.Background(), traeCallback("rt-1", "", "")); err != nil {
		t.Fatalf("first import: %v", err)
	}
	if len(*saved) != 1 {
		t.Fatalf("first import saved %d", len(*saved))
	}
	firstID := (*saved)[0].Id

	cred, err := svc.Import(context.Background(), traeCallback("rt-again", "", ""))
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if len(*saved) != 1 {
		t.Fatalf("duplicate login created new credential (total %d)", len(*saved))
	}
	if cred.Id != firstID {
		t.Errorf("should reuse existing id %s, got %s", firstID, cred.Id)
	}
	if *cred.RefreshToken != "rt-2" || cred.BearerToken != "at-2" {
		t.Errorf("token not refreshed in place: %+v", cred)
	}
}
