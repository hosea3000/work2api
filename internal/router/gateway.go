package router

import (
	"github.com/gin-gonic/gin"
	"github.com/yourname/work2api/internal/handler"
	"github.com/yourname/work2api/internal/middleware"
	"github.com/yourname/work2api/internal/service"
	"github.com/yourname/work2api/pkg/jwt"
	"github.com/yourname/work2api/pkg/log"
	"github.com/spf13/viper"
)

type RouterDeps struct {
	Logger             *log.Logger
	Config             *viper.Viper
	JWT                *jwt.JWT
	UserHandler        *handler.UserHandler
	AuthHandler        *handler.AuthHandler
	APIKeyHandler      *handler.APIKeyHandler
	CredentialHandler  *handler.CredentialHandler
	OpenAIHandler      *handler.OpenAIHandler
	AdminStubHandler   *handler.AdminStubHandler
	CodeBuddyAuthHandler *handler.CodeBuddyAuthHandler
	SessionService     service.SessionService
	APIKeyService      service.APIKeyService
}

func InitUserRouter(
	deps RouterDeps,
	r *gin.RouterGroup,
) {
	// No route group has permission
	noAuthRouter := r.Group("/")
	{
		noAuthRouter.POST("/register", deps.UserHandler.Register)
		noAuthRouter.POST("/login", deps.UserHandler.Login)
	}
	// Non-strict permission routing group
	noStrictAuthRouter := r.Group("/").Use(middleware.NoStrictAuth(deps.JWT, deps.Logger))
	{
		noStrictAuthRouter.GET("/user", deps.UserHandler.GetProfile)
	}

	// Strict permission routing group
	strictAuthRouter := r.Group("/").Use(middleware.StrictAuth(deps.JWT, deps.Logger))
	{
		strictAuthRouter.PUT("/user", deps.UserHandler.UpdateProfile)
	}
}

// InitGatewayRouter 挂载网关全部路由（登录、管理台、OpenAI 兼容、健康检查）。
func InitGatewayRouter(deps RouterDeps, s *gin.Engine) {
	// 健康检查
	s.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy", "service": "work2api"})
	})

	// 管理台登录（无鉴权）
	auth := s.Group("/auth")
	{
		auth.POST("/login", deps.AuthHandler.Login)
		auth.GET("/session", deps.AuthHandler.Session)
		auth.POST("/logout", deps.AuthHandler.Logout)
	}

	// 管理台 API（会话 Cookie 保护）
	admin := s.Group("/api/admin")
	admin.Use(middleware.SessionAuth(deps.Logger, deps.SessionService))
	{
		admin.GET("/status", deps.AdminStubHandler.Status)
		admin.GET("/settings", deps.AdminStubHandler.GetSettings)
		admin.PUT("/settings", deps.AdminStubHandler.SaveSettings)

		// API Keys
		admin.GET("/api-keys", deps.APIKeyHandler.List)
		admin.POST("/api-keys", deps.APIKeyHandler.Create)
		admin.DELETE("/api-keys/:key_id", deps.APIKeyHandler.Delete)

		// Credentials
		admin.GET("/credentials", deps.CredentialHandler.List)
		admin.POST("/credentials", deps.CredentialHandler.Create)
		admin.DELETE("/credentials/:credential_id", deps.CredentialHandler.Delete)
		admin.POST("/credentials/rotation/toggle", deps.CredentialHandler.ToggleRotation)
		admin.POST("/credentials/:credential_id/select", deps.CredentialHandler.Select)
		admin.POST("/credentials/:credential_id/test", deps.CredentialHandler.Test)
		admin.POST("/credentials/:credential_id/daily-checkin", deps.CredentialHandler.DailyCheckin)
		admin.GET("/credentials/:credential_id/quota", deps.AdminStubHandler.CredentialQuota)

		// 统计（打桩）
		admin.GET("/stats/overview", deps.AdminStubHandler.StatsOverview)
		admin.GET("/stats/requests", deps.AdminStubHandler.StatsRequests)
		admin.GET("/stats/models", deps.AdminStubHandler.StatsOverview)
	}

	// CodeBuddy 设备授权（会话 Cookie 保护，路径对齐前端 admin.ts）
	codebuddyAuth := s.Group("/codebuddy")
	codebuddyAuth.Use(middleware.SessionAuth(deps.Logger, deps.SessionService))
	{
		codebuddyAuth.POST("/auth/start", deps.CodeBuddyAuthHandler.Start)
		codebuddyAuth.POST("/auth/poll", deps.CodeBuddyAuthHandler.Poll)
		codebuddyAuth.POST("/auth/cancel", deps.CodeBuddyAuthHandler.Cancel)
	}

	// OpenAI 兼容入口（API Key 保护）
	openai := s.Group("/openai/v1")
	openai.Use(middleware.APIKeyAuth(deps.Logger, deps.APIKeyService))
	{
		openai.POST("/chat/completions", deps.OpenAIHandler.ChatCompletions)
		openai.GET("/models", deps.OpenAIHandler.Models)
	}
}
