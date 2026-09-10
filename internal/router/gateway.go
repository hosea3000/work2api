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
	CodeBuddyCredentialHandler *handler.CodeBuddyCredentialHandler
	TraeCredentialHandler      *handler.TraeCredentialHandler
	OpenAIHandler      *handler.OpenAIHandler
	AdminStubHandler   *handler.AdminStubHandler
	CodeBuddyAuthHandler *handler.CodeBuddyAuthHandler
	TraeAuthHandler    *handler.TraeAuthHandler
	TraeChatHandler    *handler.TraeChatHandler
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

		// CodeBuddy 凭证
		admin.GET("/codebuddy/credentials", deps.CodeBuddyCredentialHandler.List)
		admin.POST("/codebuddy/credentials", deps.CodeBuddyCredentialHandler.Create)
		admin.DELETE("/codebuddy/credentials/:credential_id", deps.CodeBuddyCredentialHandler.Delete)
		admin.POST("/codebuddy/credentials/rotation/toggle", deps.CodeBuddyCredentialHandler.ToggleRotation)
		admin.POST("/codebuddy/credentials/:credential_id/select", deps.CodeBuddyCredentialHandler.Select)
		admin.POST("/codebuddy/credentials/:credential_id/test", deps.CodeBuddyCredentialHandler.Test)
		admin.POST("/codebuddy/credentials/:credential_id/daily-checkin", deps.CodeBuddyCredentialHandler.DailyCheckin)
		admin.GET("/codebuddy/credentials/:credential_id/quota", deps.AdminStubHandler.CredentialQuota)

		// TRAE 凭证
		admin.GET("/trae/credentials", deps.TraeCredentialHandler.List)
		admin.DELETE("/trae/credentials/:credential_id", deps.TraeCredentialHandler.Delete)
		admin.POST("/trae/credentials/rotation/toggle", deps.TraeCredentialHandler.ToggleRotation)
		admin.POST("/trae/credentials/:credential_id/select", deps.TraeCredentialHandler.Select)
		admin.POST("/trae/credentials/:credential_id/test", deps.TraeCredentialHandler.Test)
		admin.POST("/trae/credentials/:credential_id/daily-checkin", deps.TraeCredentialHandler.DailyCheckin)

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

	// TRAE 网页登录（会话 Cookie 保护）
	traeAuth := s.Group("/api/admin/trae/login")
	traeAuth.Use(middleware.SessionAuth(deps.Logger, deps.SessionService))
	{
		traeAuth.POST("/start", deps.TraeAuthHandler.Start)
		traeAuth.GET("/result", deps.TraeAuthHandler.Result)
		traeAuth.POST("/cancel", deps.TraeAuthHandler.Cancel)
		traeAuth.POST("/import", deps.TraeAuthHandler.Import)
	}

	// TRAE 登录回调落点（公共：浏览器 302 落点，归属校验在 service 内完成）
	s.GET("/authorize", deps.TraeAuthHandler.Authorize)

	// CodeBuddy OpenAI 兼容入口（API Key 保护）
	openai := s.Group("/codebuddy/openai/v1")
	openai.Use(middleware.APIKeyAuth(deps.Logger, deps.APIKeyService))
	{
		openai.POST("/chat/completions", deps.OpenAIHandler.ChatCompletions)
		openai.GET("/models", deps.OpenAIHandler.Models)
	}

	// TRAE SOLO OpenAI 兼容入口（独立端点，同一套 API Key）
	trae := s.Group("/trae/openai/v1")
	trae.Use(middleware.APIKeyAuth(deps.Logger, deps.APIKeyService))
	{
		trae.POST("/chat/completions", deps.TraeChatHandler.ChatCompletions)
		trae.GET("/models", deps.TraeChatHandler.Models)
	}
}
