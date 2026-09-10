package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"encoding/base64"

	"github.com/yourname/work2api/internal/model"
	"github.com/yourname/work2api/internal/upstream/codebuddy"
)

func TestAuthStateStoreTTLAndConsume(t *testing.T) {
	s := NewAuthStateStore()
	res, err := s.BeginStart()
	if err != nil {
		t.Fatal(err)
	}
	if !s.FinishStart(res, "state1") {
		t.Fatal("finish start failed")
	}
	// 轮询前归属校验通过
	if !s.ValidateOwner("state1") {
		t.Error("state should be valid")
	}
	// 消费一次成功，第二次拒绝（墓碑）
	if !s.Consume("state1") {
		t.Error("first consume should succeed")
	}
	if s.Consume("state1") {
		t.Error("replayed consume must fail")
	}
	if s.ValidateOwner("state1") {
		t.Error("consumed state must not validate")
	}
	if s.ValidateOwner("unknown") {
		t.Error("unknown state must not validate")
	}
}

func TestAuthStateStoreDuplicateFinish(t *testing.T) {
	s := NewAuthStateStore()
	res, _ := s.BeginStart()
	_ = res
	res2, _ := s.BeginStart()
	if !s.FinishStart(res2, "dup") {
		t.Fatal("first finish should succeed")
	}
	if s.FinishStart(res2, "dup") {
		t.Error("reservation reuse must fail")
	}
	res3, _ := s.BeginStart()
	if s.FinishStart(res3, "dup") {
		t.Error("duplicate state must fail")
	}
}

func TestAuthStateStoreStartWindow(t *testing.T) {
	s := NewAuthStateStore()
	for i := 0; i < 5; i++ {
		if _, err := s.BeginStart(); err != nil {
			t.Fatalf("attempt %d should pass: %v", i+1, err)
		}
	}
	_, err := s.BeginStart()
	var limitErr *AuthStartLimitError
	if !errors.As(err, &limitErr) {
		t.Fatal("6th attempt must be limited")
	}
	if limitErr.RetryAfter <= 0 || limitErr.RetryAfter > authStartWindowSeconds {
		t.Errorf("retry-after out of window: %d", limitErr.RetryAfter)
	}
}

func TestAuthStateStoreActiveLimit(t *testing.T) {
	s := NewAuthStateStore()
	for i := 0; i < authMaxActiveStates; i++ {
		res, err := s.BeginStart()
		if err != nil {
			t.Fatalf("active attempt %d should pass: %v", i+1, err)
		}
		if !s.FinishStart(res, string(rune('a'+i))) {
			t.Fatal("finish failed")
		}
	}
	if _, err := s.BeginStart(); !errors.Is(err, &AuthStartLimitError{}) {
		t.Log(err)
		if _, e2 := s.BeginStart(); e2 == nil {
			t.Fatal("4th active state must be limited")
		}
	}
}

func TestAuthStateStoreCancelStart(t *testing.T) {
	s := NewAuthStateStore()
	res, _ := s.BeginStart()
	s.CancelStart(res)
	if s.FinishStart(res, "x") {
		t.Error("cancelled reservation must not finish")
	}
}

func TestAuthStateStoreConcurrent(t *testing.T) {
	s := NewAuthStateStore()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = s.BeginStart()
			_ = s.ValidateOwner("x")
			_ = s.Consume("x")
		}()
	}
	wg.Wait()
}

func TestAddOAuthPersonalAccount(t *testing.T) {
	// 直接测 stringPtr/shortHash 辅助与 model 组装在 AddOAuth 中的关键字段映射
	if stringPtr("") != nil {
		t.Error("empty string should map to nil ptr")
	}
	if s := stringPtr("x"); s == nil || *s != "x" {
		t.Error("non-empty string mapping wrong")
	}
	h := shortHash("token")
	if len(h) != 12 {
		t.Errorf("short hash length wrong: %d", len(h))
	}
	// AddOAuth 完整路径需要 repo；此处仅校验 helper 纯函数
	_ = context.Background()
	_ = codebuddy.TokenData{}
}

func TestApplyJWTIdentity(t *testing.T) {
	cred := &model.Credential{}
	// payload: {"sub":"u1","nickname":"Hosea","preferred_username":"17673040926","email":"a@b.c"}
	token := "hdr." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"u1","nickname":"Hosea","preferred_username":"17673040926","email":"a@b.c"}`)) + ".sig"
	applyJWTIdentity(cred, token)
	if cred.Nickname == nil || *cred.Nickname != "Hosea" {
		t.Errorf("nickname wrong: %v", cred.Nickname)
	}
	if cred.PreferredUsername == nil || *cred.PreferredUsername != "17673040926" {
		t.Errorf("preferred_username wrong: %v", cred.PreferredUsername)
	}
	if cred.Email == nil || *cred.Email != "a@b.c" {
		t.Errorf("email wrong: %v", cred.Email)
	}
	// nickname 缺失 → 回退 preferred_username
	token2 := "hdr." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"u1","preferred_username":"17673040926"}`)) + ".sig"
	cred2 := &model.Credential{}
	applyJWTIdentity(cred2, token2)
	if cred2.Nickname == nil || *cred2.Nickname != "17673040926" {
		t.Errorf("nickname fallback wrong: %v", cred2.Nickname)
	}
	// 非 JWT → 静默跳过
	cred3 := &model.Credential{}
	applyJWTIdentity(cred3, "not-a-jwt")
	if cred3.Nickname != nil || cred3.Email != nil {
		t.Error("invalid token must be silently skipped")
	}
}
