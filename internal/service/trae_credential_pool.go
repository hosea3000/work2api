package service

import (
	"fmt"
	"sync"
	"time"

	"github.com/yourname/work2api/internal/model"
)

// TraePoolEntry TRAE 轮换池条目：凭证 + 冷却截止时间（Unix 秒，0 表示不冷却）。
type TraePoolEntry struct {
	Credential    model.TraeCredential
	CooldownUntil int64
}

// TraeCredentialPool TRAE 独立内存轮换池：仅装 trae_credential 的 active 凭证。
// 与 codebuddy 池完全隔离；支持自动轮询与手动钉住，选择跳过冷却中的凭证。
type TraeCredentialPool struct {
	mu           sync.Mutex
	entries      []TraePoolEntry
	current      int
	usageCount   int
	autoRotation bool
}

func NewTraeCredentialPool() *TraeCredentialPool {
	return &TraeCredentialPool{autoRotation: true}
}

// Refresh 用 DB 全量列表重算：仅 active；按 id 保留尚未过期的冷却。
func (p *TraeCredentialPool) Refresh(creds []model.TraeCredential) {
	p.mu.Lock()
	defer p.mu.Unlock()
	prev := ""
	if p.current < len(p.entries) {
		prev = p.entries[p.current].Credential.Id
	}
	p.entries = p.buildEntries(creds)
	p.current = 0
	if prev != "" {
		for i, e := range p.entries {
			if e.Credential.Id == prev {
				p.current = i
				break
			}
		}
	}
}

// RefreshWithCurrent 用完整列表重算，并把指针定位到 preferID（持久化的当前凭证）。
func (p *TraeCredentialPool) RefreshWithCurrent(creds []model.TraeCredential, preferID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entries = p.buildEntries(creds)
	p.current = 0
	if preferID != "" {
		for i, e := range p.entries {
			if e.Credential.Id == preferID {
				p.current = i
				return
			}
		}
	}
}

func (p *TraeCredentialPool) buildEntries(creds []model.TraeCredential) []TraePoolEntry {
	now := time.Now().Unix()
	prevCooldown := map[string]int64{}
	for _, e := range p.entries {
		if e.CooldownUntil > now {
			prevCooldown[e.Credential.Id] = e.CooldownUntil
		}
	}
	entries := make([]TraePoolEntry, 0, len(creds))
	for _, c := range creds {
		if c.Status != "active" {
			continue
		}
		entries = append(entries, TraePoolEntry{Credential: c, CooldownUntil: prevCooldown[c.Id]})
	}
	return entries
}

// Select 选择下一个可用凭证：autoRotation 时 round-robin，否则返回钉住的 current。
// 当前凭证冷却时回退扫描下一个非冷却凭证；全部冷却或池空返回 ok=false。
func (p *TraeCredentialPool) Select() (TraePoolEntry, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := len(p.entries)
	if n == 0 {
		return TraePoolEntry{}, false
	}
	now := time.Now().Unix()
	if p.autoRotation {
		for i := 0; i < n; i++ {
			idx := (p.current + i) % n
			if p.entries[idx].CooldownUntil <= now {
				p.current = (idx + 1) % n
				return p.entries[idx], true
			}
		}
		return TraePoolEntry{}, false
	}
	// 手动模式：优先 current，冷却则扫描其余可用
	if p.current < n && p.entries[p.current].CooldownUntil <= now {
		return p.entries[p.current], true
	}
	for i := 0; i < n; i++ {
		if p.entries[i].CooldownUntil <= now {
			return p.entries[i], true
		}
	}
	return TraePoolEntry{}, false
}

// MarkExpired 摘除指定凭证（内存），并推进指针。
func (p *TraeCredentialPool) MarkExpired(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, e := range p.entries {
		if e.Credential.Id != id {
			continue
		}
		p.entries = append(p.entries[:i], p.entries[i+1:]...)
		if p.current >= len(p.entries) {
			p.current = 0
		}
		p.usageCount = 0
		return
	}
}

// Cooldown 设置指定凭证的冷却截止时间（Unix 秒）。
func (p *TraeCredentialPool) Cooldown(id string, until int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.entries {
		if p.entries[i].Credential.Id == id {
			p.entries[i].CooldownUntil = until
			return
		}
	}
}

// SelectCurrent 手动选择：把指针固定到指定凭证。
func (p *TraeCredentialPool) SelectCurrent(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, e := range p.entries {
		if e.Credential.Id == id {
			p.current = i
			p.usageCount = 0
			return nil
		}
	}
	return fmt.Errorf("credential %s not in pool", id)
}

// SelectByID 仅校验存在性，不改变指针。
func (p *TraeCredentialPool) SelectByID(id string) (TraePoolEntry, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range p.entries {
		if e.Credential.Id == id {
			return e, true
		}
	}
	return TraePoolEntry{}, false
}

// SetAutoRotation 设置自动轮换开关。
func (p *TraeCredentialPool) SetAutoRotation(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.autoRotation = enabled
	p.usageCount = 0
}

// AutoRotationEnabled 返回当前开关。
func (p *TraeCredentialPool) AutoRotationEnabled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.autoRotation
}

// Current 返回当前选中凭证（不推进计数）。
func (p *TraeCredentialPool) Current() (TraePoolEntry, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.entries) == 0 {
		return TraePoolEntry{}, false
	}
	return p.entries[p.current], true
}

// Len 返回池内凭证数量。
func (p *TraeCredentialPool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.entries)
}
