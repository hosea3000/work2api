package server

import (
	"context"

	"github.com/yourname/work2api/internal/bootstrap"
	"github.com/yourname/work2api/internal/config"
	"github.com/yourname/work2api/internal/job"
	"github.com/yourname/work2api/internal/repository"
	"github.com/yourname/work2api/internal/service"
	"github.com/yourname/work2api/pkg/log"
)

// CheckinJobServer 将签到调度与凭证刷新扫描作为 app server 运行。
type CheckinJobServer struct {
	log     *log.Logger
	job     *job.CheckinJob
	conf    *config.CodeBuddyConfig
	repo    repository.GatewayRepository
	sess    service.SessionService
	creds   service.CredentialService
	refresh *service.TokenRefreshService
}

func NewCheckinJobServer(
	log *log.Logger,
	checkinJob *job.CheckinJob,
	conf *config.CodeBuddyConfig,
	repo repository.GatewayRepository,
	sess service.SessionService,
	creds service.CredentialService,
	refresh *service.TokenRefreshService,
) *CheckinJobServer {
	return &CheckinJobServer{log: log, job: checkinJob, conf: conf, repo: repo, sess: sess, creds: creds, refresh: refresh}
}

// Start 先执行启动钩子（建户/加载池），再并行跑签到调度与刷新扫描。
func (s *CheckinJobServer) Start(ctx context.Context) error {
	if err := bootstrap.Startup(ctx, s.conf, s.repo, s.sess, s.creds, s.log); err != nil {
		return err
	}
	go s.job.Start(ctx)
	go s.refresh.RunLoop(ctx)
	<-ctx.Done()
	return nil
}

func (s *CheckinJobServer) Stop(ctx context.Context) error {
	s.job.Stop()
	return nil
}
