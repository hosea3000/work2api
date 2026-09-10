package bootstrap

import (
	"context"

	"github.com/yourname/work2api/internal/config"
	"github.com/yourname/work2api/internal/repository"
	"github.com/yourname/work2api/internal/service"
	"github.com/yourname/work2api/pkg/log"
)

// Startup 服务启动钩子：补齐 credential 新列、建管理员、加载凭证池。
func Startup(ctx context.Context, conf *config.CodeBuddyConfig, repo repository.GatewayRepository, sessions service.SessionService, creds service.CredentialService, logger *log.Logger) error {
	// 幂等迁移：存量库补 provider/machine_id/device_id 列（AutoMigrate 仅在 cmd/migration 中执行）
	if err := repo.MigrateCredentialColumns(ctx); err != nil {
		return err
	}
	logger.Info("credential columns migration ok")
	if err := sessions.BootstrapIfEmpty(ctx, conf.AdminUsername, conf.AdminPassword); err != nil {
		return err
	}
	logger.Info("admin bootstrap ok")
	creds.PoolReload(ctx)
	logger.Info("credential pool loaded")
	return nil
}
