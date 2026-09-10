package service

import (
	"testing"
	"time"

	"github.com/yourname/work2api/internal/model"
)

func TestTraePoolFiltersActive(t *testing.T) {
	p := NewTraeCredentialPool()
	p.Refresh([]model.TraeCredential{
		newTraeCred("t1", "active"),
		newTraeCred("t2", "expired"),
	})
	if p.Len() != 1 {
		t.Fatalf("len=%d want 1 (only active)", p.Len())
	}
}

func TestTraePoolRoundRobin(t *testing.T) {
	p := NewTraeCredentialPool()
	p.Refresh([]model.TraeCredential{newTraeCred("t1", "active"), newTraeCred("t2", "active")})
	seen := map[string]int{}
	for i := 0; i < 4; i++ {
		e, ok := p.Select()
		if !ok {
			t.Fatal("select failed")
		}
		seen[e.Credential.Id]++
	}
	if seen["t1"] != 2 || seen["t2"] != 2 {
		t.Errorf("round-robin distribution=%v", seen)
	}
}

func TestTraePoolManualPinsCurrent(t *testing.T) {
	p := NewTraeCredentialPool()
	p.Refresh([]model.TraeCredential{newTraeCred("t1", "active"), newTraeCred("t2", "active")})
	if err := p.SelectCurrent("t2"); err != nil {
		t.Fatalf("select current: %v", err)
	}
	p.SetAutoRotation(false)
	for i := 0; i < 3; i++ {
		e, ok := p.Select()
		if !ok || e.Credential.Id != "t2" {
			t.Fatalf("manual mode should pin t2, got %+v ok=%v", e.Credential.Id, ok)
		}
	}
}

func TestTraePoolSkipsCooldown(t *testing.T) {
	p := NewTraeCredentialPool()
	p.Refresh([]model.TraeCredential{newTraeCred("t1", "active"), newTraeCred("t2", "active")})
	p.Cooldown("t1", time.Now().Unix()+3600)
	for i := 0; i < 3; i++ {
		e, ok := p.Select()
		if !ok || e.Credential.Id != "t2" {
			t.Fatalf("cooling credential selected: %+v ok=%v", e.Credential.Id, ok)
		}
	}
}

func TestTraePoolAllCoolingReturnsFalse(t *testing.T) {
	p := NewTraeCredentialPool()
	p.Refresh([]model.TraeCredential{newTraeCred("t1", "active")})
	p.Cooldown("t1", time.Now().Unix()+3600)
	if _, ok := p.Select(); ok {
		t.Fatal("all-cooling pool should return ok=false")
	}
}

func TestTraePoolRefreshPreservesCooldown(t *testing.T) {
	p := NewTraeCredentialPool()
	cred := newTraeCred("t1", "active")
	p.Refresh([]model.TraeCredential{cred})
	p.Cooldown("t1", time.Now().Unix()+3600)
	cred.BearerToken = "at-new"
	p.Refresh([]model.TraeCredential{cred})
	if _, ok := p.Select(); ok {
		t.Fatal("cooldown should survive Refresh")
	}
}

func TestTraePoolMarkExpired(t *testing.T) {
	p := NewTraeCredentialPool()
	p.Refresh([]model.TraeCredential{newTraeCred("t1", "active"), newTraeCred("t2", "active")})
	p.MarkExpired("t1")
	if p.Len() != 1 {
		t.Fatalf("len=%d want 1", p.Len())
	}
	e, _ := p.Select()
	if e.Credential.Id != "t2" {
		t.Errorf("selected=%s want t2", e.Credential.Id)
	}
}
