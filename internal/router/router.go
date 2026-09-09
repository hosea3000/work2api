package router

import (
	"github.com/yourname/work2api/internal/handler"
	"github.com/yourname/work2api/pkg/jwt"
	"github.com/yourname/work2api/pkg/log"
	"github.com/spf13/viper"
)

type RouterDeps struct {
	Logger      *log.Logger
	Config      *viper.Viper
	JWT         *jwt.JWT
	UserHandler *handler.UserHandler
}
