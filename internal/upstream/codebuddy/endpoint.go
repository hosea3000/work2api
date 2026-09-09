package codebuddy

import (
	"runtime"
)

// defaultEndpoint 与参考实现默认上游一致。
const defaultEndpoint = "https://copilot.tencent.com"

// HostFromEndpoint 从端点 URL 提取 host。
func HostFromEndpoint(endpoint string) string {
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	s := endpoint
	// strip scheme
	for _, prefix := range []string{"https://", "http://"} {
		if len(s) > len(prefix) && s[:len(prefix)] == prefix {
			s = s[len(prefix):]
			break
		}
	}
	// strip path
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return s[:i]
		}
	}
	return s
}

func stainlessArch() string {
	switch runtime.GOARCH {
	case "arm64":
		return "arm64"
	case "amd64":
		return "x64"
	default:
		return runtime.GOARCH
	}
}

func stainlessOS() string {
	switch runtime.GOOS {
	case "darwin":
		return "MacOS"
	case "linux":
		return "Linux"
	case "windows":
		return "Windows"
	default:
		return runtime.GOOS
	}
}
