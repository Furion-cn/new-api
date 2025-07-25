package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/model"
	relayconstant "one-api/relay/constant"
	"one-api/service"
	"one-api/setting"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type ModelRequest struct {
	Model string `json:"model"`
}

func Distribute() func(c *gin.Context) {
	return func(c *gin.Context) {
		allowIpsMap := c.GetStringMap("allow_ips")
		if len(allowIpsMap) != 0 {
			clientIp := c.ClientIP()
			if _, ok := allowIpsMap[clientIp]; !ok {
				abortWithOpenAiMessage(c, http.StatusForbidden, "您的 IP 不在令牌允许访问的列表中")
				return
			}
		}
		var channel *model.Channel
		channelId, ok := c.Get("specific_channel_id")
		modelRequest, shouldSelectChannel, err := getModelRequest(c)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "Invalid request, "+err.Error())
			return
		}
		userGroup := c.GetString(constant.ContextKeyUserGroup)
		tokenGroup := c.GetString("token_group")
		if tokenGroup != "" {
			// check common.UserUsableGroups[userGroup]
			if _, ok := setting.GetUserUsableGroups(userGroup)[tokenGroup]; !ok {
				tokenGroupId := setting.GetGroupId(tokenGroup)
				abortWithOpenAiMessage(c, http.StatusForbidden, fmt.Sprintf("令牌分组id %d 已被禁用", tokenGroupId))
				return
			}
			// check group in common.GroupRatio
			if !setting.ContainsGroupRatio(tokenGroup) {
				tokenGroupId := setting.GetGroupId(tokenGroup)
				abortWithOpenAiMessage(c, http.StatusForbidden, fmt.Sprintf("分组id %d 已被弃用", tokenGroupId))
				return
			}
			userGroup = tokenGroup
		}
		c.Set("group", userGroup)
		if ok {
			id, err := strconv.Atoi(channelId.(string))
			if err != nil {
				abortWithOpenAiMessage(c, http.StatusBadRequest, "无效的渠道 Id")
				return
			}
			channel, err = model.GetChannelById(id, true)
			if err != nil {
				abortWithOpenAiMessage(c, http.StatusBadRequest, "无效的渠道 Id")
				return
			}
			if channel.Status != common.ChannelStatusEnabled {
				abortWithOpenAiMessage(c, http.StatusForbidden, "该渠道已被禁用")
				return
			}
		} else {
			// Select a channel for the user
			// check token model mapping
			modelLimitEnable := c.GetBool("token_model_limit_enabled")
			if modelLimitEnable {
				s, ok := c.Get("token_model_limit")
				var tokenModelLimit map[string]bool
				if ok {
					tokenModelLimit = s.(map[string]bool)
				} else {
					tokenModelLimit = map[string]bool{}
				}
				if tokenModelLimit != nil {
					if _, ok := tokenModelLimit[modelRequest.Model]; !ok {
						abortWithOpenAiMessage(c, http.StatusForbidden, "该令牌无权访问模型 "+modelRequest.Model)
						return
					}
				} else {
					// token model limit is empty, all models are not allowed
					abortWithOpenAiMessage(c, http.StatusForbidden, "该令牌无权访问任何模型")
					return
				}
			}

			common.LogInfo(c, "userGroup: "+userGroup+" modelRequest.Model: "+modelRequest.Model)
			if shouldSelectChannel {
				channel, err = model.CacheGetRandomSatisfiedChannel(userGroup, modelRequest.Model, 0)
				if err != nil {
					userGroupId := setting.GetGroupId(userGroup)
					message := fmt.Sprintf("当前分组id %d 下对于模型 %s 无可用渠道", userGroupId, modelRequest.Model)
					// 如果错误，但是渠道不为空，说明是数据库一致性问题
					if channel != nil {
						common.SysError(fmt.Sprintf("渠道不存在：%d", channel.Id))
						message = "数据库一致性已被破坏，请联系管理员"
					}
					// 如果错误，而且渠道为空，说明是没有可用渠道
					abortWithOpenAiMessage(c, http.StatusServiceUnavailable, message)
					return
				}
				if channel == nil {
					userGroupId := setting.GetGroupId(userGroup)
					abortWithOpenAiMessage(c, http.StatusServiceUnavailable, fmt.Sprintf("当前分组id %d 下对于模型 %s 无可用渠道（数据库一致性已被破坏）", userGroupId, modelRequest.Model))
					return
				}
			}
		}
		c.Set(constant.ContextKeyRequestStartTime, time.Now())
		SetupContextForSelectedChannel(c, channel, modelRequest.Model)
		c.Next()
	}
}

func getModelRequest(c *gin.Context) (*ModelRequest, bool, error) {
	var modelRequest ModelRequest
	shouldSelectChannel := true
	var err error

	// 添加对 /batchjob/presign 和 /batchjob/register 路径的处理
	if strings.HasPrefix(c.Request.URL.Path, "/batchjob/presign") ||
		strings.HasPrefix(c.Request.URL.Path, "/batchjob/register") ||
		strings.HasPrefix(c.Request.URL.Path, "/batchjob/startjob") {
		modelRequest.Model = c.Query("model")
		return &modelRequest, shouldSelectChannel, nil
	}

	if strings.Contains(c.Request.URL.Path, "/mj/") {
		relayMode := relayconstant.Path2RelayModeMidjourney(c.Request.URL.Path)
		if relayMode == relayconstant.RelayModeMidjourneyTaskFetch ||
			relayMode == relayconstant.RelayModeMidjourneyTaskFetchByCondition ||
			relayMode == relayconstant.RelayModeMidjourneyNotify ||
			relayMode == relayconstant.RelayModeMidjourneyTaskImageSeed {
			shouldSelectChannel = false
		} else {
			midjourneyRequest := dto.MidjourneyRequest{}
			err = common.UnmarshalBodyReusable(c, &midjourneyRequest)
			if err != nil {
				return nil, false, fmt.Errorf("无效的请求, %s", err.Error())
			}
			midjourneyModel, mjErr, success := service.GetMjRequestModel(relayMode, &midjourneyRequest)
			if mjErr != nil {
				return nil, false, fmt.Errorf(mjErr.Description)
			}
			if midjourneyModel == "" {
				if !success {
					return nil, false, fmt.Errorf("无效的请求, 无法解析模型")
				} else {
					// task fetch, task fetch by condition, notify
					shouldSelectChannel = false
				}
			}
			modelRequest.Model = midjourneyModel
		}
		c.Set("relay_mode", relayMode)
	} else if strings.Contains(c.Request.URL.Path, "/suno/") {
		relayMode := relayconstant.Path2RelaySuno(c.Request.Method, c.Request.URL.Path)
		if relayMode == relayconstant.RelayModeSunoFetch ||
			relayMode == relayconstant.RelayModeSunoFetchByID {
			shouldSelectChannel = false
		} else {
			modelName := service.CoverTaskActionToModelName(constant.TaskPlatformSuno, c.Param("action"))
			modelRequest.Model = modelName
		}
		c.Set("platform", string(constant.TaskPlatformSuno))
		c.Set("relay_mode", relayMode)
	} else if !strings.HasPrefix(c.Request.URL.Path, "/v1/audio/transcriptions") {
		// 检查请求体是否为空
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			return nil, false, fmt.Errorf("无效的请求, 读取请求体失败: %s", err.Error())
		}
		// 重置请求体
		c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

		// 如果请求体为空，根据路径设置默认模型
		if len(body) == 0 {
			if strings.HasPrefix(c.Request.URL.Path, "/v1/moderations") {
				modelRequest.Model = "text-moderation-stable"
			} else if strings.HasSuffix(c.Request.URL.Path, "embeddings") {
				modelRequest.Model = c.Param("model")
			} else if strings.HasPrefix(c.Request.URL.Path, "/v1/images/generations") {
				modelRequest.Model = "dall-e"
			} else if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/speech") {
				modelRequest.Model = "tts-1"
			} else if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/translations") {
				modelRequest.Model = "whisper-1"
			} else if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/transcriptions") {
				modelRequest.Model = "whisper-1"
			} else if strings.HasPrefix(c.Request.URL.Path, "/v1/realtime") {
				modelRequest.Model = c.Query("model")
			}
		} else {
			// 请求体不为空，尝试解析 JSON
			err = json.Unmarshal(body, &modelRequest)
			if err != nil {
				return nil, false, fmt.Errorf("无效的请求, JSON 解析失败: %s", err.Error())
			}
		}
	}

	if strings.HasPrefix(c.Request.URL.Path, "/v1/realtime") {
		//wss://api.openai.com/v1/realtime?model=gpt-4o-realtime-preview-2024-10-01
		modelRequest.Model = c.Query("model")
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/moderations") {
		if modelRequest.Model == "" {
			modelRequest.Model = "text-moderation-stable"
		}
	}
	if strings.HasSuffix(c.Request.URL.Path, "embeddings") {
		if modelRequest.Model == "" {
			modelRequest.Model = c.Param("model")
		}
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/images/generations") {
		modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "dall-e")
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/audio") {
		relayMode := relayconstant.RelayModeAudioSpeech
		if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/speech") {
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "tts-1")
		} else if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/translations") {
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, c.PostForm("model"))
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "whisper-1")
			relayMode = relayconstant.RelayModeAudioTranslation
		} else if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/transcriptions") {
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, c.PostForm("model"))
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "whisper-1")
			relayMode = relayconstant.RelayModeAudioTranscription
		}
		c.Set("relay_mode", relayMode)
	}
	return &modelRequest, shouldSelectChannel, nil
}

func SetupContextForSelectedChannel(c *gin.Context, channel *model.Channel, modelName string) {
	c.Set("original_model", modelName) // for retry
	if channel == nil {
		return
	}
	c.Set("channel_id", channel.Id)
	c.Set("channel_name", channel.Name)
	c.Set("channel_type", channel.Type)
	c.Set("channel_setting", channel.GetSetting())
	c.Set("channel_tag", channel.GetTag())
	if nil != channel.OpenAIOrganization && "" != *channel.OpenAIOrganization {
		c.Set("channel_organization", *channel.OpenAIOrganization)
	}
	c.Set("auto_ban", channel.GetAutoBan())
	c.Set("model_mapping", channel.GetModelMapping())
	c.Set("status_code_mapping", channel.GetStatusCodeMapping())
	c.Request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", channel.Key))
	c.Set("base_url", channel.GetBaseURL())
	c.Set("endpoint", channel.GetEndpoint())
	// TODO: api_version统一
	switch channel.Type {
	case common.ChannelTypeAzure:
		c.Set("api_version", channel.Other)
	case common.ChannelTypeVertexAi:
		c.Set("region", channel.Other)
	case common.ChannelTypeXunfei:
		c.Set("api_version", channel.Other)
	case common.ChannelTypeGemini:
		c.Set("api_version", channel.Other)
	case common.ChannelTypeAli:
		c.Set("plugin", channel.Other)
	case common.ChannelCloudflare:
		c.Set("api_version", channel.Other)
	case common.ChannelTypeMokaAI:
		c.Set("api_version", channel.Other)
	}
}
