package controller

import (
	"fmt"
	"net/http"
	"one-api/common"
	"time"

	"github.com/gin-gonic/gin"
)

func Ping(c *gin.Context) {
	// 使用 context 检测连接
	done := make(chan struct{})
	go func() {
		select {
		case <-c.Request.Context().Done():
			common.LogInfo(c.Request.Context(), "Client disconnected during ping")
			close(done)
		case <-done:
			return
		}
	}()

	// 等待10秒，同时检查连接状态
	for i := 0; i < 10; i++ {
		select {
		case <-done:
			return
		default:
			time.Sleep(time.Second)
		}
	}

	// 设置响应头
	c.Writer.Header().Set("Content-Type", "text/plain")
	c.Writer.WriteHeader(http.StatusOK)

	// 写入响应
	_, err := c.Writer.Write([]byte("pong"))
	if err != nil {
		common.LogInfo(c.Request.Context(), fmt.Sprintf("write_response_failed: %s", err.Error()))
		return
	}

	common.LogInfo(c.Request.Context(), "ping success")
}
