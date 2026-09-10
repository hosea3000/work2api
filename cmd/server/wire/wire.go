//go:build wireinject
// +build wireinject

package wire

import (
	"github.com/hosea3000/work2api/internal/bootstrap"
	"github.com/hosea3000/work2api/internal/config"
	"github.com/hosea3000/work2api/internal/handler"
	"github.com/hosea3000/work2api/internal/job"
	"github.com/hosea3000/work2api/internal/repository"
	"github.com/hosea3000/work2api/internal/router"
	"github.com/hosea3000/work2api/internal/server"
	"github.com/hosea3000/work2api/internal/service"
	"github.com/hosea3000/work2api/pkg/app"
	"github.com/hosea3000/work2api/pkg/jwt"
	"github.com/hosea3000/work2api/pkg/log"
	"github.com/hosea3000/work2api/pkg/server/http"
	"github.com/hosea3000/work2api/pkg/sid"
	"github.com/google/wire"
	"github.com/spf13/viper"
)

var repositorySet = wire.NewSet(
	repository.NewDB,
	//repository.NewRedis,
	//repository.NewMongo,
	repository.NewRepository,
	repository.NewTransaction,
	repository.NewUserRepository,
	repository.NewGatewayRepository,
	repository.NewCodeBuddyCredentialRepository,
	repository.NewTraeCredentialRepository,
	repository.NewPoolStateRepository,
)

var serviceSet = wire.NewSet(
	service.NewService,
	service.NewUserService,
	config.LoadCodeBuddyConfig,
	bootstrap.NewCodeBuddyClient,
	bootstrap.NewRequestPolicies,
	bootstrap.NewModelsServiceFromConfig,
	service.NewCredentialPool,
	service.NewTraeCredentialPool,
	service.NewCodeBuddyCredentialService,
	service.NewTraeCredentialService,
	service.NewCodeBuddyCheckinService,
	service.NewTraeCheckinService,
	service.NewAPIKeyService,
	service.NewSessionService,
	service.NewChatExecutor,
	service.NewTraeChatExecutor,
	bootstrap.NewTraeModelsServiceFromConfig,
	service.NewQuotaService,
)

var handlerSet = wire.NewSet(
	handler.NewHandler,
	handler.NewUserHandler,
	handler.NewAuthHandler,
	handler.NewAPIKeyHandler,
	handler.NewCodeBuddyCredentialHandler,
	handler.NewTraeCredentialHandler,
	handler.NewOpenAIHandler,
	handler.NewAdminStubHandler,
	handler.NewCodeBuddyAuthHandler,
	handler.NewTraeAuthHandler,
	handler.NewTraeChatHandler,
	service.NewAuthStateStore,
	service.NewOAuthService,
	service.NewTokenRefreshService,
	service.NewTraeTokenRefreshService,
	service.NewTraeLoginService,
	bootstrap.NewTraeClient,
)

var jobSet = wire.NewSet(
	job.NewJob,
	job.NewUserJob,
	job.NewCheckinJob,
)
var serverSet = wire.NewSet(
	server.NewHTTPServer,
	server.NewJobServer,
	server.NewCheckinJobServer,
	server.NewTraeCallbackServerFromDeps,
)

// build App
func newApp(
	httpServer *http.Server,
	jobServer *server.JobServer,
	checkinServer *server.CheckinJobServer,
	traeCallbackServer *server.TraeCallbackServer,
) *app.App {
	return app.NewApp(
		app.WithServer(httpServer, jobServer, checkinServer, traeCallbackServer),
		app.WithName("work2api"),
	)
}

func NewWire(*viper.Viper, *log.Logger) (*app.App, func(), error) {
	panic(wire.Build(
		repositorySet,
		serviceSet,
		handlerSet,
		jobSet,
		serverSet,
		wire.Struct(new(router.RouterDeps), "*"),
		sid.NewSid,
		jwt.NewJwt,
		newApp,
	))
}
