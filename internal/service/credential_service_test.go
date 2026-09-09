package service

import (
	"context"
	"encoding/base64"
	"sync"
	"testing"

	"github.com/yourname/work2api/internal/config"
	"github.com/yourname/work2api/internal/model"
)

func testConf(rotation int) *config.CodeBuddyConfig {
	return &config.CodeBuddyConfig{RotationCount: rotation}
}

func mkCred(id string) model.Credential {
	return model.Credential{Id: id, BearerToken: "tok-" + id, UserId: "u-" + id, Status: "active"}
}

func TestPoolRotationCount(t *testing.T) {
	p := NewCredentialPool(testConf(2))
	p.LoadAll([]model.Credential{mkCred("a"), mkCred("b")})
	// 请求1、2 → a；请求3、4 → b
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
	p.LoadAll([]model.Credential{mkCred("a"), mkCred("b")})
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
	p.LoadAll([]model.Credential{mkCred("a"), mkCred("b"), mkCred("c")})
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
	p.LoadAll([]model.Credential{mkCred("a"), mkCred("b")})
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
	p.LoadAll([]model.Credential{mkCred("a"), mkCred("b")})
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
	p.LoadAll([]model.Credential{mkCred("a"), mkCred("b")})
	if err := p.SelectCurrent("b"); err != nil {
		t.Fatal(err)
	}
	// b 状态变化后 refresh（仍 active）
	p.Refresh([]model.Credential{mkCred("a"), mkCred("b"), mkCred("c")})
	cur, _ := p.Current()
	if cur.Credential.Id != "b" {
		t.Errorf("refresh should keep current selected, got %s", cur.Credential.Id)
	}
	// b 被摘除 → 回到 a
	p.Refresh([]model.Credential{mkCred("a"), mkCred("c")})
	cur, _ = p.Current()
	if cur.Credential.Id != "a" {
		t.Errorf("refresh should fall back to first, got %s", cur.Credential.Id)
	}
}

func TestExtractUserIDFromJWT(t *testing.T) {
	// header.payload.signature — payload = {"sub":"user123"}
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
	// iss = https://team.example.com/auth/realms/sso-999
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

var _ = context.Background
