package controller

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var grafanaUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许跨域
	},
}

// GrafanaProxy 代理Grafana请求，支持HTTP和WebSocket
func GrafanaProxy(c *gin.Context) {
	// 检查是否是WebSocket升级请求
	if websocket.IsWebSocketUpgrade(c.Request) {
		GrafanaWebSocketProxy(c)
		return
	}

	// 普通HTTP请求代理
	GrafanaHTTPProxy(c)
}

// GrafanaHTTPProxy 处理普通HTTP/HTTPS请求
func GrafanaHTTPProxy(c *gin.Context) {
	// Grafana已配置为在子路径 /api/grafana 下运行，保留完整路径
	originalPath := c.Request.URL.Path

	// 构建目标URL，保留完整路径
	targetURL := "https://grafana.furion-tech.com" + originalPath
	if c.Request.URL.RawQuery != "" {
		targetURL += "?" + c.Request.URL.RawQuery
	}

	common.SysLog("Grafana proxy forwarding: " + c.Request.Method + " " + originalPath + " -> " + targetURL)

	// 创建新的请求
	req, err := http.NewRequest(c.Request.Method, targetURL, c.Request.Body)
	if err != nil {
		common.SysError("Failed to create proxy request: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create proxy request",
		})
		return
	}

	// 注意：使用完整 URL 创建请求时，req.URL.Path 已经被设置为 originalPath
	// 不需要再次设置，但要确保查询参数正确
	if c.Request.URL.RawQuery != "" {
		req.URL.RawQuery = c.Request.URL.RawQuery
	}

	// 复制请求头，保留所有头部（包括 Origin、Cookie、Authorization 等）
	for key, values := range c.Request.Header {
		// 只跳过一些会导致问题的头部
		lowerKey := strings.ToLower(key)
		if lowerKey == "host" {
			// Host 头需要设置为目标服务器
			continue
		}
		if lowerKey == "content-length" {
			// Content-Length 会自动设置
			continue
		}
		if lowerKey == "origin" {
			// 修改 Origin 头为 Grafana 服务器的 Origin
			req.Header.Set("Origin", "https://grafana.furion-tech.com")
			continue
		}
		if lowerKey == "referer" {
			// 修改 Referer 头为 Grafana 服务器的 Referer
			req.Header.Set("Referer", "https://grafana.furion-tech.com"+originalPath)
			continue
		}
		// 保留所有其他头部，特别是 Authorization 等
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// 设置Host头
	req.Host = "grafana.furion-tech.com"

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		common.SysError("Failed to proxy request: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to proxy request to Grafana",
		})
		return
	}
	defer resp.Body.Close()

	// 记录响应状态码和响应头用于调试
	common.SysLog("Grafana proxy response: " + c.Request.Method + " " + originalPath + " -> Status: " + fmt.Sprintf("%d", resp.StatusCode))
	common.SysLog("Grafana response headers: " + fmt.Sprintf("%+v", resp.Header))

	// 读取响应体
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		common.SysError("Failed to read response body: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to read response body",
		})
		return
	}

	// 构建代理基础 URL（用于替换 URL）
	proxyScheme := "http"
	if c.Request.TLS != nil {
		proxyScheme = "https"
	}
	proxyHost := c.Request.Host
	proxyBaseURL := proxyScheme + "://" + proxyHost + "/api/grafana"

	// 处理 Location 重定向头
	if location := resp.Header.Get("Location"); location != "" {
		// 替换 Location 头中的 URL
		newLocation := strings.ReplaceAll(location, "https://test.furion-tech.com:82", proxyBaseURL)
		newLocation = strings.ReplaceAll(newLocation, "https://www.furion-tech.com", proxyBaseURL)
		newLocation = strings.ReplaceAll(newLocation, "https://grafana.furion-tech.com", proxyBaseURL)
		newLocation = strings.ReplaceAll(newLocation, "http://grafana.furion-tech.com", proxyBaseURL)
		// if strings.HasPrefix(newLocation, "/grafana") {
		// 	newLocation = strings.Replace(newLocation, "/grafana", "/api/grafana", 1)
		// }
		resp.Header.Set("Location", newLocation)
		common.SysLog("Grafana Location header replaced: " + location + " -> " + newLocation)
	}

	// 获取 Content-Type
	// contentType := resp.Header.Get("Content-Type")
	// // 如果是 HTML、JavaScript、CSS 或 JSON 内容，需要替换 URL
	// if strings.Contains(contentType, "text/html") ||
	// 	strings.Contains(contentType, "application/javascript") ||
	// 	strings.Contains(contentType, "text/javascript") ||
	// 	strings.Contains(contentType, "text/css") ||
	// 	strings.Contains(contentType, "application/json") {

	// 	bodyStr := string(bodyBytes)

	// 	// 需要替换的 URL 模式（按顺序替换，先替换完整 URL，再替换相对路径）
	// 	replacements := []struct {
	// 		old string
	// 		new string
	// 	}{
	// 		// 替换完整的 Grafana 服务器地址（带协议和端口）
	// 		{"http://1.117.220.28:81/grafana", proxyBaseURL},
	// 		{"https://grafana.furion-tech.com", proxyBaseURL},
	// 		{"http://grafana.furion-tech.com", proxyBaseURL},
	// 		// 替换相对路径（确保只替换路径开头的 /grafana）
	// 		{`"/grafana/`, `"/api/grafana/`},
	// 		{`'/grafana/`, `'/api/grafana/`},
	// 		{`"/grafana"`, `"/api/grafana"`},
	// 		{`'/grafana'`, `'/api/grafana'`},
	// 		{`"/grafana?`, `"/api/grafana?`},
	// 		{`'/grafana?`, `'/api/grafana?`},
	// 		{`href="/grafana`, `href="/api/grafana`},
	// 		{`src="/grafana`, `src="/api/grafana`},
	// 		{`url("/grafana`, `url("/api/grafana`},
	// 		{`url('/grafana`, `url('/api/grafana`},
	// 		{`url( "/grafana`, `url( "/api/grafana`},
	// 		{`url( '/grafana`, `url( '/api/grafana`},
	// 	}

	// 	for _, repl := range replacements {
	// 		bodyStr = strings.ReplaceAll(bodyStr, repl.old, repl.new)
	// 	}

	// 	bodyBytes = []byte(bodyStr)
	// 	common.SysLog("Grafana response body URL replaced for path: " + originalPath)
	// }

	// 复制响应头，确保 Set-Cookie 头的所有值都被正确转发
	for key, values := range resp.Header {
		lowerKey := strings.ToLower(key)
		// Set-Cookie 头需要特殊处理，每个值都要单独设置
		if lowerKey == "set-cookie" {
			for _, value := range values {
				c.Writer.Header().Add(key, value)
			}
		} else if lowerKey == "content-length" {
			// 更新 Content-Length 为修改后的长度
			c.Header(key, fmt.Sprintf("%d", len(bodyBytes)))
		} else {
			// 其他响应头直接复制
			for _, value := range values {
				c.Header(key, value)
			}
		}
	}

	// 设置状态码
	c.Status(resp.StatusCode)

	// 写入修改后的响应体
	_, err = io.Copy(c.Writer, bytes.NewReader(bodyBytes))
	if err != nil {
		common.SysError("Failed to write response body: " + err.Error())
	}
}

// GrafanaWebSocketProxy 处理WebSocket/WSS请求
func GrafanaWebSocketProxy(c *gin.Context) {
	// 将客户端连接升级为WebSocket
	clientWs, err := grafanaUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		common.SysError("Failed to upgrade client connection: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to upgrade to WebSocket",
		})
		return
	}
	defer clientWs.Close()

	// Grafana已配置为在子路径 /api/grafana 下运行，保留完整路径
	originalPath := c.Request.URL.Path

	// 构建目标WebSocket URL，保留完整路径
	targetURL := "wss://grafana.furion-tech.com" + originalPath
	if c.Request.URL.RawQuery != "" {
		targetURL += "?" + c.Request.URL.RawQuery
	}

	common.SysLog("Grafana WebSocket proxy forwarding: " + originalPath + " -> " + targetURL)

	// 复制必要的WebSocket头
	targetHeader := http.Header{}
	for key, values := range c.Request.Header {
		lowerKey := strings.ToLower(key)
		if lowerKey == "host" ||
			lowerKey == "connection" ||
			lowerKey == "upgrade" {
			continue
		}
		if lowerKey == "origin" {
			// 修改 Origin 头为 Grafana 服务器的 Origin
			targetHeader.Set("Origin", "https://grafana.furion-tech.com")
			continue
		}
		// 保留WebSocket相关头部，特别是 Authorization 等
		for _, value := range values {
			targetHeader.Add(key, value)
		}
	}

	// 连接到目标WebSocket服务器
	targetWs, _, err := websocket.DefaultDialer.Dial(targetURL, targetHeader)
	if err != nil {
		common.SysError("Failed to connect to target WebSocket: " + err.Error())
		clientWs.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "Failed to connect to Grafana"))
		return
	}
	defer targetWs.Close()

	// 双向转发消息
	done := make(chan bool)

	// 从客户端到目标服务器
	go func() {
		defer func() { done <- true }()
		for {
			messageType, message, err := clientWs.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					common.SysError("Error reading from client: " + err.Error())
				}
				return
			}
			if err := targetWs.WriteMessage(messageType, message); err != nil {
				common.SysError("Error writing to target: " + err.Error())
				return
			}
		}
	}()

	// 从目标服务器到客户端
	go func() {
		defer func() { done <- true }()
		for {
			messageType, message, err := targetWs.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					common.SysError("Error reading from target: " + err.Error())
				}
				return
			}
			if err := clientWs.WriteMessage(messageType, message); err != nil {
				common.SysError("Error writing to client: " + err.Error())
				return
			}
		}
	}()

	// 等待任一方向关闭
	<-done
}
