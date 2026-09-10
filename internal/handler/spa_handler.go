package handler

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// SPAStaticHandler 托管 web/dist 构建产物，未匹配路径回退 index.html。
type SPAStaticHandler struct {
	fs   embed.FS
	root string
}

// NewSPAStaticHandler 用嵌入文件系统构造；root 为 dist 在 fs 内的路径前缀。
func NewSPAStaticHandler(dist embed.FS, root string) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := strings.TrimPrefix(c.Request.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		data, err := dist.ReadFile(root + "/" + path)
		if err != nil {
			// SPA fallback
			data, err = dist.ReadFile(root + "/index.html")
			if err != nil {
				c.Status(http.StatusNotFound)
				return
			}
			path = "index.html"
		}
		contentType := mimeByExt(path)
		if contentType != "" {
			c.Header("Content-Type", contentType)
		}
		c.Data(http.StatusOK, contentType, data)
	}
}

// RegisterSPARoutes 挂载静态资源路由（优先级低于已注册 API 路由）。
func RegisterSPARoutes(s *gin.Engine, dist embed.FS, root string) {
	sub, err := fs.Sub(dist, root)
	if err == nil {
		// Sub 后 FS 根 = dist/；StripPrefix("/assets")（不带尾斜杠）保留前导 /，
		// FileServer 在 FS 内以 assets/xxx 命中 dist/assets/xxx。
		fileServer := http.StripPrefix("/assets", http.FileServer(http.FS(sub)))
		s.GET("/assets/*filepath", func(c *gin.Context) {
			fileServer.ServeHTTP(c.Writer, c.Request)
		})
	}
	spa := NewSPAStaticHandler(dist, root)
	s.NoRoute(spa)
}

func mimeByExt(path string) string {
	switch {
	case strings.HasSuffix(path, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(path, ".js"):
		return "application/javascript; charset=utf-8"
	case strings.HasSuffix(path, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(path, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(path, ".png"):
		return "image/png"
	case strings.HasSuffix(path, ".ico"):
		return "image/x-icon"
	case strings.HasSuffix(path, ".json"):
		return "application/json; charset=utf-8"
	case strings.HasSuffix(path, ".woff2"):
		return "font/woff2"
	case strings.HasSuffix(path, ".woff"):
		return "font/woff"
	default:
		return "application/octet-stream"
	}
}
