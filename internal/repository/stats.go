package repository

import (
	"context"

	"github.com/hosea3000/work2api/internal/model"
)

// StatsRepository 聊天请求记录的持久化与时间范围统计。
type StatsRepository interface {
	Record(ctx context.Context, rec *model.RequestRecord) error
	CountRequests(ctx context.Context, startAt, endAt int64) (total, success int64, err error)
}

func NewStatsRepository(r *Repository) StatsRepository {
	return &statsRepository{Repository: r}
}

type statsRepository struct{ *Repository }

func (r *statsRepository) Record(ctx context.Context, rec *model.RequestRecord) error {
	return r.DB(ctx).Create(rec).Error
}

// CountRequests 统计 [startAt, endAt) 区间内的总请求数与成功数。
func (r *statsRepository) CountRequests(ctx context.Context, startAt, endAt int64) (int64, int64, error) {
	var total int64
	if err := r.DB(ctx).Model(&model.RequestRecord{}).
		Where("started_at >= ? AND started_at < ?", startAt, endAt).
		Count(&total).Error; err != nil {
		return 0, 0, err
	}
	var success int64
	if err := r.DB(ctx).Model(&model.RequestRecord{}).
		Where("started_at >= ? AND started_at < ?", startAt, endAt).
		Where("outcome = ?", "success").
		Count(&success).Error; err != nil {
		return 0, 0, err
	}
	return total, success, nil
}
