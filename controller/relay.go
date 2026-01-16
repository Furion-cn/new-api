package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/metrics"
	"one-api/middleware"
	"one-api/model"
	"one-api/relay"
	relaycommon "one-api/relay/common"
	relayconstant "one-api/relay/constant"
	"one-api/relay/helper"
	"one-api/service"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func relayInfoHandler(c *gin.Context, relayMode int) (*relaycommon.RelayInfo, interface{}, string, *dto.OpenAIErrorWithStatusCode) {
	switch relayMode {
	case relayconstant.RelayModeImagesGenerations:
		relayInfo, request, err := relay.ImageInfo(c)
		if err != nil {
			return nil, nil, "", err
		}
		return relayInfo, request, request.Model, nil
	case relayconstant.RelayModeAudioSpeech:
		fallthrough
	case relayconstant.RelayModeAudioTranslation:
		fallthrough
	case relayconstant.RelayModeAudioTranscription:
		relayInfo, request, err := relay.AudioInfo(c)
		if err != nil {
			return nil, nil, "", err
		}
		return relayInfo, request, request.Model, nil
	case relayconstant.RelayModeRerank:
		relayInfo, request, err := relay.EmbeddingInfo(c)
		if err != nil {
			return nil, nil, "", err
		}
		return relayInfo, request, request.Model, nil
	case relayconstant.RelayModeEmbeddings:
		relayInfo, request, err := relay.EmbeddingInfo(c)
		if err != nil {
			return nil, nil, "", err
		}
		return relayInfo, request, request.Model, nil
	case relayconstant.RelayModeProxy:
		relayInfo, request, model, err := relay.ProxyInfo(c)
		if err != nil {
			return nil, nil, "", err
		}
		return relayInfo, request, model, nil
	default:
		relayInfo, request, err := relay.TextInfo(c)
		if err != nil {
			return nil, nil, "", err
		}
		return relayInfo, request, request.Model, nil
	}
}

func relayExecuteHandler(c *gin.Context, relayMode int, relayInfo *relaycommon.RelayInfo, request interface{}) *dto.OpenAIErrorWithStatusCode {
	var err *dto.OpenAIErrorWithStatusCode
	switch relayMode {
	case relayconstant.RelayModeImagesGenerations:
		imageRequest, ok := request.(*dto.ImageRequest)
		if !ok {
			return service.OpenAIErrorWrapperLocal(fmt.Errorf("failed assert request: %d", relayMode), "invalid_request_type", http.StatusInternalServerError)
		}
		err = relay.ImageHelper(c, relayInfo, imageRequest)
	case relayconstant.RelayModeAudioSpeech:
		fallthrough
	case relayconstant.RelayModeAudioTranslation:
		fallthrough
	case relayconstant.RelayModeAudioTranscription:
		audioRequest, ok := request.(*dto.AudioRequest)
		if !ok {
			return service.OpenAIErrorWrapperLocal(fmt.Errorf("failed assert request: %d", relayMode), "invalid_request_type", http.StatusInternalServerError)
		}
		err = relay.AudioHelper(c, relayInfo, audioRequest)
	case relayconstant.RelayModeRerank:
		rerankRequest, ok := request.(*dto.RerankRequest)
		if !ok {
			return service.OpenAIErrorWrapperLocal(fmt.Errorf("failed assert request: %d", relayMode), "invalid_request_type", http.StatusInternalServerError)
		}
		err = relay.RerankHelper(c, relayInfo, rerankRequest)
	case relayconstant.RelayModeEmbeddings:
		embeddingRequest, ok := request.(*dto.EmbeddingRequest)
		if !ok {
			return service.OpenAIErrorWrapperLocal(fmt.Errorf("failed assert request: %d", relayMode), "invalid_request_type", http.StatusInternalServerError)
		}
		err = relay.EmbeddingHelper(c, relayInfo, embeddingRequest)
	case relayconstant.RelayModeProxy:
		err = relay.ProxyHelper(c, relayInfo, request)
	default:
		textRequest, ok := request.(*dto.GeneralOpenAIRequest)
		if !ok {
			return service.OpenAIErrorWrapperLocal(fmt.Errorf("failed assert request: %d", relayMode), "invalid_request_type", http.StatusInternalServerError)
		}
		err = relay.TextHelper(c, relayInfo, textRequest)
	}
	return err
}

func Relay(c *gin.Context) {
	startTime := time.Now()
	relayMode := relayconstant.Path2RelayMode(c.Request.URL.Path)
	requestId := c.GetString(common.RequestIdKey)
	group := c.GetString("group")
	originalModel := c.GetString("original_model")
	tokenKey := c.GetString("token_key")
	tokenName := c.GetString("token_name")
	userId := strconv.Itoa(c.GetInt("id"))
	userName := c.GetString(constant.ContextKeyUserName)
	var openaiErr *dto.OpenAIErrorWithStatusCode
	var err error
	var channel *model.Channel
	var requestModel string
	defer func() {
		if channel == nil {
			channel = &model.Channel{
				Id:   -1,
				Name: "nofallback",
			}
		}
		var code string
		if openaiErr == nil {
			// 成功请求
			code = "200"
		} else {
			// 失败请求
			code = strconv.Itoa(openaiErr.StatusCode)
			// e2e 失败计数
			if strings.Contains(openaiErr.Error.Message, "write: connection timed out") && openaiErr.Error.Code == "copy_response_body_failed" {
				code = "499"
			}
			common.LogInfo(c, fmt.Sprintf("id %s channel: %d,name %s, requestModel: %s, group: %s, tokenKey: %s, tokenName: %s, userId: %s, userName: %s", requestId, channel.Id, channel.Name, requestModel, group, tokenKey, tokenName, userId, userName))
			metrics.IncrementRelayRequestE2EFailedCounter(strconv.Itoa(channel.Id), channel.Name, requestModel, group, code, tokenKey, tokenName, userId, userName, openaiErr.Error.Message, 1)
		}
		// 统计所有请求的耗时（成功和失败）
		metrics.ObserveRelayRequestE2EDuration(strconv.Itoa(channel.Id), channel.Name, requestModel, group, tokenKey, tokenName, userId, userName, code, time.Since(startTime).Seconds())
	}()

	for i := 0; i <= common.RetryTimes; i++ {
		channel, err = getChannel(c, group, originalModel, i)
		if err != nil {
			common.LogError(c, err.Error())
			openaiErr = service.OpenAIErrorWrapperLocal(err, "get_channel_failed", http.StatusInternalServerError)
			break
		}
		var channelSetting map[string]interface{}

		err = json.Unmarshal([]byte(channel.Setting), &channelSetting)
		if err != nil {
			common.LogError(c, fmt.Sprintf("Failed to unmarshal channel setting: %v", err))
		}

		// 设置 channel 信息到上下文
		c.Set("channel", strconv.Itoa(channel.Id))
		c.Set("channel_name", channel.Name)
		common.LogInfo(c, fmt.Sprintf("channelSetting: %+v", channelSetting))
		// 检查passthrough_body，支持布尔值和字符串两种形式
		passthroughBody := false
		if val, exists := channelSetting["passthrough_body"]; exists {
			if boolVal, ok := val.(bool); ok {
				passthroughBody = boolVal
			} else if strVal, ok := val.(string); ok {
				passthroughBody = strVal == "true"
			}
		}
		if passthroughBody {
			common.LogInfo(c, "passthrough_body is true, use proxy")
			c.Set("proxy", true)
			relayMode = relayconstant.RelayModeProxy
		}
		fillRelayRequest(c, channel)
		var (
			relayInfo *relaycommon.RelayInfo
			request   interface{}
		)

		relayInfo, request, requestModel, openaiErr = relayInfoHandler(c, relayMode)
		if i == 0 {
			// e2e 用户请求计数
			metrics.IncrementRelayRequestE2ETotalCounter(strconv.Itoa(channel.Id), channel.Name, requestModel, group, tokenKey, tokenName, userId, userName, 1)
		} else {
			// 重试计数
			channelTag := ""
			if channel.Tag != nil {
				channelTag = *channel.Tag
			}
			metrics.IncrementRelayRetryCounter(strconv.Itoa(channel.Id), channel.Name, channelTag, channel.GetBaseURL(), requestModel, group, userId, userName, 1)
		}
		if openaiErr == nil {
			openaiErr = executeRelayRequest(c, relayMode, relayInfo, request)
			common.LogInfo(c, fmt.Sprintf("openaiErr: %+v", openaiErr))
			if openaiErr == nil {
				common.LogInfo(c, fmt.Sprintf("channel: %d,name %s, requestModel: %s, group: %s, tokenKey: %s, tokenName: %s, userId: %s, userName: %s", channel.Id, channel.Name, requestModel, group, tokenKey, tokenName, userId, userName))
				metrics.IncrementRelayRequestE2ESuccessCounter(strconv.Itoa(channel.Id), channel.Name, requestModel, group, tokenKey, tokenName, userId, userName, 1)

				// 处理 /v1/videos 响应，提取 video_id 并存储到 Redis
				if strings.HasPrefix(c.Request.URL.Path, "/v1/videos") && !strings.Contains(c.Request.URL.Path, "/v1/videos/video_") {
					handleVideoResponse(c, channel.Id)
				}

				return
			}
			if strings.Contains(openaiErr.Error.Message, "No candidates returned") && originalModel == "gemini-2.5-pro" {
				originalModel = "gemini-2.5-pro-youtube"
			}
		}

		go processChannelError(c, channel.Id, channel.Type, channel.Name, channel.GetAutoBan(), openaiErr)

		if !shouldRetry(c, openaiErr, common.RetryTimes-i) {

			break
		}
	}
	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		common.LogInfo(c, retryLogStr)
	}

	if openaiErr != nil {
		if openaiErr.StatusCode == http.StatusTooManyRequests {
			common.LogError(c, fmt.Sprintf("origin 429 error: %s", openaiErr.Error.Message))
			openaiErr.Error.Message = "当前分组上游负载已饱和，请稍后再试"
		}

		// 处理自定义的 NewAPI batch 错误码
		if openaiErr.StatusCode == dto.StatusNewAPIBatchRateLimitExceeded {
			common.LogError(c, fmt.Sprintf("origin %d error: %s", openaiErr.StatusCode, openaiErr.Error.Message))
			openaiErr.Error.Message = "当前服务端限速已满，请稍后再试"
		}
		if openaiErr.StatusCode == dto.StatusNewAPIBatchTimeout {
			common.LogError(c, fmt.Sprintf("origin %d error: %s", openaiErr.StatusCode, openaiErr.Error.Message))
			openaiErr.Error.Message = "未等待到结果，请稍后使用Retry_request_id再次查询"
		}
		if openaiErr.StatusCode == dto.StatusNewAPIBatchInternal {
			common.LogError(c, fmt.Sprintf("origin %d error: %s", openaiErr.StatusCode, openaiErr.Error.Message))
			openaiErr.Error.Message = "服务内部错误，请稍后再试"
		}
		if openaiErr.StatusCode == dto.StatusNewAPIBatchSubmitted {
			common.LogError(c, fmt.Sprintf("origin %d error: %s", openaiErr.StatusCode, openaiErr.Error.Message))
			openaiErr.Error.Message = "批量请求已提交，但是结果还未出来，请使用Retry_request_id查询结果"
		}
		if openaiErr.StatusCode == dto.StatusNewAPIBatchAccepted {
			common.LogError(c, fmt.Sprintf("origin %d error: %s", openaiErr.StatusCode, openaiErr.Error.Message))
			openaiErr.Error.Message = "批量请求已接受，正在处理中，请稍后使用Retry_request_id查询结果"
		}
		if openaiErr.StatusCode == dto.StatusRequestConflict {
			common.LogError(c, fmt.Sprintf("origin %d error: %s", openaiErr.StatusCode, openaiErr.Error.Message))
			openaiErr.Error.Message = "请求冲突，有其他请求使用了这个Retry_request_id，请稍后再试"
		}

		openaiErr.Error.Message = common.MessageWithRequestId(openaiErr.Error.Message, requestId)
		c.JSON(openaiErr.StatusCode, gin.H{
			"error": openaiErr.Error,
		})
	}
}

var upgrader = websocket.Upgrader{
	Subprotocols: []string{"realtime"}, // WS 握手支持的协议，如果有使用 Sec-WebSocket-Protocol，则必须在此声明对应的 Protocol TODO add other protocol
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许跨域
	},
}

func WssRelay(c *gin.Context) {
	// 将 HTTP 连接升级为 WebSocket 连接

	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	defer ws.Close()

	if err != nil {
		openaiErr := service.OpenAIErrorWrapper(err, "get_channel_failed", http.StatusInternalServerError)
		helper.WssError(c, ws, openaiErr.Error)
		return
	}

	startTime := time.Now()
	relayMode := relayconstant.Path2RelayMode(c.Request.URL.Path)
	requestId := c.GetString(common.RequestIdKey)
	group := c.GetString("group")
	//wss://api.openai.com/v1/realtime?model=gpt-4o-realtime-preview-2024-10-01
	originalModel := c.GetString("original_model")
	tokenKey := c.GetString("token_key")
	tokenName := c.GetString("token_name")
	userId := c.GetString("user_id")
	userName := c.GetString("user_name")
	var openaiErr *dto.OpenAIErrorWithStatusCode
	var channel *model.Channel
	defer func() {
		if channel == nil {
			channel = &model.Channel{
				Id:   -1,
				Name: "nofallback",
			}
		}
		var code string
		if openaiErr == nil {
			// 成功请求
			code = "200"
		} else {
			// 失败请求
			code = strconv.Itoa(openaiErr.StatusCode)
			if strings.Contains(openaiErr.Error.Message, "write: connection timed out") && openaiErr.Error.Code == "copy_response_body_failed" {
				code = "499"
			}
		}
		// 统计所有请求的耗时（成功和失败）
		metrics.ObserveRelayRequestE2EDuration(strconv.Itoa(channel.Id), channel.Name, originalModel, group, tokenKey, tokenName, userId, userName, code, time.Since(startTime).Seconds())
	}()

	for i := 0; i <= common.RetryTimes; i++ {
		channel, err = getChannel(c, group, originalModel, i)
		if err != nil {
			common.LogError(c, err.Error())
			openaiErr = service.OpenAIErrorWrapperLocal(err, "get_channel_failed", http.StatusInternalServerError)
			break
		}

		if i == 0 {
			// e2e 用户请求计数
			metrics.IncrementRelayRequestE2ETotalCounter(strconv.Itoa(channel.Id), channel.Name, originalModel, group, tokenKey, tokenName, userId, userName, 1)
		}

		openaiErr = wssRequest(c, ws, relayMode, channel)

		if openaiErr == nil {
			metrics.IncrementRelayRequestE2ESuccessCounter(strconv.Itoa(channel.Id), channel.Name, originalModel, group, tokenKey, tokenName, userId, userName, 1)
			return // 成功处理请求，直接返回
		}

		go processChannelError(c, channel.Id, channel.Type, channel.Name, channel.GetAutoBan(), openaiErr)

		if !shouldRetry(c, openaiErr, common.RetryTimes-i) {
			break
		}
	}
	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		common.LogInfo(c, retryLogStr)
	}

	if openaiErr != nil {
		if openaiErr.StatusCode == http.StatusTooManyRequests {
			openaiErr.Error.Message = "当前分组上游负载已饱和，请稍后再试"
		}
		// 处理自定义的 NewAPI batch 错误码
		if openaiErr.StatusCode == dto.StatusNewAPIBatchRateLimitExceeded {
			common.LogError(c, fmt.Sprintf("origin %d error: %s", openaiErr.StatusCode, openaiErr.Error.Message))
			openaiErr.Error.Message = "当前服务端限速已满，请稍后再试"
		}
		if openaiErr.StatusCode == dto.StatusNewAPIBatchTimeout {
			common.LogError(c, fmt.Sprintf("origin %d error: %s", openaiErr.StatusCode, openaiErr.Error.Message))
			openaiErr.Error.Message = "未等待到结果，请稍后使用Retry_request_id再次查询"
		}
		if openaiErr.StatusCode == dto.StatusNewAPIBatchInternal {
			common.LogError(c, fmt.Sprintf("origin %d error: %s", openaiErr.StatusCode, openaiErr.Error.Message))
			openaiErr.Error.Message = "服务内部错误，请稍后再试"
		}
		if openaiErr.StatusCode == dto.StatusNewAPIBatchSubmitted {
			common.LogError(c, fmt.Sprintf("origin %d error: %s", openaiErr.StatusCode, openaiErr.Error.Message))
			openaiErr.Error.Message = "批量请求已提交，但是结果还未出来，请使用Retry_request_id查询结果"
		}
		if openaiErr.StatusCode == dto.StatusNewAPIBatchAccepted {
			common.LogError(c, fmt.Sprintf("origin %d error: %s", openaiErr.StatusCode, openaiErr.Error.Message))
			openaiErr.Error.Message = "批量请求已接受，正在处理中，请稍后使用Retry_request_id查询结果"
		}
		if openaiErr.StatusCode == dto.StatusRequestConflict {
			common.LogError(c, fmt.Sprintf("origin %d error: %s", openaiErr.StatusCode, openaiErr.Error.Message))
			openaiErr.Error.Message = "请求冲突，有其他请求使用了这个Retry_request_id，请稍后再试"
		}
		// e2e 失败计数
		if channel != nil {
			code := strconv.Itoa(openaiErr.StatusCode)
			if strings.Contains(openaiErr.Error.Message, "write: connection timed out") && openaiErr.Error.Code == "copy_response_body_failed" {
				code = "499"
			}
			metrics.IncrementRelayRequestE2EFailedCounter(strconv.Itoa(channel.Id), channel.Name, originalModel, group, code, tokenKey, tokenName, userId, userName, openaiErr.Error.Message, 1)
		}
		openaiErr.Error.Message = common.MessageWithRequestId(openaiErr.Error.Message, requestId)
		helper.WssError(c, ws, openaiErr.Error)
	}
}

func fillRelayRequest(c *gin.Context, channel *model.Channel) {
	addUsedChannel(c, channel.Id)
	requestBody, _ := common.GetRequestBody(c)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
}

func executeRelayRequest(c *gin.Context, relayMode int, relayInfo *relaycommon.RelayInfo, request interface{}) *dto.OpenAIErrorWithStatusCode {
	return relayExecuteHandler(c, relayMode, relayInfo, request)
}

func wssRequest(c *gin.Context, ws *websocket.Conn, relayMode int, channel *model.Channel) *dto.OpenAIErrorWithStatusCode {
	addUsedChannel(c, channel.Id)
	requestBody, _ := common.GetRequestBody(c)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
	return relay.WssHelper(c, ws)
}

func addUsedChannel(c *gin.Context, channelId int) {
	useChannel := c.GetStringSlice("use_channel")
	useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
	c.Set("use_channel", useChannel)
}

func getChannel(c *gin.Context, group, originalModel string, retryCount int) (*model.Channel, error) {
	if retryCount == 0 {
		autoBan := c.GetBool("auto_ban")
		autoBanInt := 1
		if !autoBan {
			autoBanInt = 0
		}
		channelTag := c.GetString("channel_tag")
		// 获取channel_setting (map[string]interface{}) 并转换为JSON字符串
		var settingStr string
		if settingMap, exists := c.Get("channel_setting"); exists {
			if setting, ok := settingMap.(map[string]interface{}); ok {
				if settingBytes, err := json.Marshal(setting); err == nil {
					settingStr = string(settingBytes)
				}
			}
		}
		return &model.Channel{
			Id:      c.GetInt("channel_id"),
			Type:    c.GetInt("channel_type"),
			Name:    c.GetString("channel_name"),
			Tag:     &channelTag,
			AutoBan: &autoBanInt,
			Setting: settingStr,
		}, nil
	}

	// 获取已使用的渠道列表，传递给 CacheGetRandomSatisfiedChannelExclude 以排除
	excludeChannelIds := make(map[int]bool)
	useChannelList := c.GetStringSlice("use_channel")
	for _, chStr := range useChannelList {
		if chId, err := strconv.Atoi(chStr); err == nil {
			excludeChannelIds[chId] = true
		}
	}

	if len(excludeChannelIds) > 0 {
		excludedIds := make([]int, 0, len(excludeChannelIds))
		for id := range excludeChannelIds {
			excludedIds = append(excludedIds, id)
		}
		common.LogInfo(c, fmt.Sprintf("重试时排除已使用的渠道: %v (重试次数: %d)", excludedIds, retryCount))
	}

	channel, err := model.CacheGetRandomSatisfiedChannelExclude(group, originalModel, retryCount, excludeChannelIds)
	if err != nil {
		// 如果所有渠道都已使用，或者找不到渠道，使用最后一个已使用的渠道
		if len(useChannelList) > 0 {
			lastChannelIdStr := useChannelList[len(useChannelList)-1]
			lastChannelId, parseErr := strconv.Atoi(lastChannelIdStr)
			if parseErr == nil {
				lastChannel, getErr := model.GetChannelById(lastChannelId, true)
				if getErr == nil && lastChannel != nil {
					if strings.Contains(err.Error(), "all channels have been used") {
						common.LogInfo(c, fmt.Sprintf("所有渠道都已使用，使用最后一个已使用的渠道: #%d (重试次数: %d)", lastChannelId, retryCount))
					} else {
						common.LogInfo(c, fmt.Sprintf("无法找到新渠道 (%s)，使用最后一个已使用的渠道: #%d (重试次数: %d)", err.Error(), lastChannelId, retryCount))
					}
					middleware.SetupContextForSelectedChannel(c, lastChannel, originalModel)
					return lastChannel, nil
				}
			}
		}
		return nil, fmt.Errorf("获取重试渠道失败: %s", err.Error())
	}

	// 验证选择的渠道确实不在已使用列表中
	if excludeChannelIds[channel.Id] {
		return nil, fmt.Errorf("选择的重试渠道 #%d 在已使用列表中，这不应该发生", channel.Id)
	}

	common.LogInfo(c, fmt.Sprintf("重试时选择的新渠道: #%d (重试次数: %d)", channel.Id, retryCount))
	middleware.SetupContextForSelectedChannel(c, channel, originalModel)
	return channel, nil
}

func shouldRetry(c *gin.Context, openaiErr *dto.OpenAIErrorWithStatusCode, retryTimes int) bool {
	if openaiErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if openaiErr == nil {
		return false
	}
	if openaiErr.Error.Code == "completion_tokens_zero" || strings.Contains(openaiErr.Error.Message, "No candidates returned") {
		return true
	}
	if openaiErr.LocalError {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	if strings.Contains(openaiErr.Error.Message, "deadline exceeded") ||
		strings.Contains(openaiErr.Error.Message, "request canceled") ||
		strings.Contains(openaiErr.Error.Message, "copy_response_body_failed") {
		common.LogInfo(c, fmt.Sprintf("客户端请求下游超时，不再重试 : %s", openaiErr.Error.Message))
		return false
	}
	if openaiErr.Error.Code == "copy_response_body_failed" {
		common.LogInfo(c, fmt.Sprintf("客户端连接断开，不再重试 : %s", openaiErr.Error.Message))
		return false
	}

	// 处理自定义的 NewAPI batch 错误码
	if openaiErr.StatusCode == dto.StatusNewAPIBatchRateLimitExceeded {
		return false
	}
	if openaiErr.StatusCode == dto.StatusNewAPIBatchTimeout {
		return false
	}
	if openaiErr.StatusCode == dto.StatusNewAPIBatchInternal {
		return false
	}
	if openaiErr.StatusCode == dto.StatusNewAPIBatchSubmitted {
		return false
	}
	if openaiErr.StatusCode == dto.StatusNewAPIBatchAccepted {
		return false
	}
	if openaiErr.StatusCode == dto.StatusRequestConflict {
		return false
	}

	if openaiErr.StatusCode == 307 {
		return true
	}

	if openaiErr.StatusCode/100 == 5 {
		// 超时不重试
		if openaiErr.StatusCode == 504 || openaiErr.StatusCode == 524 {
			return false
		}
		return true
	}
	if openaiErr.StatusCode == http.StatusBadRequest {
		channelType := c.GetInt("channel_type")
		if channelType == common.ChannelTypeAnthropic {
			return true
		}
		return false
	}
	if openaiErr.StatusCode == 408 {
		// azure处理超时不重试
		return false
	}
	if openaiErr.StatusCode/100 == 2 {
		return false
	}
	return false
}

func processChannelError(c *gin.Context, channelId int, channelType int, channelName string, autoBan bool, err *dto.OpenAIErrorWithStatusCode) {
	// 不要使用context获取渠道信息，异步处理时可能会出现渠道信息不一致的情况
	// do not use context to get channel info, there may be inconsistent channel info when processing asynchronously
	common.LogError(c, fmt.Sprintf("relay error (channel #%d, status code: %d): %s", channelId, err.StatusCode, err.Error.Message))
	if service.ShouldDisableChannel(channelType, err) && autoBan {
		service.DisableChannel(channelId, channelName, err.Error.Message)
	}
}

func RelayMidjourney(c *gin.Context) {
	relayMode := c.GetInt("relay_mode")
	var err *dto.MidjourneyResponse
	switch relayMode {
	case relayconstant.RelayModeMidjourneyNotify:
		err = relay.RelayMidjourneyNotify(c)
	case relayconstant.RelayModeMidjourneyTaskFetch, relayconstant.RelayModeMidjourneyTaskFetchByCondition:
		err = relay.RelayMidjourneyTask(c, relayMode)
	case relayconstant.RelayModeMidjourneyTaskImageSeed:
		err = relay.RelayMidjourneyTaskImageSeed(c)
	case relayconstant.RelayModeSwapFace:
		err = relay.RelaySwapFace(c)
	default:
		err = relay.RelayMidjourneySubmit(c, relayMode)
	}
	//err = relayMidjourneySubmit(c, relayMode)
	log.Println(err)
	if err != nil {
		statusCode := http.StatusBadRequest
		if err.Code == 30 {
			err.Result = "当前分组负载已饱和，请稍后再试，或升级账户以提升服务质量。"
			statusCode = http.StatusTooManyRequests
		}
		c.JSON(statusCode, gin.H{
			"description": fmt.Sprintf("%s %s", err.Description, err.Result),
			"type":        "upstream_error",
			"code":        err.Code,
		})
		channelId := c.GetInt("channel_id")
		common.LogError(c, fmt.Sprintf("relay error (channel #%d, status code %d): %s", channelId, statusCode, fmt.Sprintf("%s %s", err.Description, err.Result)))
	}
}

func RelayNotImplemented(c *gin.Context) {
	err := dto.OpenAIError{
		Message: "API not implemented",
		Type:    "new_api_error",
		Param:   "",
		Code:    "api_not_implemented",
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": err,
	})
}

func RelayNotFound(c *gin.Context) {
	err := dto.OpenAIError{
		Message: fmt.Sprintf("Invalid URL (%s %s)", c.Request.Method, c.Request.URL.Path),
		Type:    "invalid_request_error",
		Param:   "",
		Code:    "",
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": err,
	})
}

func RelayTask(c *gin.Context) {
	retryTimes := common.RetryTimes
	channelId := c.GetInt("channel_id")
	relayMode := c.GetInt("relay_mode")
	group := c.GetString("group")
	originalModel := c.GetString("original_model")
	c.Set("use_channel", []string{fmt.Sprintf("%d", channelId)})
	taskErr := taskRelayHandler(c, relayMode)
	if taskErr == nil {
		retryTimes = 0
	}
	for i := 0; shouldRetryTaskRelay(c, channelId, taskErr, retryTimes) && i < retryTimes; i++ {
		channel, err := model.CacheGetRandomSatisfiedChannel(group, originalModel, i)
		if err != nil {
			common.LogError(c, fmt.Sprintf("CacheGetRandomSatisfiedChannel failed: %s", err.Error()))
			break
		}
		channelId = channel.Id
		useChannel := c.GetStringSlice("use_channel")
		useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
		c.Set("use_channel", useChannel)
		common.LogInfo(c, fmt.Sprintf("using channel #%d to retry (remain times %d)", channel.Id, i))
		middleware.SetupContextForSelectedChannel(c, channel, originalModel)

		requestBody, err := common.GetRequestBody(c)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		taskErr = taskRelayHandler(c, relayMode)
	}
	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		common.LogInfo(c, retryLogStr)
	}
	if taskErr != nil {
		if taskErr.StatusCode == http.StatusTooManyRequests {
			taskErr.Message = "当前分组上游负载已饱和，请稍后再试"
		}
		c.JSON(taskErr.StatusCode, taskErr)
	}
}

func taskRelayHandler(c *gin.Context, relayMode int) *dto.TaskError {
	var err *dto.TaskError
	switch relayMode {
	case relayconstant.RelayModeSunoFetch, relayconstant.RelayModeSunoFetchByID:
		err = relay.RelayTaskFetch(c, relayMode)
	default:
		err = relay.RelayTaskSubmit(c, relayMode)
	}
	return err
}

func shouldRetryTaskRelay(c *gin.Context, channelId int, taskErr *dto.TaskError, retryTimes int) bool {
	if taskErr == nil {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	if taskErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if taskErr.StatusCode == 307 {
		return true
	}
	if taskErr.StatusCode/100 == 5 {
		// 超时不重试
		if taskErr.StatusCode == 504 || taskErr.StatusCode == 524 {
			return false
		}
		return true
	}
	if taskErr.StatusCode == http.StatusBadRequest {
		return false
	}
	if taskErr.StatusCode == 408 {
		// azure处理超时不重试
		return false
	}
	if taskErr.LocalError {
		return false
	}
	if taskErr.StatusCode/100 == 2 {
		return false
	}
	return true
}

// handleVideoResponse 处理 /v1/videos 响应，提取 video_id 并存储到 Redis
func handleVideoResponse(c *gin.Context, channelId int) {
	// 从上下文获取响应体
	responseBody, exists := c.Get(common.CtxResponseBody)
	if !exists {
		common.LogInfo(c, "响应体未找到，跳过 video_id 存储")
		return
	}

	responseBodyStr, ok := responseBody.(string)
	if !ok || responseBodyStr == "" {
		common.LogInfo(c, "响应体为空或格式错误，跳过 video_id 存储")
		return
	}

	// 解析 JSON 响应
	var responseData map[string]interface{}
	if err := json.Unmarshal([]byte(responseBodyStr), &responseData); err != nil {
		common.LogError(c, fmt.Sprintf("解析响应 JSON 失败: %v", err))
		return
	}

	// 尝试从不同可能的字段中提取 video_id
	var videoId string
	if id, ok := responseData["id"].(string); ok && id != "" {
		videoId = id
	} else if videoIdVal, ok := responseData["video_id"].(string); ok && videoIdVal != "" {
		videoId = videoIdVal
	} else if videoIdVal, ok := responseData["videoId"].(string); ok && videoIdVal != "" {
		videoId = videoIdVal
	}

	if videoId == "" {
		common.LogInfo(c, "响应中未找到 video_id，跳过存储")
		return
	}

	// 存储到 Redis：key = video_id, value = channel_id
	if common.RedisEnabled {
		channelIdStr := strconv.Itoa(channelId)
		// 设置过期时间为 7 天
		expiration := 7 * 24 * time.Hour
		if err := common.RedisSet(videoId, channelIdStr, expiration); err != nil {
			common.LogError(c, fmt.Sprintf("存储 video_id=%s 到 Redis 失败: %v", videoId, err))
		} else {
			common.LogInfo(c, fmt.Sprintf("成功存储 video_id=%s 到 Redis，对应渠道 ID=%d", videoId, channelId))
		}
	} else {
		common.LogInfo(c, "Redis 未启用，跳过 video_id 存储")
	}
}

// VideoDownloadProxy 直接代理视频请求，不经过 Relay 的完整流程
// 用于 GET /v1/videos/video_xxx、GET /v1/videos/video_xxx/content 和 DELETE /v1/videos/video_xxx 请求
// 直接从 Redis 获取渠道 ID 并转发，支持 variant 查询参数（video、thumbnail、spritesheet）
func VideoDownloadProxy(c *gin.Context) {
	// 检查是否是 GET 或 DELETE 请求且 videoId 以 video_ 开头
	videoId := c.Param("videoId")
	if (c.Request.Method != "GET" && c.Request.Method != "DELETE") || !strings.HasPrefix(videoId, "video_") {
		// 如果不是 GET/DELETE 请求或者不是 video_ 开头，走正常的 Relay 流程
		Relay(c)
		return
	}

	// 从上下文获取渠道 ID（应该在 middleware.Distribute 中已经设置）
	channelId := c.GetInt("channel_id")
	if channelId == 0 {
		// 如果上下文没有渠道 ID，尝试从 Redis 获取
		if common.RedisEnabled {
			channelIdStr, err := common.RedisGet(videoId)
			if err == nil && channelIdStr != "" {
				if id, err := strconv.Atoi(channelIdStr); err == nil {
					channelId = id
				}
			}
		}
	}

	if channelId == 0 {
		common.LogError(c, "无法获取渠道 ID，无法代理视频下载请求")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "无法获取渠道 ID",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// 获取渠道信息
	channel, err := model.GetChannelById(channelId, true)
	if err != nil {
		common.LogError(c, fmt.Sprintf("获取渠道失败: %v", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "获取渠道信息失败",
				"type":    "internal_error",
			},
		})
		return
	}

	if channel == nil {
		common.LogError(c, fmt.Sprintf("渠道 #%d 不存在", channelId))
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": "渠道不存在",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	if channel.Status != common.ChannelStatusEnabled {
		common.LogError(c, fmt.Sprintf("渠道 #%d 已被禁用", channelId))
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"message": "渠道已被禁用",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// 构建目标 URL
	baseURL := channel.GetBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	targetURL := baseURL + c.Request.URL.Path
	if c.Request.URL.RawQuery != "" {
		targetURL += "?" + c.Request.URL.RawQuery
	}

	common.LogInfo(c, fmt.Sprintf("视频下载代理: %s %s -> %s (渠道 #%d)", c.Request.Method, c.Request.URL.Path, targetURL, channelId))

	// 读取请求体（GET 请求通常没有请求体，但为了通用性还是读取）
	requestBody, err := common.GetRequestBody(c)
	if err != nil {
		common.LogError(c, fmt.Sprintf("读取请求体失败: %v", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "读取请求体失败",
				"type":    "internal_error",
			},
		})
		return
	}

	// 创建 HTTP 请求
	req, err := http.NewRequest(c.Request.Method, targetURL, bytes.NewBuffer(requestBody))
	if err != nil {
		common.LogError(c, fmt.Sprintf("创建请求失败: %v", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "创建请求失败",
				"type":    "internal_error",
			},
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
		// 替换 Authorization header 为渠道的 Key
		if lowerKey == "authorization" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", channel.Key))
			continue
		}
		// 保留所有其他头部
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// 创建 HTTP 客户端（使用默认客户端，不使用代理）
	// 使用环境变量 RELAY_TIMEOUT 配置超时时间，如果未设置或为 0，则使用 3600 秒作为默认值
	timeout := 3600 * time.Second
	if common.RelayTimeout > 0 {
		timeout = time.Duration(common.RelayTimeout) * time.Second
	}
	client := &http.Client{
		Timeout: timeout,
	}

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		common.LogError(c, fmt.Sprintf("代理请求失败: %v", err))
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"message": "代理请求失败",
				"type":    "upstream_error",
			},
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

	// 复制响应体（直接流式传输，不缓存）
	_, err = io.Copy(c.Writer, resp.Body)
	if err != nil {
		common.LogError(c, fmt.Sprintf("复制响应体失败: %v", err))
		return
	}

	common.LogInfo(c, fmt.Sprintf("视频下载代理完成: %s %s (状态码: %d)", c.Request.Method, c.Request.URL.Path, resp.StatusCode))
}
