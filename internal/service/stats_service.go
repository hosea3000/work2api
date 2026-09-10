package service

import (
	"context"

	"github.com/hosea3000/work2api/internal/model"
	"github.com/hosea3000/work2api/internal/repository"
)

// StatsService 聊天请求统计：记录单次调用，并按时间范围汇总。
type StatsService interface {
	Record(ctx context.Context, startedAt int64, success bool) error
	Overview(ctx context.Context, startAt, endAt int64) (total, success int64, err error)
}

func NewStatsService(repo repository.StatsRepository) StatsService {
	return &statsService{repo: repo}
}

type statsService struct {
	repo repository.StatsRepository
}

func (s *statsService) Record(ctx context.Context, startedAt int64, success bool) error {
	outcome := "failure"
	if success {
		outcome = "success"
	}
	return s.repo.Record(ctx, &model.RequestRecord{StartedAt: startedAt, Outcome: outcome})
}

func (s *statsService) Overview(ctx context.Context, startAt, endAt int64) (int64, int64, error) {
	return s.repo.CountRequests(ctx, startAt, endAt)
}
