package controller

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"one-api/common"
	"one-api/model"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// createProxyHTTPClient 创建使用代理的 HTTP 客户端
func createProxyHTTPClient() *http.Client {
	// 从环境变量读取代理地址，默认值为 http://43.153.3.247:8118
	proxyAddr := common.GetEnvOrDefaultString("GOOGLE_API_PROXY_HTTPS_PROXY", "http://43.153.3.247:8118")
	if proxyAddr == "" {
		proxyAddr = "http://43.153.3.247:8118"
	}

	proxyURL, err := url.Parse(proxyAddr)
	if err != nil {
		common.SysError(fmt.Sprintf("Failed to parse proxy URL %s: %v, using direct connection", proxyAddr, err))
		return &http.Client{
			Timeout: 300 * time.Second,
		}
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
	}
	return &http.Client{
		Transport: transport,
		Timeout:   300 * time.Second, // 设置超时时间
	}
}

// GoogleStorageErrorDetail Google Storage API 错误详情
type GoogleStorageErrorDetail struct {
	Message string `json:"message"`
	Domain  string `json:"domain"`
	Reason  string `json:"reason"`
}

// GoogleStorageError Google Storage API 错误结构
type GoogleStorageError struct {
	Code    int                        `json:"code"`
	Message string                     `json:"message"`
	Errors  []GoogleStorageErrorDetail `json:"errors"`
}

// GoogleStorageErrorResponse Google Storage API 错误响应
type GoogleStorageErrorResponse struct {
	Error GoogleStorageError `json:"error"`
}

// sendGoogleStorageError 发送 Google Storage API 格式的错误响应
func sendGoogleStorageError(c *gin.Context, statusCode int, message, reason string) {
	errorResponse := GoogleStorageErrorResponse{
		Error: GoogleStorageError{
			Code:    statusCode,
			Message: message,
			Errors: []GoogleStorageErrorDetail{
				{
					Message: message,
					Domain:  "global",
					Reason:  reason,
				},
			},
		},
	}
	c.JSON(statusCode, errorResponse)
	c.Abort()
}

// extractPathPrefixFromGcsUri 从 GCS URI 中提取路径前缀
// 例如：gs://batch-gemini-1/yusi/103635414441993822399/dest/result-batch_requests-5
// 返回：yusi/103635414441993822399
func extractPathPrefixFromGcsUri(gcsUri string) string {
	// 去掉 gs:// 前缀（如果存在）
	gcsUri = strings.TrimPrefix(gcsUri, "gs://")
	// 找到第一个 / 后的路径部分
	parts := strings.SplitN(gcsUri, "/", 2)
	if len(parts) < 2 {
		return ""
	}
	path := parts[1]
	// 提取前两级路径（project_id/client_id）
	pathParts := strings.Split(path, "/")
	if len(pathParts) >= 2 {
		return pathParts[0] + "/" + pathParts[1]
	}
	return ""
}

// extractNameFromMultipartBody 从 multipart body 中提取 name 字段
// 例如：multipart body 的第一部分是 JSON，包含 {"name": "yusi/103635414441993822399/batch_requests-5.jsonl", ...}
// 返回：yusi/103635414441993822399/batch_requests-5.jsonl
func extractNameFromMultipartBody(body []byte, contentType string) (string, error) {
	// 提取 boundary
	boundary := ""
	parts := strings.Split(contentType, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "boundary=") {
			boundary = strings.Trim(part[9:], "\"")
			break
		}
	}
	if boundary == "" {
		return "", fmt.Errorf("no boundary found in Content-Type")
	}

	// 使用 multipart.Reader 解析 body
	reader := multipart.NewReader(bytes.NewReader(body), boundary)

	// 读取第一个 part（应该是 JSON 部分）
	part, err := reader.NextPart()
	if err != nil {
		return "", fmt.Errorf("failed to read first part: %w", err)
	}
	defer part.Close()

	// 读取 part 的内容
	partBody, err := io.ReadAll(part)
	if err != nil {
		return "", fmt.Errorf("failed to read part body: %w", err)
	}

	// 解析 JSON
	var jsonData map[string]interface{}
	if err := json.Unmarshal(partBody, &jsonData); err != nil {
		return "", fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	// 提取 name 字段
	if name, ok := jsonData["name"].(string); ok {
		return name, nil
	}

	return "", fmt.Errorf("name field not found in JSON")
}

// extractPathPrefixFromName 从 name 字段中提取路径前缀
// 例如：yusi/103635414441993822399/batch_requests-5.jsonl
// 返回：yusi/103635414441993822399
func extractPathPrefixFromName(name string) string {
	// 提取前两级路径（project_id/client_id）
	pathParts := strings.Split(name, "/")
	if len(pathParts) >= 2 {
		return pathParts[0] + "/" + pathParts[1]
	}
	return ""
}

// extractPathPrefixFromDownloadUrl 从 download URL 路径中提取路径前缀
// 例如：/google/download/storage/v1/b/batch-gemini-1/o/yusi/103635414441993822399/dest/result-batch_requests-5/...
// 返回：yusi/103635414441993822399
func extractPathPrefixFromDownloadUrl(path string) string {
	// 路径格式：/google/download/storage/v1/b/{bucket}/o/{object_path}
	// 需要找到 /o/ 后面的部分，然后提取前两级路径
	oIndex := strings.Index(path, "/o/")
	if oIndex == -1 {
		return ""
	}

	// 获取 /o/ 后面的 object_path
	objectPath := path[oIndex+3:]
	if objectPath == "" {
		return ""
	}

	// 提取前两级路径（project_id/client_id）
	pathParts := strings.Split(objectPath, "/")
	if len(pathParts) >= 2 {
		return pathParts[0] + "/" + pathParts[1]
	}
	return ""
}

// parseBatchJobInfoFromAPIResponse 从 Google API 响应中解析 BatchJobInfo
func parseBatchJobInfoFromAPIResponse(apiResponse map[string]interface{}) *model.BatchJobInfo {
	jobInfo := &model.BatchJobInfo{}

	// 解析基本字段（支持驼峰和小写下划线两种格式）
	if name, ok := apiResponse["name"].(string); ok {
		jobInfo.Name = name
	}
	// 尝试 displayName 或 display_name
	if displayName, ok := apiResponse["displayName"].(string); ok {
		jobInfo.DisplayName = displayName
	} else if displayName, ok := apiResponse["display_name"].(string); ok {
		jobInfo.DisplayName = displayName
	}
	if state, ok := apiResponse["state"].(string); ok {
		jobInfo.State = model.JobState(state)
	}
	if modelPath, ok := apiResponse["model"].(string); ok {
		jobInfo.ModelPath = modelPath
	}

	// 解析错误信息
	if errorVal, ok := apiResponse["error"]; ok && errorVal != nil {
		if errorStr, ok := errorVal.(string); ok {
			jobInfo.Error = &errorStr
		}
	}

	// 解析时间字段（Google API 返回 RFC3339 格式）
	parseTime := func(timeStr string) *time.Time {
		if timeStr == "" {
			return nil
		}
		t, err := time.Parse(time.RFC3339, timeStr)
		if err != nil {
			return nil
		}
		return &t
	}

	// 解析时间字段（支持驼峰和小写下划线两种格式）
	if createTime, ok := apiResponse["createTime"].(string); ok {
		if t := parseTime(createTime); t != nil {
			jobInfo.CreateTime = *t
		}
	} else if createTime, ok := apiResponse["create_time"].(string); ok {
		if t := parseTime(createTime); t != nil {
			jobInfo.CreateTime = *t
		}
	}
	if startTime, ok := apiResponse["startTime"].(string); ok {
		jobInfo.StartTime = parseTime(startTime)
	} else if startTime, ok := apiResponse["start_time"].(string); ok {
		jobInfo.StartTime = parseTime(startTime)
	}
	if endTime, ok := apiResponse["endTime"].(string); ok {
		jobInfo.EndTime = parseTime(endTime)
	} else if endTime, ok := apiResponse["end_time"].(string); ok {
		jobInfo.EndTime = parseTime(endTime)
	}
	if updateTime, ok := apiResponse["updateTime"].(string); ok {
		if t := parseTime(updateTime); t != nil {
			jobInfo.UpdateTime = *t
		}
	} else if updateTime, ok := apiResponse["update_time"].(string); ok {
		if t := parseTime(updateTime); t != nil {
			jobInfo.UpdateTime = *t
		}
	}

	// 解析 src 和 dest（需要序列化为 JSON 字符串存储）
	if src, ok := apiResponse["src"].(map[string]interface{}); ok {
		if srcJSON, err := json.Marshal(src); err == nil {
			var batchJobSource model.BatchJobSource
			if err := json.Unmarshal(srcJSON, &batchJobSource); err == nil {
				jobInfo.Src = batchJobSource
			}
		}
	}
	if dest, ok := apiResponse["dest"].(map[string]interface{}); ok {
		if destJSON, err := json.Marshal(dest); err == nil {
			var batchJobDest model.BatchJobDestination
			if err := json.Unmarshal(destJSON, &batchJobDest); err == nil {
				jobInfo.Dest = batchJobDest
			}
		}
	}

	return jobInfo
}

// GoogleStorageProxy 代理 Google Storage 请求
func GoogleStorageProxy(c *gin.Context) {
	// 获取原始路径，例如：/google/storage/bucket/file.txt
	originalPath := c.Request.URL.Path

	// 去除 /google 前缀，得到：/storage/bucket/file.txt 或 /download/storage/... 或 /upload/storage/...
	targetPath := strings.TrimPrefix(originalPath, "/google")
	if targetPath == "" {
		targetPath = "/"
	}

	// 构建目标 URL
	targetURL := "https://storage.googleapis.com" + targetPath
	if c.Request.URL.RawQuery != "" {
		targetURL += "?" + c.Request.URL.RawQuery
	}

	common.LogInfo(c.Request.Context(), "Google Storage proxy forwarding: "+c.Request.Method+" "+originalPath+" -> "+targetURL)

	// 根据路径类型进行不同的验证
	// 1. download 接口：
	//    - POST 方法：验证 prefix 参数
	//    - GET 方法：验证 URL 路径中的 project_id/client_id
	// 2. upload 接口：验证请求 body 中的 name 字段
	if strings.HasPrefix(originalPath, "/google/download/storage/v1/b") {
		// 获取 OAuth 对象（用于验证）
		oauthValue, exists := c.Get("oauth")
		if !exists || oauthValue == nil {
			// 如果没有 oauth 对象，尝试从 Authorization header 中查询
			authHeader := c.Request.Header.Get("Authorization")
			if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
				clientToken := strings.TrimPrefix(authHeader, "Bearer ")
				clientToken = strings.TrimSpace(clientToken)
				oauthFromDB, err := model.GetOAuthByClientAccessToken(clientToken)
				if err == nil && oauthFromDB != nil {
					oauthValue = oauthFromDB
				}
			}
		}

		if oauthValue != nil {
			oauth, ok := oauthValue.(*model.OAuth)
			if ok && oauth != nil {
				switch c.Request.Method {
				case "POST":
					// POST 方法：验证 prefix 参数
					prefixParam := c.Query("prefix")
					if prefixParam != "" {
						// URL 解码 prefix 参数
						decodedPrefix, err := url.QueryUnescape(prefixParam)
						if err != nil {
							decodedPrefix = prefixParam // 如果解码失败，使用原始值
						}

						if !strings.HasPrefix(decodedPrefix, oauth.ProjectId+"/"+oauth.ClientId) {
							common.LogError(c.Request.Context(), fmt.Sprintf("Prefix 权限验证失败: prefix=%s, ProjectId=%s, ClientId=%s", decodedPrefix, oauth.ProjectId, oauth.ClientId))
							sendGoogleStorageError(c, http.StatusForbidden, "路径没有权限", "permissionDenied")
							return
						}
					} else {
						common.LogError(c.Request.Context(), "POST 方法需要 prefix 参数")
						sendGoogleStorageError(c, http.StatusBadRequest, "prefix 参数为空", "invalidArgument")
						return
					}
				case "GET":
					// GET 方法：从 URL 路径中提取并验证 project_id/client_id
					pathPrefix := extractPathPrefixFromDownloadUrl(originalPath)
					if pathPrefix == "" {
						common.LogError(c.Request.Context(), fmt.Sprintf("无法从 URL 路径中提取路径前缀: path=%s", originalPath))
						sendGoogleStorageError(c, http.StatusBadRequest, "URL 路径格式不正确", "invalidArgument")
						return
					}

					expectedPrefix := oauth.ProjectId + "/" + oauth.ClientId
					if pathPrefix != expectedPrefix && !strings.HasPrefix(pathPrefix, expectedPrefix+"/") {
						common.LogError(c.Request.Context(), fmt.Sprintf("URL 路径前缀验证失败: pathPrefix=%s, expectedPrefix=%s, path=%s", pathPrefix, expectedPrefix, originalPath))
						sendGoogleStorageError(c, http.StatusForbidden, fmt.Sprintf("路径没有权限，URL 路径前缀必须是 %s", expectedPrefix), "permissionDenied")
						return
					}
				default:
					// 其他 HTTP 方法暂不支持
					common.LogError(c.Request.Context(), fmt.Sprintf("不支持的 HTTP 方法: %s", c.Request.Method))
					sendGoogleStorageError(c, http.StatusMethodNotAllowed, fmt.Sprintf("不支持的 HTTP 方法: %s", c.Request.Method), "methodNotAllowed")
					return
				}
			}
		}
	} else if strings.HasPrefix(originalPath, "/google/upload/storage/v1/b") {
		// 验证请求 body 中的 name 字段
		contentType := c.Request.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "multipart/") {
			common.LogError(c.Request.Context(), fmt.Sprintf("Upload 接口需要 multipart Content-Type，当前为: %s", contentType))
			sendGoogleStorageError(c, http.StatusBadRequest, "Upload 接口需要 multipart Content-Type", "invalidArgument")
			return
		}

		// 读取请求 body
		bodyBytes, err := common.GetRequestBody(c)
		if err != nil {
			common.LogError(c.Request.Context(), "Failed to read request body: "+err.Error())
			sendGoogleStorageError(c, http.StatusBadRequest, "无法读取请求体", "invalidArgument")
			return
		}

		// 从 multipart body 中提取 name 字段
		name, err := extractNameFromMultipartBody(bodyBytes, contentType)
		if err != nil {
			common.LogError(c.Request.Context(), "Failed to extract name from multipart body: "+err.Error())
			sendGoogleStorageError(c, http.StatusBadRequest, "无法从请求体中提取 name 字段", "invalidArgument")
			return
		}

		// 从 name 中提取路径前缀
		namePrefix := extractPathPrefixFromName(name)
		if namePrefix == "" {
			common.LogError(c.Request.Context(), fmt.Sprintf("无法从 name 字段中提取路径前缀: name=%s", name))
			sendGoogleStorageError(c, http.StatusBadRequest, "name 字段格式不正确", "invalidArgument")
			return
		}

		// 从 context 中获取 oauth 对象（由中间件设置）
		oauthValue, exists := c.Get("oauth")
		if !exists || oauthValue == nil {
			// 如果没有 oauth 对象，尝试从 Authorization header 中查询
			authHeader := c.Request.Header.Get("Authorization")
			if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
				clientToken := strings.TrimPrefix(authHeader, "Bearer ")
				clientToken = strings.TrimSpace(clientToken)
				oauthFromDB, err := model.GetOAuthByClientAccessToken(clientToken)
				if err == nil && oauthFromDB != nil {
					oauthValue = oauthFromDB
				}
			}
		}

		if oauthValue != nil {
			oauth, ok := oauthValue.(*model.OAuth)
			if ok && oauth != nil {
				expectedPrefix := oauth.ProjectId + "/" + oauth.ClientId
				if namePrefix != expectedPrefix && !strings.HasPrefix(namePrefix, expectedPrefix+"/") {
					common.LogError(c.Request.Context(), fmt.Sprintf("Name 路径前缀验证失败: namePrefix=%s, expectedPrefix=%s, name=%s", namePrefix, expectedPrefix, name))
					sendGoogleStorageError(c, http.StatusForbidden, fmt.Sprintf("路径没有权限，name 字段的路径前缀必须是 %s", expectedPrefix), "permissionDenied")
					return
				}
			}
		}
	}

	// 处理 Authorization header：从 client_access_token 替换为 access_token
	authHeader := c.Request.Header.Get("Authorization")
	var finalAuthToken string
	if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
		clientToken := strings.TrimPrefix(authHeader, "Bearer ")
		clientToken = strings.TrimSpace(clientToken)

		// 查询 OAuth 表，根据 client_access_token 查找对应的 access_token
		oauth, err := model.GetOAuthByClientAccessToken(clientToken)
		if err == nil && oauth != nil && oauth.AccessToken != "" {
			// 找到匹配的记录，使用 access_token 替换
			finalAuthToken = "Bearer " + oauth.AccessToken
			common.LogInfo(c.Request.Context(), fmt.Sprintf("Token replaced: client_access_token -> access_token for OAuth ID: %d", oauth.ID))
		} else if err != nil && err != gorm.ErrRecordNotFound {
			common.LogError(c.Request.Context(), "Failed to query OAuth by client_access_token: "+err.Error())
		}
		// 如果没找到或查询失败，使用原始 token
		if finalAuthToken == "" {
			finalAuthToken = authHeader
		}
	}

	// 对于 download 接口，在创建请求时对 URL 进行编码
	// 解析 URL
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		common.LogError(c.Request.Context(), "Failed to parse target URL: "+err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to parse target URL",
		})
		return
	}

	if strings.HasPrefix(originalPath, "/google/download/storage/v1/b") {
		// 对于 Google Storage API，需要特殊处理：
		// /download/storage/v1/b/{bucket}/o/{object} 格式
		// /o/ 之前的部分：每个段分别编码，保留 / 分隔符
		// /o/ 之后的部分（object name）：整体编码，包括 / 也要编码为 %2F
		common.LogInfo(c.Request.Context(), fmt.Sprintf("Original parsed path: %s", parsedURL.Path))

		// 查找 /o/ 的位置
		oIndex := strings.Index(parsedURL.Path, "/o/")
		var encodedPath string

		if oIndex != -1 {
			// 分割成两部分：/o/ 之前和之后
			beforeO := parsedURL.Path[:oIndex+2] // 包括 "/o"
			afterO := parsedURL.Path[oIndex+3:]  // /o/ 之后的 object name

			// 对 /o/ 之前的部分，每个段分别编码
			pathParts := strings.Split(beforeO, "/")
			encodedParts := make([]string, 0, len(pathParts))
			for _, part := range pathParts {
				if part != "" {
					encoded := url.QueryEscape(part)
					encoded = strings.ReplaceAll(encoded, "+", "%20")
					encodedParts = append(encodedParts, encoded)
				}
			}
			encodedBeforeO := "/" + strings.Join(encodedParts, "/")

			// 对 /o/ 之后的 object name 整体编码（包括 / 也编码）
			encodedAfterO := url.QueryEscape(afterO)
			encodedAfterO = strings.ReplaceAll(encodedAfterO, "+", "%20")

			encodedPath = encodedBeforeO + "/" + encodedAfterO
			common.LogInfo(c.Request.Context(), fmt.Sprintf("Object name: %s -> %s", afterO, encodedAfterO))
		} else {
			// 如果没有 /o/，按原逻辑处理
			pathParts := strings.Split(parsedURL.Path, "/")
			encodedParts := make([]string, 0, len(pathParts))
			for _, part := range pathParts {
				if part != "" {
					encoded := url.QueryEscape(part)
					encoded = strings.ReplaceAll(encoded, "+", "%20")
					encodedParts = append(encodedParts, encoded)
				}
			}
			encodedPath = "/" + strings.Join(encodedParts, "/")
		}

		common.LogInfo(c.Request.Context(), fmt.Sprintf("Encoded path: %s", encodedPath))
		// 设置 RawPath 来保留编码后的路径
		// 同时需要设置 Path 为解码后的路径（虽然可能包含特殊字符）
		// Go 的 HTTP 客户端会优先使用 RawPath（如果已设置）
		parsedURL.RawPath = encodedPath
		// 解码 encodedPath 作为 Path（用于 URL 构建）
		decodedPath, err := url.PathUnescape(encodedPath)
		if err != nil {
			// 如果解码失败，使用原始路径
			decodedPath = parsedURL.Path
		}
		parsedURL.Path = decodedPath
	}

	// 打印最终 URL 以确认编码
	finalTargetURL := parsedURL.String()
	common.LogInfo(c.Request.Context(), fmt.Sprintf("Final target URL for request: %s", finalTargetURL))

	// 直接使用 url.URL 结构体创建请求，避免 http.NewRequest 自动解码路径
	req := &http.Request{
		Method: c.Request.Method,
		URL:    parsedURL,
		Body:   c.Request.Body,
		Header: make(http.Header),
	}

	// 复制请求头
	for key, values := range c.Request.Header {
		lowerKey := strings.ToLower(key)
		// 跳过一些会导致问题的头部
		if lowerKey == "host" {
			continue
		}
		if lowerKey == "content-length" {
			continue
		}
		// 如果是 Authorization header，使用替换后的 token
		if lowerKey == "authorization" {
			if finalAuthToken != "" {
				req.Header.Set("Authorization", finalAuthToken)
			}
			continue
		}
		// 保留所有其他头部
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// 设置 Host 头
	req.Host = "storage.googleapis.com"

	// 创建带代理的 HTTP 客户端并发送请求
	client := createProxyHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		common.LogError(c.Request.Context(), "Failed to proxy request: "+err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to proxy request to Google Storage",
		})
		return
	}
	defer resp.Body.Close()

	// 复制响应头
	for key, values := range resp.Header {
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}

	// 设置状态码
	c.Writer.WriteHeader(resp.StatusCode)

	// 复制响应体
	_, err = io.Copy(c.Writer, resp.Body)
	if err != nil {
		common.LogError(c.Request.Context(), "Failed to copy response body: "+err.Error())
		return
	}

	common.LogInfo(c.Request.Context(), "Google Storage proxy response: "+c.Request.Method+" "+originalPath+" -> Status: "+resp.Status)
}

// GoogleV1Beta1Proxy 代理 Google Vertex AI v1beta1 API 请求
func GoogleV1Beta1Proxy(c *gin.Context) {
	// 获取原始路径，例如：/google/v1beta1/projects/xxx/locations/xxx/models/xxx
	originalPath := c.Request.URL.Path

	// 去除 /google 前缀，得到：/v1beta1/projects/xxx/locations/xxx/models/xxx
	targetPath := strings.TrimPrefix(originalPath, "/google")
	if targetPath == "" {
		targetPath = "/"
	}

	// 构建目标 URL
	targetURL := "https://us-central1-aiplatform.googleapis.com" + targetPath
	if c.Request.URL.RawQuery != "" {
		targetURL += "?" + c.Request.URL.RawQuery
	}

	common.LogInfo(c.Request.Context(), "Google Vertex AI v1beta1 proxy forwarding: "+c.Request.Method+" "+originalPath+" -> "+targetURL)

	// 验证请求 body 中的 outputUriPrefix（仅 POST 请求）
	if c.Request.Method == "POST" {
		// 读取请求 body
		bodyBytes, err := common.GetRequestBody(c)
		if err == nil && len(bodyBytes) > 0 {
			var requestBody map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &requestBody); err == nil {
				// 解析 outputConfig.gcsDestination.outputUriPrefix
				if outputConfig, ok := requestBody["outputConfig"].(map[string]interface{}); ok {
					if gcsDestination, ok := outputConfig["gcsDestination"].(map[string]interface{}); ok {
						if outputUriPrefix, ok := gcsDestination["outputUriPrefix"].(string); ok && outputUriPrefix != "" {
							// 从 GCS URI 中提取路径前缀（去掉 gs://bucket-name/ 部分）
							// 例如：gs://batch-gemini-1/yusi/103635414441993822399/dest/result-batch_requests-5
							// 提取：yusi/103635414441993822399
							uriPrefix := extractPathPrefixFromGcsUri(outputUriPrefix)
							if uriPrefix != "" {
								// 从 context 中获取 oauth 对象
								oauthValue, exists := c.Get("oauth")
								if !exists || oauthValue == nil {
									// 如果没有 oauth 对象，尝试从 Authorization header 中查询
									authHeader := c.Request.Header.Get("Authorization")
									if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
										clientToken := strings.TrimPrefix(authHeader, "Bearer ")
										clientToken = strings.TrimSpace(clientToken)
										oauthFromDB, err := model.GetOAuthByClientAccessToken(clientToken)
										if err == nil && oauthFromDB != nil {
											oauthValue = oauthFromDB
										}
									}
								}

								if oauthValue != nil {
									oauth, ok := oauthValue.(*model.OAuth)
									if ok && oauth != nil && oauth.ProjectId != "" && oauth.ClientId != "" {
										// 构建期望的路径前缀：project_id/client_id（使用 oauth 表的字段）
										expectedPrefix := oauth.ProjectId + "/" + oauth.ClientId
										// 检查路径前缀是否匹配
										if uriPrefix != expectedPrefix && !strings.HasPrefix(uriPrefix, expectedPrefix+"/") {
											common.LogError(c.Request.Context(), fmt.Sprintf("outputUriPrefix 路径前缀验证失败: uriPrefix=%s, expectedPrefix=%s", uriPrefix, expectedPrefix))
											sendGoogleStorageError(c, http.StatusBadRequest, fmt.Sprintf("outputUriPrefix 路径前缀必须是 %s", expectedPrefix), "invalidArgument")
											return
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// 替换路径中的 project_id
	// 路径格式：/google/v1beta1/projects/{project_id}/locations/...
	pathParts := strings.Split(strings.TrimPrefix(originalPath, "/google/v1beta1/projects/"), "/")
	if len(pathParts) > 0 && pathParts[0] != "" {
		pathProjectId := pathParts[0]

		// 从 context 中获取 oauth 对象（由中间件设置）
		oauthValue, exists := c.Get("oauth")
		if !exists || oauthValue == nil {
			// 如果没有 oauth 对象，尝试从 Authorization header 中查询
			authHeader := c.Request.Header.Get("Authorization")
			if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
				clientToken := strings.TrimPrefix(authHeader, "Bearer ")
				clientToken = strings.TrimSpace(clientToken)
				oauthFromDB, err := model.GetOAuthByClientAccessToken(clientToken)
				if err == nil && oauthFromDB != nil {
					oauthValue = oauthFromDB
				}
			}
		}

		if oauthValue != nil {
			oauth, ok := oauthValue.(*model.OAuth)
			if ok && oauth != nil && oauth.ChannelId > 0 {
				// 查询 channel
				channel, err := model.GetChannelById(oauth.ChannelId, true)
				if err == nil && channel != nil && channel.Key != "" {
					// 解析 channel.Key (JSON) 获取 project_id
					var serviceAccountJSON map[string]interface{}
					if err := json.Unmarshal([]byte(channel.Key), &serviceAccountJSON); err == nil {
						if channelProjectId, ok := serviceAccountJSON["project_id"].(string); ok && channelProjectId != "" {
							// 如果路径中的 project_id 与 channel 中的 project_id 不一致，替换为正确的 project_id
							if pathProjectId != channelProjectId {
								common.LogInfo(c.Request.Context(), fmt.Sprintf("替换路径中的 project_id: %s -> %s", pathProjectId, channelProjectId))
								// 替换路径中的 project_id
								originalPath = strings.Replace(originalPath, "/projects/"+pathProjectId+"/", "/projects/"+channelProjectId+"/", 1)
								// 重新计算 targetPath
								targetPath = strings.TrimPrefix(originalPath, "/google")
								if targetPath == "" {
									targetPath = "/"
								}
								// 重新构建目标 URL
								targetURL = "https://us-central1-aiplatform.googleapis.com" + targetPath
								if c.Request.URL.RawQuery != "" {
									targetURL += "?" + c.Request.URL.RawQuery
								}
							}
						}
					}
				}
			}
		}
	}

	// 处理 Authorization header：从 client_access_token 替换为 access_token
	authHeader := c.Request.Header.Get("Authorization")
	var finalAuthToken string
	if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
		clientToken := strings.TrimPrefix(authHeader, "Bearer ")
		clientToken = strings.TrimSpace(clientToken)

		// 查询 OAuth 表，根据 client_access_token 查找对应的 access_token
		oauth, err := model.GetOAuthByClientAccessToken(clientToken)
		if err == nil && oauth != nil && oauth.AccessToken != "" {
			// 找到匹配的记录，使用 access_token 替换
			finalAuthToken = "Bearer " + oauth.AccessToken
			common.LogInfo(c.Request.Context(), fmt.Sprintf("Token replaced: client_access_token -> access_token for OAuth ID: %d", oauth.ID))
		} else if err != nil && err != gorm.ErrRecordNotFound {
			common.LogError(c.Request.Context(), "Failed to query OAuth by client_access_token: "+err.Error())
		}
		// 如果没找到或查询失败，使用原始 token
		if finalAuthToken == "" {
			finalAuthToken = authHeader
		}
	}

	// 创建新的请求
	req, err := http.NewRequest(c.Request.Method, targetURL, c.Request.Body)
	if err != nil {
		common.LogError(c.Request.Context(), "Failed to create proxy request: "+err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create proxy request",
		})
		return
	}

	// 复制请求头
	for key, values := range c.Request.Header {
		lowerKey := strings.ToLower(key)
		// 跳过一些会导致问题的头部
		if lowerKey == "host" {
			continue
		}
		if lowerKey == "content-length" {
			continue
		}
		// 如果是 Authorization header，使用替换后的 token
		if lowerKey == "authorization" {
			if finalAuthToken != "" {
				req.Header.Set("Authorization", finalAuthToken)
			}
			continue
		}
		// 保留所有其他头部
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// 设置 Host 头
	req.Host = "us-central1-aiplatform.googleapis.com"

	// 创建带代理的 HTTP 客户端并发送请求
	client := createProxyHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		common.LogError(c.Request.Context(), "Failed to proxy request: "+err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to proxy request to Google Vertex AI",
		})
		return
	}
	defer resp.Body.Close()

	// 读取原始响应体（保持压缩状态，用于返回给客户端）
	originalResponseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		common.LogError(c.Request.Context(), "Failed to read response body: "+err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to read response body",
		})
		return
	}

	// 复制响应头（保持原始响应头，包括 Content-Encoding 和 Content-Length）
	for key, values := range resp.Header {
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}

	// 设置状态码
	c.Writer.WriteHeader(resp.StatusCode)

	// 复制原始响应体到客户端（保持压缩状态）
	_, err = c.Writer.Write(originalResponseBody)
	if err != nil {
		common.LogError(c.Request.Context(), "Failed to write response body: "+err.Error())
		return
	}

	// 如果响应成功（200-299），解析并保存到数据库
	// 需要解压缩用于解析 JSON
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var decompressedBody []byte
		contentEncoding := resp.Header.Get("Content-Encoding")

		if contentEncoding == "gzip" {
			// 解压缩用于解析 JSON
			gzipReader, err := gzip.NewReader(bytes.NewReader(originalResponseBody))
			if err != nil {
				common.LogError(c.Request.Context(), "Failed to create gzip reader for parsing: "+err.Error())
			} else {
				defer gzipReader.Close()
				decompressedBody, err = io.ReadAll(gzipReader)
				if err != nil {
					common.LogError(c.Request.Context(), "Failed to decompress response body for parsing: "+err.Error())
				}
			}
		} else {
			// 非压缩响应，直接使用原始响应体
			decompressedBody = originalResponseBody
		}

		// 如果有解压缩后的数据，解析并保存
		if len(decompressedBody) > 0 {
			var apiResponse map[string]interface{}
			if err := json.Unmarshal(decompressedBody, &apiResponse); err == nil {
				jobInfo := parseBatchJobInfoFromAPIResponse(apiResponse)
				if jobInfo != nil && jobInfo.Name != "" {
					// 获取 OAuth ID（从 context 中获取 oauth 对象）
					oauthValue, exists := c.Get("oauth")
					if !exists || oauthValue == nil {
						// 如果没有 oauth 对象，尝试从 Authorization header 中查询
						authHeader := c.Request.Header.Get("Authorization")
						if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
							clientToken := strings.TrimPrefix(authHeader, "Bearer ")
							clientToken = strings.TrimSpace(clientToken)
							oauthFromDB, err := model.GetOAuthByClientAccessToken(clientToken)
							if err == nil && oauthFromDB != nil {
								oauthValue = oauthFromDB
							}
						}
					}
					if oauthValue != nil {
						if oauth, ok := oauthValue.(*model.OAuth); ok && oauth != nil {
							jobInfo.OAuthId = uint(oauth.ID)
						}
					}

					// 尝试根据 Name 或 DisplayName 查找现有记录
					var existingJob model.BatchJobInfo
					err := model.DB.Where("name = ? OR display_name = ?", jobInfo.Name, jobInfo.DisplayName).First(&existingJob).Error
					switch {
					case err == nil:
						// 记录已存在，更新
						jobInfo.ID = existingJob.ID
						jobInfo.CreatedAt = existingJob.CreatedAt
						if err := model.DB.Save(jobInfo).Error; err != nil {
							common.LogError(c.Request.Context(), fmt.Sprintf("Failed to update BatchJobInfo: %v", err))
						} else {
							common.LogInfo(c.Request.Context(), fmt.Sprintf("Updated BatchJobInfo: %s, OAuthId: %d", jobInfo.Name, jobInfo.OAuthId))
						}
					case errors.Is(err, gorm.ErrRecordNotFound):
						// 记录不存在，创建新记录
						if err := model.DB.Create(jobInfo).Error; err != nil {
							common.LogError(c.Request.Context(), fmt.Sprintf("Failed to create BatchJobInfo: %v", err))
						} else {
							common.LogInfo(c.Request.Context(), fmt.Sprintf("Created BatchJobInfo: %s, OAuthId: %d", jobInfo.Name, jobInfo.OAuthId))
						}
					default:
						common.LogError(c.Request.Context(), fmt.Sprintf("Failed to query BatchJobInfo: %v", err))
					}
				} else {
					common.LogError(c.Request.Context(), fmt.Sprintf("Failed to parse BatchJobInfo: jobInfo is nil or Name is empty, decompressedBody length: %d", len(decompressedBody)))
				}
			} else {
				common.LogError(c.Request.Context(), fmt.Sprintf("Failed to unmarshal response body: %v, decompressedBody length: %d", err, len(decompressedBody)))
			}
		}
	}

	common.LogInfo(c.Request.Context(), "Google Vertex AI v1beta1 proxy response: "+c.Request.Method+" "+originalPath+" -> Status: "+resp.Status)
}
