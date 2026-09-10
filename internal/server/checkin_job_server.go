package server

import (
	"context"

	"github.com/hosea3000/work2api/internal/bootstrap"
	"github.com/hosea3000/work2api/internal/config"
	"github.com/hosea3000/work2api/internal/job"
	"github.com/hosea3000/work2api/internal/repository"
	"github.com/hosea3000/work2api/internal/service"
	"github.com/hosea3000/work2api/pkg/log"
)

// CheckinJobServer 将签到调度与两个 provider 的凭证刷新扫描作为 app server 运行。
type CheckinJobServer struct {
	log       *log.Logger
	job       *job.CheckinJob
	conf      *config.CodeBuddyConfig
	repo      repository.GatewayRepository
	sess      service.SessionService
	cbCreds   service.CodeBuddyCredentialService
	traeCreds service.TraeCredentialService
	refresh   *service.TokenRefreshService
	trae      *service.TraeTokenRefreshService
	quota     service.QuotaService
}

func NewCheckinJobServer(
	log *log.Logger,
	checkinJob *job.CheckinJob,
	conf *config.CodeBuddyConfig,
	repo repository.GatewayRepository,
	sess service.SessionService,
	cbCreds service.CodeBuddyCredentialService,
	traeCreds service.TraeCredentialService,
	refresh *service.TokenRefreshService,
	traeRefresh *service.TraeTokenRefreshService,
	quota service.QuotaService,
) *CheckinJobServer {
	return &CheckinJobServer{log: log, job: checkinJob, conf: conf, repo: repo, sess: sess, cbCreds: cbCreds, traeCreds: traeCreds, refresh: refresh, trae: traeRefresh, quota: quota}
}

// Start 先执行启动钩子（建户/加载池），再并行跑签到调度与双 provider 刷新扫描。
func (s *CheckinJobServer) Start(ctx context.Context) error {
	if err := bootstrap.Startup(ctx, s.conf, s.repo, s.sess, s.cbCreds, s.traeCreds, s.log); err != nil {
		return err
	}
	go s.job.Start(ctx)
	go s.refresh.RunLoop(ctx)
	go s.trae.RunLoop(ctx)
	go s.quota.RunLoop(ctx)
	<-ctx.Done()
	return nil
}

func (s *CheckinJobServer) Stop(ctx context.Context) error {
	s.job.Stop()
	return nil
}
