package bootstrap

import (
	"context"

	"github.com/yourname/work2api/internal/config"
	"github.com/yourname/work2api/internal/repository"
	"github.com/yourname/work2api/internal/service"
	"github.com/yourname/work2api/pkg/log"
)

// Startup 服务启动钩子：确保表结构、建管理员、加载两个 provider 的凭证池。
func Startup(
	ctx context.Context,
	conf *config.CodeBuddyConfig,
	repo repository.GatewayRepository,
	sessions service.SessionService,
	cbCreds service.CodeBuddyCredentialService,
	traeCreds service.TraeCredentialService,
	logger *log.Logger,
) error {
	// 自愈建表：全新/删库启动无需先跑 cmd/migration
	if err := repo.EnsureSchema(ctx); err != nil {
		return err
	}
	logger.Info("schema ensured")
	if err := sessions.BootstrapIfEmpty(ctx, conf.AdminUsername, conf.AdminPassword); err != nil {
		return err
	}
	logger.Info("admin bootstrap ok")
	cbCreds.PoolReload(ctx)
	traeCreds.PoolReload(ctx)
	logger.Info("credential pools loaded")
	return nil
}
