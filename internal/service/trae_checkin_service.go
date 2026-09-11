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
	"github.com/hosea3000/work2api/internal/upstream/trae"
)

// TraeCheckinService trae 每日签到（表/服务独立，两步式）。
type TraeCheckinService interface {
	ManualCheckin(ctx context.Context, credentialId string) (map[string]any, error)
	RunScheduledCheckin(ctx context.Context)
}

func NewTraeCheckinService(repo repository.TraeCredentialRepository, creds TraeCredentialService, client *trae.Client, conf *config.CodeBuddyConfig) TraeCheckinService {
	return &traeCheckinService{
		repo:   repo,
		creds:  creds,
		client: client,
		conf:   conf,
		rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

type traeCheckinService struct {
	repo   repository.TraeCredentialRepository
	creds  TraeCredentialService
	client *trae.Client
	conf   *config.CodeBuddyConfig
	rand   *rand.Rand
	mu     sync.Mutex
}

func (s *traeCheckinService) ManualCheckin(ctx context.Context, credentialId string) (map[string]any, error) {
	cred, err := s.creds.GetByID(ctx, credentialId)
	if err != nil || cred == nil {
		return nil, fmt.Errorf("credential not found")
	}
	return checkinMap(credentialId, s.performTraeCheckin(ctx, credentialId, cred)), nil
}

// performTraeCheckin TRAE 两步式签到：CheckinStatus 幂等闸门 → CheckinClaim → EntUsage 查积分。
func (s *traeCheckinService) performTraeCheckin(ctx context.Context, credentialId string, cred *model.TraeCredential) CheckinDetail {
	now := time.Now()
	attemptedAt := now.Unix()
	date := localDate(now)
	at := cred.BearerToken
	deviceID := derefStr(cred.DeviceID)

	checkedIn, _, enable, err := s.client.CheckinStatus(ctx, at, deviceID)
	if err != nil {
		return s.saveDetail(ctx, credentialId, date, attemptedAt, false, nil, "无法连接签到服务", nil)
	}
	if checkedIn {
		return s.saveDetail(ctx, credentialId, date, attemptedAt, true, nil, "已签到", s.entUsage(ctx, at, deviceID))
	}
	if !enable {
		return s.saveDetail(ctx, credentialId, date, attemptedAt, false, nil, "签到功能未开启", nil)
	}

	res, err := s.client.CheckinClaim(ctx, at, deviceID)
	if err != nil {
		return s.saveDetail(ctx, credentialId, date, attemptedAt, false, nil, "无法连接签到服务", nil)
	}
	var credit *float64
	if res.Success {
		credit = s.entUsage(ctx, at, deviceID)
	}
	return s.saveDetail(ctx, credentialId, date, attemptedAt, res.Success, res.Code, res.Message, credit)
}

func (s *traeCheckinService) entUsage(ctx context.Context, at, deviceID string) *float64 {
	remain, _, _, err := s.client.EntUsage(ctx, at, deviceID)
	if err != nil {
		log.Printf("[trae-checkin] ent usage failed: %v", err)
		return nil
	}
	f := float64(remain)
	return &f
}

func (s *traeCheckinService) saveDetail(ctx context.Context, credentialId, date string, attemptedAt int64, success bool, code *int, message string, credit *float64) CheckinDetail {
	var checkedInAt *int64
	if success {
		t := attemptedAt
		checkedInAt = &t
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
	record := &model.TraeCheckinRecord{
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
		log.Printf("save trae checkin record failed: %v", saveErr)
	}
	return detail
}

// RunScheduledCheckin 遍历 active trae 凭证；幂等走上游 status，带抖动逐个签到。
func (s *traeCheckinService) RunScheduledCheckin(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.creds.List(ctx)
	if err != nil {
		return
	}
	for _, view := range list {
		if view.Status != "active" {
			continue
		}
		if !s.jitter(ctx) {
			return
		}
		cred, err := s.creds.GetByID(ctx, view.Id)
		if err != nil || cred == nil {
			continue
		}
		s.performTraeCheckin(ctx, view.Id, cred)
	}
}

func (s *traeCheckinService) jitter(ctx context.Context) bool {
	if s.conf == nil || s.conf.DelayMaxSeconds <= 0 {
		return true
	}
	minMs := int64(s.conf.DelayMinSeconds * 1000)
	maxMs := int64(s.conf.DelayMaxSeconds * 1000)
	if maxMs <= minMs {
		return true
	}
	delay := time.Duration(minMs+s.rand.Int63n(maxMs-minMs)) * time.Millisecond
	select {
	case <-time.After(delay):
		return true
	case <-ctx.Done():
		return false
	}
}
