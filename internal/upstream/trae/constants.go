// Package trae TRAE SOLO 上游客户端：网页登录闭环所需的常量、登录 URL 构造、
// 回调解析、ExchangeToken 与 GetUserInfo。移植自 trae2api-web（纯标准库）。
package trae

// 上游技术常量（来自 trae2api-web 实测，禁止改动）。
const (
	AgentHost      = "https://trae-api-cn.mchost.guru"
	UgHost         = "https://api.trae.cn"
	OAuthHost      = "https://api.trae.com.cn"
	ConsoleHost    = "https://www.trae.cn"
	ClientID       = "en1oxy7wnw8j9n" // SOLO stable
	IdeVersion     = "0.1.52"
	IdeVersionCode = "20260811"

	// 端点
	EpExchange = "/cloudide/api/v3/trae/oauth/ExchangeToken"
	EpUserInfo = "/cloudide/api/v3/trae/GetUserInfo"
)

// Domain SOLO 账号归属域。
const Domain = "trae.cn"
