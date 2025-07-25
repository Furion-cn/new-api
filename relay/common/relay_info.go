package common

import (
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	relayconstant "one-api/relay/constant"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type ThinkingContentInfo struct {
	IsFirstThinkingContent  bool
	SendLastThinkingContent bool
}

type RelayInfo struct {
	ChannelType       int
	ChannelId         int
	ChannelTag        string
	ChannelName       string
	TokenId           int
	TokenKey          string
	UserId            int
	UserName          string
	Group             string
	TokenUnlimited    bool
	StartTime         time.Time
	FirstResponseTime time.Time
	isFirstResponse   bool
	//SendLastReasoningResponse bool
	ApiType           int
	IsStream          bool
	IsPlayground      bool
	UsePrice          bool
	RelayMode         int
	UpstreamModelName string
	OriginModelName   string
	//RecodeModelName      string
	RequestURLPath       string
	ApiVersion           string
	PromptTokens         int
	ApiKey               string
	Organization         string
	BaseUrl              string
	Endpoint             string
	SupportStreamOptions bool
	ShouldIncludeUsage   bool
	IsModelMapped        bool
	ClientWs             *websocket.Conn
	TargetWs             *websocket.Conn
	InputAudioFormat     string
	OutputAudioFormat    string
	RealtimeTools        []dto.RealTimeTool
	IsFirstRequest       bool
	AudioUsage           bool
	ReasoningEffort      string
	ChannelSetting       map[string]interface{}
	UserSetting          map[string]interface{}
	UserEmail            string
	UserQuota            int
	Direct               bool
	RetryCount           int
	Headers              map[string]string
	ThinkingContentInfo
}

// 定义支持流式选项的通道类型
var streamSupportedChannels = map[int]bool{
	common.ChannelTypeOpenAI:     true,
	common.ChannelTypeAnthropic:  true,
	common.ChannelTypeAws:        true,
	common.ChannelTypeGemini:     true,
	common.ChannelCloudflare:     true,
	common.ChannelTypeAzure:      true,
	common.ChannelTypeVolcEngine: true,
	common.ChannelTypeOllama:     true,
	common.ChannelTypeVertexAi:   true,
}

func GenRelayInfoWs(c *gin.Context, ws *websocket.Conn) *RelayInfo {
	info := GenRelayInfo(c)
	info.ClientWs = ws
	info.InputAudioFormat = "pcm16"
	info.OutputAudioFormat = "pcm16"
	info.IsFirstRequest = true
	return info
}

func GenRelayInfo(c *gin.Context) *RelayInfo {
	channelType := c.GetInt("channel_type")
	channelId := c.GetInt("channel_id")
	channelSetting := c.GetStringMap("channel_setting")
	channelTag := c.GetString("channel_tag")
	channelName := c.GetString("channel_name")
	tokenId := c.GetInt("token_id")
	tokenKey := c.GetString("token_key")
	userId := c.GetInt("id")
	group := c.GetString("group")
	tokenUnlimited := c.GetBool("token_unlimited_quota")
	startTime := c.GetTime(constant.ContextKeyRequestStartTime)
	// firstResponseTime = time.Now() - 1 second

	apiType, _ := relayconstant.ChannelType2APIType(channelType)

	info := &RelayInfo{
		UserQuota:         c.GetInt(constant.ContextKeyUserQuota),
		UserSetting:       c.GetStringMap(constant.ContextKeyUserSetting),
		UserEmail:         c.GetString(constant.ContextKeyUserEmail),
		UserName:          c.GetString(constant.ContextKeyUserName),
		isFirstResponse:   true,
		RelayMode:         relayconstant.Path2RelayMode(c.Request.URL.Path),
		BaseUrl:           c.GetString("base_url"),
		Endpoint:          c.GetString("endpoint"),
		RequestURLPath:    c.Request.URL.String(),
		ChannelType:       channelType,
		ChannelId:         channelId,
		ChannelTag:        channelTag,
		ChannelName:       channelName,
		TokenId:           tokenId,
		TokenKey:          tokenKey,
		UserId:            userId,
		Group:             group,
		TokenUnlimited:    tokenUnlimited,
		StartTime:         startTime,
		FirstResponseTime: startTime.Add(-time.Second),
		OriginModelName:   c.GetString("original_model"),
		UpstreamModelName: c.GetString("original_model"),
		//RecodeModelName:   c.GetString("original_model"),
		IsModelMapped:  false,
		ApiType:        apiType,
		ApiVersion:     c.GetString("api_version"),
		ApiKey:         strings.TrimPrefix(c.Request.Header.Get("Authorization"), "Bearer "),
		Organization:   c.GetString("channel_organization"),
		ChannelSetting: channelSetting,
		Headers:        make(map[string]string),
		ThinkingContentInfo: ThinkingContentInfo{
			IsFirstThinkingContent:  true,
			SendLastThinkingContent: false,
		},
	}
	// 使用直连模式
	if strings.HasPrefix(c.Request.URL.Path, "/v1/messages") {
		info.Direct = true
	}

	if strings.HasPrefix(c.Request.URL.Path, "/pg") {
		info.IsPlayground = true
		info.RequestURLPath = strings.TrimPrefix(info.RequestURLPath, "/pg")
		info.RequestURLPath = "/v1" + info.RequestURLPath
	}
	if info.BaseUrl == "" {
		info.BaseUrl = common.ChannelBaseURLs[channelType]
	}
	if info.ChannelType == common.ChannelTypeAzure {
		info.ApiVersion = GetAPIVersion(c)
	}
	if info.ChannelType == common.ChannelTypeVertexAi {
		info.ApiVersion = c.GetString("region")
	}
	if streamSupportedChannels[info.ChannelType] {
		info.SupportStreamOptions = true
	}
	return info
}

func (info *RelayInfo) SetPromptTokens(promptTokens int) {
	info.PromptTokens = promptTokens
}

func (info *RelayInfo) SetIsStream(isStream bool) {
	info.IsStream = isStream
}

func (info *RelayInfo) SetFirstResponseTime() {
	if info.isFirstResponse {
		info.FirstResponseTime = time.Now()
		info.isFirstResponse = false
	}
}

type TaskRelayInfo struct {
	*RelayInfo
	Action       string
	OriginTaskID string

	ConsumeQuota bool
}

func GenTaskRelayInfo(c *gin.Context) *TaskRelayInfo {
	info := &TaskRelayInfo{
		RelayInfo: GenRelayInfo(c),
	}
	return info
}
