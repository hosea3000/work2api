//go:build wireinject
// +build wireinject

package wire

import (
	"github.com/yourname/work2api/internal/handler"
	"github.com/yourname/work2api/internal/job"
	"github.com/yourname/work2api/internal/repository"
	"github.com/yourname/work2api/internal/router"
	"github.com/yourname/work2api/internal/server"
	"github.com/yourname/work2api/internal/service"
	"github.com/yourname/work2api/pkg/app"
	"github.com/yourname/work2api/pkg/jwt"
	"github.com/yourname/work2api/pkg/log"
	"github.com/yourname/work2api/pkg/server/http"
	"github.com/yourname/work2api/pkg/sid"
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
)

var serviceSet = wire.NewSet(
	service.NewService,
	service.NewUserService,
)

var handlerSet = wire.NewSet(
	handler.NewHandler,
	handler.NewUserHandler,
)

var jobSet = wire.NewSet(
	job.NewJob,
	job.NewUserJob,
)
var serverSet = wire.NewSet(
	server.NewHTTPServer,
	server.NewJobServer,
)

// build App
func newApp(
	httpServer *http.Server,
	jobServer *server.JobServer,
	// task *server.Task,
) *app.App {
	return app.NewApp(
		app.WithServer(httpServer, jobServer),
		app.WithName("demo-server"),
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
