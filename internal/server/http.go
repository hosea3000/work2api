package server

import (
	"github.com/gin-gonic/gin"
	"github.com/yourname/work2api/docs"
	"github.com/yourname/work2api/internal/handler"
	"github.com/yourname/work2api/internal/middleware"
	"github.com/yourname/work2api/internal/router"
	"github.com/yourname/work2api/pkg/server/http"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func NewHTTPServer(
	deps router.RouterDeps,
) *http.Server {
	if deps.Config.GetString("env") == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}
	s := http.NewServer(
		gin.Default(),
		deps.Logger,
		http.WithServerHost(deps.Config.GetString("http.host")),
		http.WithServerPort(deps.Config.GetInt("http.port")),
	)

	// swagger doc
	docs.SwaggerInfo.BasePath = "/v1"
	s.GET("/swagger/*any", ginSwagger.WrapHandler(
		swaggerfiles.Handler,
		//ginSwagger.URL(fmt.Sprintf("http://localhost:%d/swagger/doc.json", deps.Config.GetInt("app.http.port"))),
		ginSwagger.DefaultModelsExpandDepth(-1),
		ginSwagger.PersistAuthorization(true),
	))

	s.Use(
		middleware.CORSMiddleware(),
		middleware.ResponseLogMiddleware(deps.Logger),
		middleware.RequestLogMiddleware(deps.Logger),
		//middleware.SignMiddleware(log),
	)

	router.InitGatewayRouter(deps, s.Engine)
	handler.RegisterSPARoutes(s.Engine, distFS, "webdist/dist")

	v1 := s.Group("/v1")
	router.InitUserRouter(deps, v1)

	return s
}
