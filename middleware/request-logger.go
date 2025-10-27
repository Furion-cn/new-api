package middleware

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"one-api/common"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// EnableRequestBodyLogging 控制是否打印请求体，通过环境变量 ENABLE_REQUEST_BODY_LOGGING 控制
var EnableRequestBodyLogging bool = common.GetEnvOrDefaultBool("ENABLE_REQUEST_BODY_LOGGING", false)

func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取请求头
		headers := make(map[string]string)
		for k, v := range c.Request.Header {
			// 跳过敏感信息
			if strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "Cookie") {
				headers[k] = "***"
				continue
			}
			headers[k] = strings.Join(v, ", ")
		}

		// 获取 requestId（应该已经在 RequestId 中间件中设置）
		requestId := c.GetString(common.RequestIdKey)

		// 构建日志信息
		logInfo := fmt.Sprintf("Request: %s %s\tRequestID: %s\tClient IP: %s\tHeaders: %s\t",
			c.Request.Method,
			c.Request.URL.String(),
			requestId,
			c.ClientIP(),
			common.FormatMap(headers),
		)

		// 如果启用了请求体日志，则记录请求体
		if EnableRequestBodyLogging && c.Request.Method != "GET" {
			truncateMod := os.Getenv("LOG_TRUNCATE_TYPE")
			bodyInfo := common.LogRequestBody(c, truncateMod)
			if bodyInfo != "" {
				logInfo += fmt.Sprintf("\tBody: %s", bodyInfo)
			}
		}

		// 构建全链路上下文
		ctx := context.WithValue(c.Request.Context(), common.RequestIdKey, requestId)
		ctx = context.WithValue(ctx, "gin_context", c)

		body, _ := io.ReadAll(c.Request.Body)
		// 恢复请求体
		c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
		c.Set(common.CtxRequestBody, string(body))

		c.Set(common.CtxRequestHeaders, common.FormatMap(headers))

		common.LogInfo(ctx, logInfo)

		c.Next()
	}
}
