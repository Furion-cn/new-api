package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"one-api/common"
	"one-api/dto"
	"one-api/metrics"
	relaycommon "one-api/relay/common"
	"one-api/relay/helper"
	"one-api/service"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func getRerankPromptToken(rerankRequest dto.RerankRequest) int {
	token, _ := service.CountTokenInput(rerankRequest.Query, rerankRequest.Model)
	for _, document := range rerankRequest.Documents {
		tkm, err := service.CountTokenInput(document, rerankRequest.Model)
		if err == nil {
			token += tkm
		}
	}
	return token
}

func RerankInfo(c *gin.Context) (*relaycommon.RelayInfo, *dto.RerankRequest, *dto.OpenAIErrorWithStatusCode) {
	relayInfo := relaycommon.GenRelayInfo(c)

	var rerankRequest *dto.RerankRequest
	err := common.UnmarshalBodyReusable(c, &rerankRequest)
	if err != nil {
		common.LogError(c, fmt.Sprintf("getAndValidateTextRequest failed: %s", err.Error()))
		return nil, nil, service.OpenAIErrorWrapperLocal(err, "invalid_text_request", http.StatusBadRequest)
	}
	return relayInfo, rerankRequest, nil
}

func RerankHelper(c *gin.Context, relayInfo *relaycommon.RelayInfo, rerankRequest *dto.RerankRequest) (openaiErr *dto.OpenAIErrorWithStatusCode) {
	startTime := time.Now()
	var funcErr *dto.OpenAIErrorWithStatusCode

	var statusCode int = -1
	metrics.IncrementRelayRequestTotalCounter(strconv.Itoa(relayInfo.ChannelId), relayInfo.ChannelName, relayInfo.ChannelTag, relayInfo.BaseUrl, rerankRequest.Model, relayInfo.Group, strconv.Itoa(relayInfo.UserId), relayInfo.UserName, 1)
	defer func() {
		if funcErr != nil {
			metrics.IncrementRelayRequestFailedCounter(strconv.Itoa(relayInfo.ChannelId), relayInfo.ChannelName, relayInfo.ChannelTag, relayInfo.BaseUrl, rerankRequest.Model, relayInfo.Group, strconv.Itoa(funcErr.StatusCode), strconv.Itoa(relayInfo.UserId), relayInfo.UserName, 1)
		} else {
			metrics.IncrementRelayRequestSuccessCounter(strconv.Itoa(relayInfo.ChannelId), relayInfo.ChannelName, relayInfo.ChannelTag, relayInfo.BaseUrl, rerankRequest.Model, relayInfo.Group, strconv.Itoa(statusCode), strconv.Itoa(relayInfo.UserId), relayInfo.UserName, 1)
			metrics.ObserveRelayRequestDuration(strconv.Itoa(relayInfo.ChannelId), relayInfo.ChannelName, relayInfo.ChannelTag, relayInfo.BaseUrl, rerankRequest.Model, relayInfo.Group, strconv.Itoa(relayInfo.UserId), relayInfo.UserName, time.Since(startTime).Seconds())
		}
	}()
	if rerankRequest.Query == "" {
		funcErr = service.OpenAIErrorWrapperLocal(fmt.Errorf("query is empty"), "invalid_query", http.StatusBadRequest)
		return funcErr
	}
	if len(rerankRequest.Documents) == 0 {
		funcErr = service.OpenAIErrorWrapperLocal(fmt.Errorf("documents is empty"), "invalid_documents", http.StatusBadRequest)
		return funcErr
	}

	err := helper.ModelMappedHelper(c, relayInfo)
	if err != nil {
		funcErr = service.OpenAIErrorWrapperLocal(err, "model_mapped_error", http.StatusInternalServerError)
		return funcErr
	}

	rerankRequest.Model = relayInfo.UpstreamModelName

	promptToken := getRerankPromptToken(*rerankRequest)
	relayInfo.PromptTokens = promptToken

	priceData, err := helper.ModelPriceHelper(c, relayInfo, promptToken, 0)
	if err != nil {
		funcErr = service.OpenAIErrorWrapperLocal(err, "model_price_error", http.StatusInternalServerError)
		return funcErr
	}
	// pre-consume quota 预消耗配额
	preConsumedQuota, userQuota, openaiErr := preConsumeQuota(c, priceData.ShouldPreConsumedQuota, relayInfo)
	if openaiErr != nil {
		funcErr = openaiErr
		return openaiErr
	}
	defer func() {
		if openaiErr != nil {
			returnPreConsumedQuota(c, relayInfo, userQuota, preConsumedQuota)
		}
	}()

	adaptor := GetAdaptor(relayInfo.ApiType)
	if adaptor == nil {
		funcErr = service.OpenAIErrorWrapperLocal(fmt.Errorf("invalid api type: %d", relayInfo.ApiType), "invalid_api_type", http.StatusBadRequest)
		return funcErr
	}
	adaptor.Init(relayInfo)

	convertedRequest, err := adaptor.ConvertRerankRequest(c, relayInfo.RelayMode, *rerankRequest)
	if err != nil {
		funcErr = service.OpenAIErrorWrapperLocal(err, "convert_request_failed", http.StatusInternalServerError)
		return funcErr
	}
	jsonData, err := json.Marshal(convertedRequest)
	if err != nil {
		funcErr = service.OpenAIErrorWrapperLocal(err, "json_marshal_failed", http.StatusInternalServerError)
		return funcErr
	}
	requestBody := bytes.NewBuffer(jsonData)
	statusCodeMappingStr := c.GetString("status_code_mapping")
	resp, err := adaptor.DoRequest(c, relayInfo, requestBody)
	if err != nil {
		funcErr = service.OpenAIErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
		return funcErr
	}

	var httpResp *http.Response
	if resp != nil {
		httpResp = resp.(*http.Response)
		if httpResp.StatusCode != http.StatusOK {
			openaiErr = service.RelayErrorHandler(httpResp)
			funcErr = openaiErr
			// reset status code 重置状态码
			service.ResetStatusCode(openaiErr, statusCodeMappingStr)
			return openaiErr
		}
	}

	usage, openaiErr := adaptor.DoResponse(c, httpResp, relayInfo)
	if openaiErr != nil {
		funcErr = openaiErr
		// reset status code 重置状态码
		service.ResetStatusCode(openaiErr, statusCodeMappingStr)
		return openaiErr
	}

	// 设置状态码用于指标记录
	statusCode = resp.(*http.Response).StatusCode

	postConsumeQuota(c, relayInfo, usage.(*dto.Usage), preConsumedQuota, userQuota, priceData, "")
	return nil
}
