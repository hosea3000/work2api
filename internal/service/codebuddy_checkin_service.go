package service

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/hosea3000/work2api/internal/config"
	"github.com/hosea3000/work2api/internal/model"
	"github.com/hosea3000/work2api/internal/repository"
	"github.com/hosea3000/work2api/internal/upstream/codebuddy"
)

// CodeBuddyCheckinService codebuddy 每日签到（表/服务独立）。
type CodeBuddyCheckinService interface {
	ManualCheckin(ctx context.Context, credentialId string) (map[string]any, error)
	RunScheduledCheckin(ctx context.Context)
}

func NewCodeBuddyCheckinService(repo repository.CodeBuddyCredentialRepository, creds CodeBuddyCredentialService, client *codebuddy.Client, conf *config.CodeBuddyConfig) CodeBuddyCheckinService {
	return &codeBuddyCheckinService{
		repo:   repo,
		creds:  creds,
		client: client,
		conf:   conf,
		rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

type codeBuddyCheckinService struct {
	repo   repository.CodeBuddyCredentialRepository
	creds  CodeBuddyCredentialService
	client *codebuddy.Client
	conf   *config.CodeBuddyConfig
	rand   *rand.Rand
	mu     sync.Mutex
}

func (s *codeBuddyCheckinService) ManualCheckin(ctx context.Context, credentialId string) (map[string]any, error) {
	cred, err := s.creds.GetByID(ctx, credentialId)
	if err != nil || cred == nil {
		return nil, fmt.Errorf("credential not found")
	}
	entry := toPoolEntry(*cred)
	return checkinMap(credentialId, s.performCheckin(ctx, credentialId, entry.Snapshot)), nil
}

// performCheckin 执行一次签到并落库。
func (s *codeBuddyCheckinService) performCheckin(ctx context.Context, credentialId string, snap codebuddy.CredentialSnapshot) CheckinDetail {
	now := time.Now()
	attemptedAt := now.Unix()
	date := localDate(now)

	success, code, message, credit, err := s.client.Checkin(ctx, snap)
	var checkedInAt *int64
	if err == nil && success {
		t := attemptedAt
		checkedInAt = &t
	}
	if err != nil && message == "" {
		message = "无法连接签到服务"
	}
	detail := CheckinDetail{
		CredentialId: credentialId,
		CheckinDate:  date,
		Success:      success,
		Code:         code,
		Message:      message,
		Credit:       credit,
		AttemptedAt:  attemptedAt,
		CheckedInAt:  checkedInAt,
	}
	record := &model.CodeBuddyCheckinRecord{
		CredentialId: credentialId,
		CheckinDate:  date,
		Success:      success,
		Code:         code,
		Message:      message,
		Credit:       credit,
		AttemptedAt:  attemptedAt,
		CheckedInAt:  checkedInAt,
	}
	if saveErr := s.repo.SaveCheckinRecord(ctx, record); saveErr != nil {
		log.Printf("save codebuddy checkin record failed: %v", saveErr)
	}
	return detail
}

// RunScheduledCheckin 遍历 active 且当日未签到的 codebuddy 凭证，带抖动逐个签到。
func (s *codeBuddyCheckinService) RunScheduledCheckin(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.creds.List(ctx)
	if err != nil {
		return
	}
	today := localDate(time.Now())
	for _, view := range list {
		if view.Status != "active" {
			continue
		}
		if rec, err := s.repo.GetCheckinRecord(ctx, view.Id, today); err == nil && rec != nil && rec.Success {
			continue
		}
		if !s.jitter(ctx) {
			return
		}
		cred, err := s.creds.GetByID(ctx, view.Id)
		if err != nil || cred == nil {
			continue
		}
		entry := toPoolEntry(*cred)
		s.performCheckin(ctx, view.Id, entry.Snapshot)
	}
}

func (s *codeBuddyCheckinService) jitter(ctx context.Context) bool {
	if s.conf == nil || s.conf.DelayMaxSeconds <= 0 {
		return true
	}
	minMs := int64(s.conf.DelayMinSeconds * 1000)
	maxMs := int64(s.conf.DelayMaxSeconds * 1000)
	if maxMs <= minMs {
		return true
	}
	delay := time.Duration(minMs + s.rand.Int63n(maxMs-minMs)) * time.Millisecond
	select {
	case <-time.After(delay):
		return true
	case <-ctx.Done():
		return false
	}
}
