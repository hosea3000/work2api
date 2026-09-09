package bootstrap

import (
	"github.com/yourname/work2api/internal/config"
	"github.com/yourname/work2api/internal/upstream/codebuddy"
	"github.com/yourname/work2api/internal/service"
)

// NewCodeBuddyClient 从配置构造上游客户端。
func NewCodeBuddyClient(conf *config.CodeBuddyConfig) *codebuddy.Client {
	return codebuddy.NewClient(conf.APIEndpoint, conf.CLIVersion)
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
func NewModelsServiceFromConfig(conf *config.CodeBuddyConfig, client *codebuddy.Client, creds service.CredentialService) *service.ModelsService {
	return service.NewModelsService(client, creds, conf.Models, conf.ModelsCacheTTL)
}

func firstNonEmpty(list []string) string {
	for _, s := range list {
		if s != "" {
			return s
		}
	}
	return ""
}
