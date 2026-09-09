package service

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

// AuthStartLimitError 启动频率/并发超限，携带 Retry-After 秒数。
type AuthStartLimitError struct {
	RetryAfter int
}

func (e *AuthStartLimitError) Error() string { return "auth start limit exceeded" }

const (
	authStateTTLSeconds       = 600
	authMaxActiveStates       = 3
	authMaxStartAttempts      = 5
	authStartWindowSeconds    = 60
)

type authStateEntry struct {
	createdAt  int64
	consumedAt *int64 // 墓碑：非 nil 表示已消费，防重放
}

// AuthStateStore 认证 state 内存管理：TTL、并发上限、启动限流、墓碑。
// 单管理员 → 无 username 归属维度。
type AuthStateStore struct {
	mu            sync.Mutex
	owners        map[string]*authStateEntry
	starting      map[string]int64 // reservation → created_at
	startAttempts []int64          // 启动尝试时间戳（窗口限流）
}

func NewAuthStateStore() *AuthStateStore {
	return &AuthStateStore{
		owners:   map[string]*authStateEntry{},
		starting: map[string]int64{},
	}
}

func (s *AuthStateStore) now() int64 { return time.Now().Unix() }

// prune 惰性清理：过期 state 与过窗尝试记录。
// 返回 (活跃 state 数, 窗口内尝试数)。
func (s *AuthStateStore) prune() (int, int) {
	now := s.now()
	ttl := int64(authStateTTLSeconds)
	for state, e := range s.owners {
		expired := e.createdAt+ttl <= now
		consumed := e.consumedAt != nil && *e.consumedAt+ttl <= now
		if expired || consumed {
			delete(s.owners, state)
		}
	}
	for res, at := range s.starting {
		if at+ttl <= now {
			delete(s.starting, res)
		}
	}
	kept := s.startAttempts[:0]
	for _, at := range s.startAttempts {
		if at+int64(authStartWindowSeconds) > now {
			kept = append(kept, at)
		}
	}
	s.startAttempts = kept
	active := 0
	for _, e := range s.owners {
		if e.consumedAt == nil && e.createdAt+ttl > now {
			active++
		}
	}
	return active, len(s.startAttempts)
}

// BeginStart 登记一次启动尝试；超限返回 AuthStartLimitError。
func (s *AuthStateStore) BeginStart() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	active, attempts := s.prune()
	if attempts >= authMaxStartAttempts {
		oldest := s.startAttempts[0]
		return "", &AuthStartLimitError{RetryAfter: int(oldest + authStartWindowSeconds - now)}
	}
	if active >= authMaxActiveStates {
		// 最早活跃 state 的到期时间即释放时间
		earliest := int64(-1)
		for _, e := range s.owners {
			if e.consumedAt == nil && e.createdAt+ttl() > now {
				if earliest < 0 || e.createdAt < earliest {
					earliest = e.createdAt
				}
			}
		}
		return "", &AuthStartLimitError{RetryAfter: int(earliest + ttl() - now)}
	}
	s.startAttempts = append(s.startAttempts, now)
	reservation := newReservation()
	s.starting[reservation] = now
	return reservation, nil
}

func ttl() int64 { return authStateTTLSeconds }

// FinishStart 将预约原子转换为 auth_state；state 已存在返回 false。
func (s *AuthStateStore) FinishStart(reservation, state string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	at, ok := s.starting[reservation]
	if !ok {
		return false
	}
	delete(s.starting, reservation)
	if _, exists := s.owners[state]; exists {
		return false
	}
	s.owners[state] = &authStateEntry{createdAt: at}
	return true
}

// CancelStart 取消预约（上游请求失败时回滚名额）。
func (s *AuthStateStore) CancelStart(reservation string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.starting, reservation)
}

// ValidateOwner 校验 state 存在、未消费、未过期。
func (s *AuthStateStore) ValidateOwner(state string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	e, ok := s.owners[state]
	if !ok {
		return false
	}
	return e.consumedAt == nil && e.createdAt+ttl() > s.now()
}

// Consume 原子消费 state（幂等拒绝：已消费返回 false）。
func (s *AuthStateStore) Consume(state string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	e, ok := s.owners[state]
	if !ok || e.consumedAt != nil {
		return false
	}
	if e.createdAt+ttl() <= s.now() {
		return false
	}
	now := s.now()
	e.consumedAt = &now
	return true
}

func newReservation() string {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		// rand.Read 失败极罕见；退化用时间戳
		return fmt.Sprintf("r%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
