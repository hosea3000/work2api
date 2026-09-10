package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/yourname/work2api/internal/config"
	"github.com/yourname/work2api/internal/model"
	"github.com/yourname/work2api/internal/upstream/codebuddy"
)

// PoolEntry 轮换池中的凭证快照。
type PoolEntry struct {
	Credential model.Credential
	Snapshot   codebuddy.CredentialSnapshot
}

// CredentialPool 内存轮换池：active 凭证 + round-robin 指针。
// SQLite 为持久层真相；任何写库后调用 Refresh() 同步内存。
type CredentialPool struct {
	mu            sync.Mutex
	entries       []PoolEntry
	current       int
	usageCount    int
	rotationCount int
	autoRotation  bool
}

func NewCredentialPool(conf *config.CodeBuddyConfig) *CredentialPool {
	return &CredentialPool{
		rotationCount: conf.RotationCount,
		autoRotation:  true,
	}
}

// isCodebuddy 报告凭证是否属于 codebuddy provider（池与调度路径只装 codebuddy）。
func isCodebuddy(c model.Credential) bool {
	return c.Provider == "" || c.Provider == "codebuddy"
}

// isCodebuddyView isCodebuddy 的 CredentialView 版本。
func isCodebuddyView(c CredentialView) bool {
	return c.Provider == "" || c.Provider == "codebuddy"
}

// LoadAll 用 DB 中的全部 active codebuddy 凭证重建池。
func (p *CredentialPool) LoadAll(creds []model.Credential) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entries := make([]PoolEntry, 0, len(creds))
	for _, c := range creds {
		if c.Status != "active" || !isCodebuddy(c) {
			continue
		}
		entries = append(entries, toPoolEntry(c))
	}
	p.entries = entries
	p.current = 0
	p.usageCount = 0
}

// Refresh 增删/状态变更后调用：用完整列表重算，尽量保持当前凭证选中。
func (p *CredentialPool) Refresh(creds []model.Credential) {
	p.mu.Lock()
	defer p.mu.Unlock()
	prev := ""
	if p.current < len(p.entries) {
		prev = p.entries[p.current].Credential.Id
	}
	entries := make([]PoolEntry, 0, len(creds))
	for _, c := range creds {
		if c.Status != "active" || !isCodebuddy(c) {
			continue
		}
		entries = append(entries, toPoolEntry(c))
	}
	p.entries = entries
	p.current = 0
	if prev != "" {
		for i, e := range entries {
			if e.Credential.Id == prev {
				p.current = i
				break
			}
		}
	}
}

func toPoolEntry(c model.Credential) PoolEntry {
	return PoolEntry{
		Credential: c,
		Snapshot: codebuddy.CredentialSnapshot{
			BearerToken:        c.BearerToken,
			UserID:             c.UserId,
			AccountUID:         deref(c.AccountUid),
			Domain:             deref(c.Domain),
			EnterpriseID:       deref(c.EnterpriseId),
			DepartmentFullName: deref(c.DepartmentFullName),
		},
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Selection 原子选择结果。
type Selection struct {
	Entry PoolEntry
}

// Select 按 round-robin 选择凭证（每 rotationCount 次请求切换一次）。
// 手动选择模式（autoRotation=false）固定返回当前凭证。
// 池空返回 ok=false。
func (p *CredentialPool) Select() (Selection, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.entries) == 0 {
		return Selection{}, false
	}
	if p.autoRotation {
		if p.usageCount >= maxInt(p.rotationCount, 1) {
			p.usageCount = 0
			p.current = (p.current + 1) % len(p.entries)
		}
		p.usageCount++
	}
	return Selection{Entry: p.entries[p.current]}, true
}

// MarkExpired 摘除指定凭证（内存），并推进指针到下一个可用凭证。
func (p *CredentialPool) MarkExpired(id string) {
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

// SelectCurrent 手动选择：把轮换指针固定到指定凭证。
func (p *CredentialPool) SelectCurrent(id string) error {
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

// SelectByID 手动选择（不改变轮换指针状态，仅用于 select 前校验存在性）。
func (p *CredentialPool) SelectByID(id string) (PoolEntry, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range p.entries {
		if e.Credential.Id == id {
			return e, true
		}
	}
	// 已摘除的凭证可能仍在 DB 中；由调用方决定是否从 DB 取
	return PoolEntry{}, false
}

// SetAutoRotation 设置自动轮换开关。
func (p *CredentialPool) SetAutoRotation(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.autoRotation = enabled
	p.usageCount = 0
}

// AutoRotationEnabled 返回当前开关。
func (p *CredentialPool) AutoRotationEnabled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.autoRotation
}

// Current 返回当前选中凭证（不推进计数）。
func (p *CredentialPool) Current() (PoolEntry, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.entries) == 0 {
		return PoolEntry{}, false
	}
	return p.entries[p.current], true
}

// Len 返回池内凭证数量。
func (p *CredentialPool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.entries)
}

var _ = context.Background
