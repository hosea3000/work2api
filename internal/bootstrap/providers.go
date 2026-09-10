package bootstrap

import (
	"github.com/hosea3000/work2api/internal/config"
	"github.com/hosea3000/work2api/internal/upstream/codebuddy"
	"github.com/hosea3000/work2api/internal/upstream/trae"
	"github.com/hosea3000/work2api/internal/service"
)

// NewCodeBuddyClient 从配置构造上游客户端。
func NewCodeBuddyClient(conf *config.CodeBuddyConfig) *codebuddy.Client {
	return codebuddy.NewClient(conf.APIEndpoint, conf.CLIVersion)
}

// NewTraeClient 构造 TRAE OAuth 域客户端（登录闭环专用）。
func NewTraeClient() *trae.Client {
	return trae.New()
}

// NewRequestPolicies 从配置构造聊天请求策略。
func NewRequestPolicies(conf *config.CodeBuddyConfig) *service.RequestPolicies {
	return &service.RequestPolicies{
		ForcedTemperature:     conf.ForcedTemperature,
		StripModelNamespace:   true,
		ForcedReasoningModels: []string{"deepseek-v4-pro", "deepseek-v4-flash", "glm-5.1", "glm-5.2"},
		DefaultModel:          firstNonEmpty(conf.Models),
	}
}

// NewModelsServiceFromConfig 构造模型服务。
func NewModelsServiceFromConfig(conf *config.CodeBuddyConfig, client *codebuddy.Client, creds service.CodeBuddyCredentialService) *service.ModelsService {
	return service.NewModelsService(client, creds, conf.Models, conf.ModelsCacheTTL)
}

// NewTraeModelsServiceFromConfig 构造 TRAE 模型服务。
func NewTraeModelsServiceFromConfig(conf *config.CodeBuddyConfig, client *trae.Client, creds service.TraeCredentialService) *service.TraeModelsService {
	return service.NewTraeModelsService(client, creds, conf.ModelsCacheTTL)
}

func firstNonEmpty(list []string) string {
	for _, s := range list {
		if s != "" {
			return s
		}
	}
	return ""
}
