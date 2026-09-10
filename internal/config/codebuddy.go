package config

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/viper"
)

// CodeBuddyConfig 承载 codebuddy 配置段。所有取值函数在启动 fail-fast 校验后被调用。
type CodeBuddyConfig struct {
	APIEndpoint       string
	AllowedEndpoints  []string
	Models            []string
	ForcedTemperature *float64
	RotationCount     int
	AutoCheckin       bool
	CheckinHour       int
	CheckinMinute     int
	DelayMinSeconds   float64
	DelayMaxSeconds   float64
	CLIVersion        string
	ModelsCacheTTL    int
	AdminUsername     string
	AdminPassword     string
	TraeCallbackPort  int
}

// DefaultModels 与参考实现 DEFAULT_CODEBUDDY_MODELS 对齐。
var DefaultModels = []string{"glm-5.2", "deepseek-v4-pro"}

// LoadCodeBuddyConfig 读取并校验 codebuddy 配置段；非法配置直接报错（fail-fast）。
func LoadCodeBuddyConfig(conf *viper.Viper) (*CodeBuddyConfig, error) {
	c := &CodeBuddyConfig{
		APIEndpoint:      strings.TrimRight(conf.GetString("codebuddy.api_endpoint"), "/"),
		CLIVersion:       conf.GetString("codebuddy.cli_version"),
		RotationCount:    conf.GetInt("codebuddy.rotation_count"),
		AutoCheckin:      conf.GetBool("codebuddy.auto_checkin_enabled"),
		CheckinHour:      conf.GetInt("codebuddy.checkin_hour"),
		CheckinMinute:    conf.GetInt("codebuddy.checkin_minute"),
		DelayMinSeconds:  conf.GetFloat64("codebuddy.background_delay_min_seconds"),
		DelayMaxSeconds:  conf.GetFloat64("codebuddy.background_delay_max_seconds"),
		ModelsCacheTTL:   conf.GetInt("codebuddy.models_cache_ttl_seconds"),
		AdminUsername:    conf.GetString("codebuddy.admin_username"),
		AdminPassword:    conf.GetString("codebuddy.admin_password"),
		TraeCallbackPort: conf.GetInt("trae.callback_port"),
		AllowedEndpoints: splitCSV(conf.GetString("codebuddy.allowed_endpoints")),
		Models:           splitCSV(conf.GetString("codebuddy.models")),
	}
	if c.CLIVersion == "" {
		c.CLIVersion = "2.107.0"
	}
	if c.RotationCount <= 0 {
		c.RotationCount = 1
	}
	if c.DelayMaxSeconds < c.DelayMinSeconds {
		return nil, fmt.Errorf("codebuddy.background_delay_max_seconds (%v) must be >= min (%v)", c.DelayMaxSeconds, c.DelayMinSeconds)
	}
	if c.CheckinHour < 0 || c.CheckinHour > 23 || c.CheckinMinute < 0 || c.CheckinMinute > 59 {
		return nil, fmt.Errorf("codebuddy.checkin_hour/checkin_minute out of range")
	}
	if c.ModelsCacheTTL <= 0 {
		c.ModelsCacheTTL = 30
	}
	if c.AdminUsername == "" {
		c.AdminUsername = "admin"
	}
	if c.AdminPassword == "" {
		return nil, fmt.Errorf("codebuddy.admin_password is required for initial admin bootstrap")
	}
	if c.TraeCallbackPort <= 0 {
		c.TraeCallbackPort = 18080
	}

	if u, err := url.Parse(c.APIEndpoint); err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("codebuddy.api_endpoint invalid: %q", c.APIEndpoint)
	}
	allowed := make([]string, 0, len(c.AllowedEndpoints))
	for _, ep := range c.AllowedEndpoints {
		ep = strings.TrimRight(strings.TrimSpace(ep), "/")
		if ep == "" {
			continue
		}
		if u, err := url.Parse(ep); err != nil || u.Scheme != "https" || u.Host == "" {
			return nil, fmt.Errorf("codebuddy.allowed_endpoints contains invalid endpoint: %q", ep)
		}
		allowed = append(allowed, ep)
	}
	if len(allowed) == 0 {
		allowed = []string{"https://copilot.tencent.com", "https://www.codebuddy.ai"}
	}
	c.AllowedEndpoints = allowed
	found := false
	for _, ep := range allowed {
		if ep == c.APIEndpoint {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("codebuddy.api_endpoint %q not in allowed_endpoints %v", c.APIEndpoint, allowed)
	}

	if ft := conf.GetString("codebuddy.forced_temperature"); ft != "" {
		var v float64
		if _, err := fmt.Sscanf(ft, "%g", &v); err != nil {
			return nil, fmt.Errorf("codebuddy.forced_temperature invalid: %q", ft)
		}
		c.ForcedTemperature = &v
	}
	if len(c.Models) == 0 {
		c.Models = append([]string{}, DefaultModels...)
	}
	return c, nil
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
