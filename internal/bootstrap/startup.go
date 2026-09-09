package bootstrap

import (
	"context"

	"github.com/yourname/work2api/internal/config"
	"github.com/yourname/work2api/internal/repository"
	"github.com/yourname/work2api/internal/service"
	"github.com/yourname/work2api/pkg/log"
)

// Startup 服务启动钩子：建管理员、加载凭证池。
func Startup(ctx context.Context, conf *config.CodeBuddyConfig, repo repository.GatewayRepository, sessions service.SessionService, creds service.CredentialService, logger *log.Logger) error {
	if err := sessions.BootstrapIfEmpty(ctx, conf.AdminUsername, conf.AdminPassword); err != nil {
		return err
	}
	logger.Info("admin bootstrap ok")
	creds.PoolReload(ctx)
	logger.Info("credential pool loaded")
	return nil
}
