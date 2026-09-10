package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourname/work2api/pkg/log"
)

// TraeCallbackServer TRAE 登录回调监听器：独立端口（默认 18080）只服务 /authorize。
// TRAE 登录页强制回调 127.0.0.1；端口占用时仅告警不 fatal（自动捕获失效，
// 仍有粘贴回调 URL 兜底），网关主功能不受影响。
type TraeCallbackServer struct {
	port int
	srv  *http.Server
	log  *log.Logger
}

func NewTraeCallbackServer(port int, authorizeHandler gin.HandlerFunc, log *log.Logger) *TraeCallbackServer {
	g := gin.New()
	g.GET("/authorize", authorizeHandler)
	return &TraeCallbackServer{
		port: port,
		srv:  &http.Server{Handler: g, ReadHeaderTimeout: 10 * time.Second},
		log:  log,
	}
}

func (s *TraeCallbackServer) Start(_ context.Context) error {
	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		s.log.Sugar().Warnf("trae callback listener on %s unavailable (%v); automatic login capture disabled, use paste-import instead", addr, err)
		return nil
	}
	s.log.Sugar().Infof("trae callback server on %s (TRAE login /authorize)", addr)
	if err := s.srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		s.log.Sugar().Warnf("trae callback server stopped: %v", err)
	}
	return nil
}

func (s *TraeCallbackServer) Stop(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return s.srv.Shutdown(shutdownCtx)
}
