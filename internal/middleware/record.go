package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hosea3000/work2api/internal/service"
	"github.com/hosea3000/work2api/pkg/log"
	"go.uber.org/zap"
)

// RecordRequest 记录 chat completions 调用：处理完成后按响应状态码判定成功与否并落库。
// 写库失败仅记日志，不改变响应；挂载在鉴权之后，鉴权失败的请求不计入。
func RecordRequest(logger *log.Logger, stats service.StatsService) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		success := c.Writer.Status() < 400
		// 客户端断开会取消请求上下文，记录不应因此丢失。
		ctx := context.WithoutCancel(c.Request.Context())
		if err := stats.Record(ctx, time.Now().Unix(), success); err != nil && logger != nil {
			logger.Error("record request stat failed", zap.Error(err))
		}
	}
}
