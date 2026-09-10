package job

import (
	"context"
	"log"
	"time"

	"github.com/yourname/work2api/internal/config"
	"github.com/yourname/work2api/internal/service"
)

// CheckinJob 每日签到调度（每天 checkin_hour:checkin_minute 本地时区），触发两个 provider 的签到。
type CheckinJob struct {
	cbCheckin   service.CodeBuddyCheckinService
	traeCheckin service.TraeCheckinService
	conf        *config.CodeBuddyConfig
	stopCh      chan struct{}
}

func NewCheckinJob(cbCheckin service.CodeBuddyCheckinService, traeCheckin service.TraeCheckinService, conf *config.CodeBuddyConfig) *CheckinJob {
	return &CheckinJob{cbCheckin: cbCheckin, traeCheckin: traeCheckin, conf: conf, stopCh: make(chan struct{})}
}

// Start 阻塞运行调度循环（供 goroutine 调用）。
func (j *CheckinJob) Start(ctx context.Context) {
	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), j.conf.CheckinHour, j.conf.CheckinMinute, 0, 0, now.Location())
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		wait := next.Sub(now)
		log.Printf("[checkin-job] next checkin at %s (in %v)", next.Format(time.RFC3339), wait)
		select {
		case <-time.After(wait):
			j.cbCheckin.RunScheduledCheckin(ctx)
			j.traeCheckin.RunScheduledCheckin(ctx)
		case <-j.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// Stop 停止调度。
func (j *CheckinJob) Stop() {
	close(j.stopCh)
}
