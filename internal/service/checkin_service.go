package service

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/yourname/work2api/internal/config"
	"github.com/yourname/work2api/internal/model"
	"github.com/yourname/work2api/internal/repository"
	"github.com/yourname/work2api/internal/upstream/codebuddy"
)

// CheckinService 每日签到（自动调度 + 手动触发）。
type CheckinService interface {
	ManualCheckin(ctx context.Context, credentialId string) (map[string]any, error)
	RunScheduledCheckin(ctx context.Context)
	Bootstrap(ctx context.Context)
}

func NewCheckinService(repo repository.GatewayRepository, creds CredentialService, client *codebuddy.Client, conf *config.CodeBuddyConfig) CheckinService {
	return &checkinService{
		repo:   repo,
		creds:  creds,
		client: client,
		conf:   conf,
		rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

type checkinService struct {
	repo   repository.GatewayRepository
	creds  CredentialService
	client *codebuddy.Client
	conf   *config.CodeBuddyConfig
	rand   *rand.Rand
	mu     sync.Mutex
}

func localDate(t time.Time) string {
	return t.Format("2006-01-02")
}

// ManualCheckin 立即对指定凭证签到（无视当日自动记录）。
func (s *checkinService) ManualCheckin(ctx context.Context, credentialId string) (map[string]any, error) {
	cred, err := s.creds.GetByID(ctx, credentialId)
	if err != nil || cred == nil {
		return nil, fmt.Errorf("credential not found")
	}
	if !isCodebuddy(*cred) {
		return nil, fmt.Errorf("credential provider does not support checkin")
	}
	entry := toPoolEntry(*cred)
	detail := s.performCheckin(ctx, credentialId, entry.Snapshot)
	return map[string]any{
		"credential_id": credentialId,
		"success":       detail.Success,
		"code":          detail.Code,
		"message":       detail.Message,
		"credit":        detail.Credit,
		"date":          detail.CheckinDate,
	}, nil
}

// CheckinDetail 单次签到结果。
type CheckinDetail struct {
	CredentialId string
	CheckinDate  string
	Success      bool
	Code         *int
	Message      string
	Credit       *float64
	AttemptedAt  int64
	CheckedInAt  *int64
}

// performCheckin 执行一次签到并落库（幂等判定对齐参考实现）。
func (s *checkinService) performCheckin(ctx context.Context, credentialId string, snap codebuddy.CredentialSnapshot) CheckinDetail {
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
	record := &model.CheckinRecord{
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
		log.Printf("save checkin record failed: %v", saveErr)
	}
	return detail
}

// RunScheduledCheckin 每日调度入口：遍历 active 且当日未成功签到的凭证，逐个带抖动签到。
func (s *checkinService) RunScheduledCheckin(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.creds.List(ctx)
	if err != nil {
		return
	}
	today := localDate(time.Now())
	checkedIn := 0
	for _, view := range list {
		if view.Status != "active" || !isCodebuddyView(view) {
			continue // TRAE 凭证不参与 codebuddy 签到（trae 签到接口二期接入）
		}
		cred, err := s.creds.GetByID(ctx, view.Id)
		if err != nil || cred == nil {
			continue
		}
		// 当日已成功签到 → 跳过
		if rec, err := s.repo.GetCheckinRecord(ctx, view.Id, today); err == nil && rec != nil && rec.Success {
			continue
		}
		// 抖动间隔（防风控）
		if s.conf.DelayMaxSeconds > 0 {
			minMs := int64(s.conf.DelayMinSeconds * 1000)
			maxMs := int64(s.conf.DelayMaxSeconds * 1000)
			if maxMs > minMs {
				delay := time.Duration(minMs + s.rand.Int63n(maxMs-minMs)) * time.Millisecond
				select {
				case <-time.After(delay):
				case <-ctx.Done():
					return
				}
			}
		}
		entry := toPoolEntry(*cred)
		detail := s.performCheckin(ctx, view.Id, entry.Snapshot)
		if detail.Success {
			checkedIn++
		}
	}
	_ = checkedIn
}

// Bootstrap 启动时占位（对齐参考实现 initial_scan_waiter 语义的一期简化）。
func (s *checkinService) Bootstrap(ctx context.Context) {}
