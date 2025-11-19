package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"one-api/common"
	"one-api/model"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// responseWriter 包装 gin.ResponseWriter 以捕获响应体
type googleStorageResponseWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *googleStorageResponseWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// GoogleStorageLogger 记录 Google Storage 接口请求日志的中间件
func GoogleStorageLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 只记录 Google 相关的路径（包括 /google/token）
		path := c.Request.URL.Path
		if !strings.HasPrefix(path, "/google/storage/") &&
			!strings.HasPrefix(path, "/google/download/storage/") &&
			!strings.HasPrefix(path, "/google/upload/storage/") &&
			!strings.HasPrefix(path, "/google/token") {
			c.Next()
			return
		}

		startTime := time.Now()

		// 创建日志记录对象
		logEntry := &model.GoogleStorageLog{
			Method:   c.Request.Method,
			URL:      c.Request.URL.String(),
			ClientIP: c.ClientIP(),
		}

		// 获取 OAuth ID
		if oauthValue, exists := c.Get("oauth"); exists && oauthValue != nil {
			if oauth, ok := oauthValue.(*model.OAuth); ok && oauth != nil {
				logEntry.OAuthId = int(oauth.ID)
			}
		}

		// 如果没有从 context 获取到 OAuth，尝试从 Authorization header 中查询
		if logEntry.OAuthId == 0 {
			authHeader := c.Request.Header.Get("Authorization")
			if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
				clientToken := strings.TrimPrefix(authHeader, "Bearer ")
				clientToken = strings.TrimSpace(clientToken)
				oauth, err := model.GetOAuthByClientAccessToken(clientToken)
				if err == nil && oauth != nil {
					logEntry.OAuthId = int(oauth.ID)
				}
			}
		}

		// 记录请求头
		requestHeadersMap := make(map[string]string)
		for key, values := range c.Request.Header {
			requestHeadersMap[key] = strings.Join(values, ", ")
		}
		requestHeadersJSON, _ := json.Marshal(requestHeadersMap)
		logEntry.RequestHeaders = string(requestHeadersJSON)

		// 读取并记录请求体
		var requestBody []byte
		if c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			// 恢复请求体，供后续处理使用
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// 对请求体进行截断处理
		contentType := c.Request.Header.Get("Content-Type")
		if strings.HasPrefix(contentType, "multipart/") {
			// 对 multipart 数据进行简化记录
			logEntry.RequestBody = common.ParseMultipartFormData(requestBody, contentType)
		} else {
			// 其他类型的请求体
			logEntry.RequestBody = common.TruncatedBody(string(requestBody), contentType)
		}

		// 包装 ResponseWriter 以捕获响应
		responseWriter := &googleStorageResponseWriter{
			ResponseWriter: c.Writer,
			body:           bytes.NewBuffer(nil),
		}
		c.Writer = responseWriter

		// 处理请求
		c.Next()

		// 请求处理完成，记录响应信息
		duration := time.Since(startTime)

		// 记录响应头
		responseHeadersMap := make(map[string]string)
		for key, values := range c.Writer.Header() {
			responseHeadersMap[key] = strings.Join(values, ", ")
		}
		responseHeadersJSON, _ := json.Marshal(responseHeadersMap)
		logEntry.ResponseHeaders = string(responseHeadersJSON)

		// 记录响应体（进行截断处理）
		responseBody := responseWriter.body.Bytes()
		responseContentType := c.Writer.Header().Get("Content-Type")
		logEntry.ResponseBody = common.TruncatedBody(string(responseBody), responseContentType)

		// 异步保存日志到数据库
		go func() {
			if err := model.DB.Create(logEntry).Error; err != nil {
				common.LogError(c.Request.Context(), "Failed to save GoogleStorageLog: "+err.Error())
			} else {
				common.LogInfo(c.Request.Context(), fmt.Sprintf("GoogleStorageLog saved: OAuthId=%d, Method=%s, URL=%s, ClientIP=%s, Duration=%s",
					logEntry.OAuthId, logEntry.Method, logEntry.URL, logEntry.ClientIP, duration.String()))
			}
		}()
	}
}
