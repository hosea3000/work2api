package service

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hosea3000/work2api/internal/model"
	"github.com/hosea3000/work2api/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

// SessionService 管理台会话：内存 token + HttpOnly Cookie。
type SessionService interface {
	Login(ctx context.Context, username, password string) (token string, ok bool, err error)
	Logout(ctx context.Context, token string) error
	Validate(ctx context.Context, token string) (username string, ok bool)
	BootstrapIfEmpty(ctx context.Context, username, password string) error
}

const sessionTTL = 7 * 24 * time.Hour

func NewSessionService(repo repository.GatewayRepository) SessionService {
	return &sessionService{repo: repo, sessions: map[string]*sessionEntry{}}
}

type sessionEntry struct {
	username  string
	expiresAt time.Time
}

type sessionService struct {
	repo     repository.GatewayRepository
	mu       sync.Mutex
	sessions map[string]*sessionEntry
}

func (s *sessionService) Login(ctx context.Context, username, password string) (string, bool, error) {
	admin, err := s.repo.GetAdminByUsername(ctx, username)
	if err != nil {
		return "", false, err
	}
	if admin == nil {
		// 仍执行一次 bcrypt 以消除时序差异
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$7EqJtq98hPqEX7fNZaFWoOhi5B0X0fP0V7JkUIuHcfOMzY0S6Q1Ha"), []byte(password))
		return "", false, nil
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte(password)); err != nil {
		return "", false, nil
	}
	token := uuid.NewString() + uuid.NewString()
	s.mu.Lock()
	s.sessions[token] = &sessionEntry{username: username, expiresAt: time.Now().Add(sessionTTL)}
	s.mu.Unlock()
	return token, true, nil
}

func (s *sessionService) Logout(ctx context.Context, token string) error {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
	return nil
}

// Validate 滑动续期。
func (s *sessionService) Validate(ctx context.Context, token string) (string, bool) {
	if token == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.sessions[token]
	if !ok {
		return "", false
	}
	if time.Now().After(entry.expiresAt) {
		delete(s.sessions, token)
		return "", false
	}
	entry.expiresAt = time.Now().Add(sessionTTL)
	return entry.username, true
}

// BootstrapIfEmpty 首启动建户。
func (s *sessionService) BootstrapIfEmpty(ctx context.Context, username, password string) error {
	n, err := s.repo.CountAdmins(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repo.CreateAdmin(ctx, &model.AdminUser{Username: username, Password: string(hash)})
}
