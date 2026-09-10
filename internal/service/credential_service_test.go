package service

import (
	"context"
	"encoding/base64"
	"sync"
	"testing"

	"github.com/hosea3000/work2api/internal/config"
	"github.com/hosea3000/work2api/internal/model"
)

func testConf(rotation int) *config.CodeBuddyConfig {
	return &config.CodeBuddyConfig{RotationCount: rotation}
}

func cbCreds(ids ...string) []model.CodeBuddyCredential {
	out := make([]model.CodeBuddyCredential, 0, len(ids))
	for _, id := range ids {
		out = append(out, newCbCred(id, "active"))
	}
	return out
}

func TestPoolRotationCount(t *testing.T) {
	p := NewCredentialPool(testConf(2))
	p.LoadAll(cbCreds("a", "b"))
	got := []string{}
	for i := 0; i < 4; i++ {
		sel, ok := p.Select()
		if !ok {
			t.Fatal("select failed")
		}
		got = append(got, sel.Entry.Credential.Id)
	}
	if got[0] != "a" || got[1] != "a" || got[2] != "b" || got[3] != "b" {
		t.Errorf("rotation wrong: %v", got)
	}
}

func TestPoolRotationDefault1(t *testing.T) {
	p := NewCredentialPool(testConf(1))
	p.LoadAll(cbCreds("a", "b"))
	ids := map[string]int{}
	for i := 0; i < 10; i++ {
		sel, _ := p.Select()
		ids[sel.Entry.Credential.Id]++
	}
	if ids["a"] != 5 || ids["b"] != 5 {
		t.Errorf("expected alternating 5/5, got %v", ids)
	}
}

func TestPoolConcurrentSelect(t *testing.T) {
	p := NewCredentialPool(testConf(1))
	p.LoadAll(cbCreds("a", "b", "c"))
	var wg sync.WaitGroup
	var mu sync.Mutex
	counts := map[string]int{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sel, ok := p.Select()
			if !ok {
				t.Error("select failed")
				return
			}
			mu.Lock()
			counts[sel.Entry.Credential.Id]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	total := 0
	for _, v := range counts {
		total += v
	}
	if total != 100 {
		t.Errorf("lost selections: %d", total)
	}
}

func TestPoolMarkExpiredSkips(t *testing.T) {
	p := NewCredentialPool(testConf(1))
	p.LoadAll(cbCreds("a", "b"))
	p.MarkExpired("a")
	for i := 0; i < 5; i++ {
		sel, ok := p.Select()
		if !ok {
			t.Fatal("pool should not be empty")
		}
		if sel.Entry.Credential.Id != "b" {
			t.Errorf("expired credential selected: %s", sel.Entry.Credential.Id)
		}
	}
	p.MarkExpired("b")
	if _, ok := p.Select(); ok {
		t.Error("empty pool should fail select")
	}
}

func TestPoolManualSelect(t *testing.T) {
	p := NewCredentialPool(testConf(1))
	p.LoadAll(cbCreds("a", "b"))
	p.SetAutoRotation(false)
	if err := p.SelectCurrent("b"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		sel, _ := p.Select()
		if sel.Entry.Credential.Id != "b" {
			t.Errorf("manual select not sticky: %s", sel.Entry.Credential.Id)
		}
	}
}

func TestPoolRefreshKeepsCurrent(t *testing.T) {
	p := NewCredentialPool(testConf(1))
	p.LoadAll(cbCreds("a", "b"))
	if err := p.SelectCurrent("b"); err != nil {
		t.Fatal(err)
	}
	p.Refresh(cbCreds("a", "b", "c"))
	cur, _ := p.Current()
	if cur.Credential.Id != "b" {
		t.Errorf("refresh should keep current selected, got %s", cur.Credential.Id)
	}
	p.Refresh(cbCreds("a", "c"))
	cur, _ = p.Current()
	if cur.Credential.Id != "a" {
		t.Errorf("refresh should fall back to first, got %s", cur.Credential.Id)
	}
}

func TestCodeBuddySelectPersistsAndReloads(t *testing.T) {
	repo := &fakeCbRepo{saved: cbCreds("a", "b")}
	state := &fakeStateRepo{}
	svc := NewCodeBuddyCredentialService(repo, NewCredentialPool(testConf(1)), state, testConf(1), nil)
	svc.PoolReload(context.Background())

	if _, _, err := svc.Select(context.Background(), "b"); err != nil {
		t.Fatalf("select: %v", err)
	}
	if st := state.states["codebuddy"]; st == nil || st.AutoRotation || st.CurrentCredentialId == nil || *st.CurrentCredentialId != "b" {
		t.Fatalf("state not persisted: %+v", st)
	}

	// 模拟重启：新服务从 pool_state 恢复当前指针与开关
	svc2 := NewCodeBuddyCredentialService(repo, NewCredentialPool(testConf(1)), state, testConf(1), nil)
	svc2.PoolReload(context.Background())
	cur, _ := svc2.Current(context.Background())
	if cur == nil || cur.Id != "b" {
		t.Fatalf("current not restored: %+v", cur)
	}
	if svc2.RotationEnabled() {
		t.Error("auto rotation should stay disabled after reload")
	}
}

func TestTraeSelectPersistsAndReloads(t *testing.T) {
	repo := &fakeTraeRepo{saved: []model.TraeCredential{newTraeCred("x", "active"), newTraeCred("y", "active")}}
	state := &fakeStateRepo{}
	svc := NewTraeCredentialService(repo, NewTraeCredentialPool(), state, nil)
	svc.PoolReload(context.Background())

	if _, _, err := svc.Select(context.Background(), "y"); err != nil {
		t.Fatalf("select: %v", err)
	}
	if st := state.states["trae"]; st == nil || st.AutoRotation || st.CurrentCredentialId == nil || *st.CurrentCredentialId != "y" {
		t.Fatalf("state not persisted: %+v", st)
	}
	svc2 := NewTraeCredentialService(repo, NewTraeCredentialPool(), state, nil)
	svc2.PoolReload(context.Background())
	cur, _ := svc2.Current(context.Background())
	if cur == nil || cur.Id != "y" {
		t.Fatalf("current not restored: %+v", cur)
	}
}

func TestExtractUserIDFromJWT(t *testing.T) {
	token := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1c2VyMTIzIn0.sig"
	uid, err := ExtractUserIDFromJWT(token)
	if err != nil || uid != "user123" {
		t.Fatalf("got %q %v", uid, err)
	}
	if _, err := ExtractUserIDFromJWT("not-a-jwt"); err == nil {
		t.Error("expected error for non-jwt")
	}
	if _, err := ExtractUserIDFromJWT("eyJhbGciOiJIUzI1NiJ9.e30.sig"); err == nil {
		t.Error("expected error for missing sub")
	}
}

func TestExtractIssuerInfo(t *testing.T) {
	payload := `{"sub":"u1","iss":"https://team.example.com/auth/realms/sso-999"}`
	token := "hdr." + b64url(payload) + ".sig"
	domain, ent, ok := ExtractIssuerInfo(token)
	if !ok || domain != "team.example.com" || ent != "999" {
		t.Errorf("got %q %q %v", domain, ent, ok)
	}
}

func b64url(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}
