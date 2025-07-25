package gemini

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	relaycommon "one-api/relay/common"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/setting/model_setting"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"google.golang.org/genai"
)

// Setting safety to the lowest possible values since Gemini is already powerless enough
func CovertGemini2OpenAI(textRequest dto.GeneralOpenAIRequest) (*GeminiChatRequest, error) {

	geminiRequest := GeminiChatRequest{
		Contents: make([]GeminiChatContent, 0, len(textRequest.Messages)),
		//SafetySettings: []GeminiChatSafetySettings{},
	}

	// 初始化GenerationConfig
	geminiRequest.GenerationConfig = &genai.GenerateContentConfig{
		Temperature: func() *float32 {
			if textRequest.Temperature != nil {
				temp := float32(*textRequest.Temperature)
				return &temp
			}
			return nil
		}(),
		TopP: func() *float32 {
			topP := float32(textRequest.TopP)
			return &topP
		}(),
		MaxOutputTokens: int32(textRequest.MaxTokens),
		Seed: func() *int32 {
			seed := int32(textRequest.Seed)
			return &seed
		}(),
	}

	// 如果有传入的GenerationConfig，合并配置
	if textRequest.GenerationConfig != nil {
		// 合并ResponseMIMEType和ResponseSchema
		if textRequest.GenerationConfig.ResponseMIMEType != "" {
			geminiRequest.GenerationConfig.ResponseMIMEType = textRequest.GenerationConfig.ResponseMIMEType
		}
		if textRequest.GenerationConfig.ResponseSchema != nil {
			geminiRequest.GenerationConfig.ResponseSchema = textRequest.GenerationConfig.ResponseSchema
		}
		// 合并ThinkingConfig（如果传入的没有，则使用thinking字段）
		if textRequest.GenerationConfig.ThinkingConfig != nil {
			geminiRequest.GenerationConfig.ThinkingConfig = textRequest.GenerationConfig.ThinkingConfig
		} else if textRequest.Thinking != nil {
			budget := int32(textRequest.Thinking.BudgetTokens)
			geminiRequest.GenerationConfig.ThinkingConfig = &genai.ThinkingConfig{
				ThinkingBudget: &budget,
			}
		}
	} else if textRequest.Thinking != nil {
		// 如果没有GenerationConfig但有thinking字段
		budget := int32(textRequest.Thinking.BudgetTokens)
		geminiRequest.GenerationConfig.ThinkingConfig = &genai.ThinkingConfig{
			ThinkingBudget: &budget,
		}
	}

	safetySettings := make([]GeminiChatSafetySettings, 0, len(SafetySettingList))
	for _, category := range SafetySettingList {
		safetySettings = append(safetySettings, GeminiChatSafetySettings{
			Category:  category,
			Threshold: model_setting.GetGeminiSafetySetting(category),
		})
	}
	geminiRequest.SafetySettings = safetySettings

	// openaiContent.FuncToToolCalls()
	if textRequest.Tools != nil {
		functions := make([]dto.FunctionRequest, 0, len(textRequest.Tools))
		googleSearch := false
		codeExecution := false
		for _, tool := range textRequest.Tools {
			if tool.Function.Name == "googleSearch" {
				googleSearch = true
				continue
			}
			if tool.Function.Name == "codeExecution" {
				codeExecution = true
				continue
			}
			if tool.Function.Parameters != nil {
				params, ok := tool.Function.Parameters.(map[string]interface{})
				if ok {
					if props, hasProps := params["properties"].(map[string]interface{}); hasProps {
						if len(props) == 0 {
							tool.Function.Parameters = nil
						}
					}
				}
			}
			functions = append(functions, tool.Function)
		}
		if codeExecution {
			geminiRequest.Tools = append(geminiRequest.Tools, GeminiChatTool{
				CodeExecution: make(map[string]string),
			})
		}
		if googleSearch {
			geminiRequest.Tools = append(geminiRequest.Tools, GeminiChatTool{
				GoogleSearch: make(map[string]string),
			})
		}
		if len(functions) > 0 {
			geminiRequest.Tools = append(geminiRequest.Tools, GeminiChatTool{
				FunctionDeclarations: functions,
			})
		}
		// common.SysLog("tools: " + fmt.Sprintf("%+v", geminiRequest.Tools))
		// json_data, _ := json.Marshal(geminiRequest.Tools)
		// common.SysLog("tools_json: " + string(json_data))
	} else if textRequest.Functions != nil {
		geminiRequest.Tools = []GeminiChatTool{
			{
				FunctionDeclarations: textRequest.Functions,
			},
		}
	}

	// 保持原有的ResponseFormat处理逻辑作为兼容性支持
	if textRequest.ResponseFormat != nil && (textRequest.ResponseFormat.Type == "json_schema" || textRequest.ResponseFormat.Type == "json_object") {
		geminiRequest.GenerationConfig.ResponseMIMEType = "application/json"

		if textRequest.ResponseFormat.JsonSchema != nil && textRequest.ResponseFormat.JsonSchema.Schema != nil {
			// 直接使用ResponseJsonSchema字段，不需要转换
			geminiRequest.GenerationConfig.ResponseJsonSchema = textRequest.ResponseFormat.JsonSchema.Schema
		}
	}
	tool_call_ids := make(map[string]string)
	var system_content []string
	//shouldAddDummyModelMessage := false
	for _, message := range textRequest.Messages {
		if message.Role == "system" {
			system_content = append(system_content, message.StringContent())
			continue
		} else if message.Role == "tool" || message.Role == "function" {
			if len(geminiRequest.Contents) == 0 || geminiRequest.Contents[len(geminiRequest.Contents)-1].Role == "model" {
				geminiRequest.Contents = append(geminiRequest.Contents, GeminiChatContent{
					Role: "user",
				})
			}
			var parts = &geminiRequest.Contents[len(geminiRequest.Contents)-1].Parts
			name := ""
			if message.Name != nil {
				name = *message.Name
			} else if val, exists := tool_call_ids[message.ToolCallId]; exists {
				name = val
			}
			content := common.StrToMap(message.StringContent())
			functionResp := &FunctionResponse{
				Name: name,
				Response: GeminiFunctionResponseContent{
					Name:    name,
					Content: content,
				},
			}
			if content == nil {
				functionResp.Response.Content = message.StringContent()
			}
			*parts = append(*parts, GeminiPart{
				FunctionResponse: functionResp,
			})
			continue
		}
		var parts []GeminiPart
		content := GeminiChatContent{
			Role: message.Role,
		}
		// isToolCall := false
		if message.ToolCalls != nil {
			// message.Role = "model"
			// isToolCall = true
			for _, call := range message.ParseToolCalls() {
				args := map[string]interface{}{}
				if call.Function.Arguments != "" {
					if json.Unmarshal([]byte(call.Function.Arguments), &args) != nil {
						return nil, fmt.Errorf("invalid arguments for function %s, args: %s", call.Function.Name, call.Function.Arguments)
					}
				}
				toolCall := GeminiPart{
					FunctionCall: &FunctionCall{
						FunctionName: call.Function.Name,
						Arguments:    args,
					},
				}
				parts = append(parts, toolCall)
				tool_call_ids[call.ID] = call.Function.Name
			}
		}

		openaiContent := message.ParseContent()
		imageNum := 0
		for _, part := range openaiContent {
			if part.Type == dto.ContentTypeText {
				if part.Text == "" {
					continue
				}
				parts = append(parts, GeminiPart{
					Text: part.Text,
				})
			} else if part.Type == dto.ContentTypeImageURL {
				imageNum += 1

				if constant.GeminiVisionMaxImageNum != -1 && imageNum > constant.GeminiVisionMaxImageNum {
					return nil, fmt.Errorf("too many images in the message, max allowed is %d", constant.GeminiVisionMaxImageNum)
				}
				// 判断是否是url
				if strings.HasPrefix(part.ImageUrl.(dto.MessageImageUrl).Url, "http") {
					// 是url，获取图片的类型和base64编码的数据
					fileData, err := service.GetFileBase64FromUrl(part.ImageUrl.(dto.MessageImageUrl).Url)
					if err != nil {
						return nil, fmt.Errorf("get file base64 from url failed: %s", err.Error())
					}
					parts = append(parts, GeminiPart{
						InlineData: &GeminiInlineData{
							MimeType: fileData.MimeType,
							Data:     fileData.Base64Data,
						},
					})
				} else {
					format, base64String, err := service.DecodeBase64FileData(part.ImageUrl.(dto.MessageImageUrl).Url)
					if err != nil {
						return nil, fmt.Errorf("decode base64 image data failed: %s", err.Error())
					}
					parts = append(parts, GeminiPart{
						InlineData: &GeminiInlineData{
							MimeType: format,
							Data:     base64String,
						},
					})
				}
			} else if part.Type == dto.ContentTypeInputAudio {
				// 处理音频内容
				audioData := part.InputAudio.(dto.MessageInputAudio)
				// 添加调试日志
				common.SysLog(fmt.Sprintf("Processing audio data: format=%s, data length=%d", audioData.Format, len(audioData.Data)))
				// 将音频数据转换为Gemini的InlineData格式
				parts = append(parts, GeminiPart{
					InlineData: &GeminiInlineData{
						MimeType: audioData.Format,
						Data:     audioData.Data,
					},
					VideoMetadata: &GeminiVideoMetadata{
						Fps: audioData.Fps,
					},
				})
			} else if part.Type == dto.ContentTypeYoutube {
				parts = append(parts, GeminiPart{
					FileData: &GeminiFileData{
						MimeType: part.Text,
						FileUri:  part.ImageUrl.(dto.MessageImageUrl).Url,
					},
				})
			}
		}

		content.Parts = parts

		// there's no assistant role in gemini and API shall vomit if Role is not user or model
		if content.Role == "assistant" {
			content.Role = "model"
		}
		geminiRequest.Contents = append(geminiRequest.Contents, content)
	}

	if len(system_content) > 0 {
		geminiRequest.SystemInstructions = &GeminiChatContent{
			Parts: []GeminiPart{
				{
					Text: strings.Join(system_content, "\n"),
				},
			},
		}
	}

	return &geminiRequest, nil
}

func removeAdditionalPropertiesWithDepth(schema interface{}, depth int) interface{} {
	if depth >= 5 {
		return schema
	}

	v, ok := schema.(map[string]interface{})
	if !ok || len(v) == 0 {
		return schema
	}
	// 删除所有的title字段
	delete(v, "title")
	// 如果type不为object和array，则直接返回
	if typeVal, exists := v["type"]; !exists || (typeVal != "object" && typeVal != "array") {
		return schema
	}
	switch v["type"] {
	case "object":
		delete(v, "additionalProperties")
		// 处理 properties
		if properties, ok := v["properties"].(map[string]interface{}); ok {
			for key, value := range properties {
				properties[key] = removeAdditionalPropertiesWithDepth(value, depth+1)
			}
		}
		for _, field := range []string{"allOf", "anyOf", "oneOf"} {
			if nested, ok := v[field].([]interface{}); ok {
				for i, item := range nested {
					nested[i] = removeAdditionalPropertiesWithDepth(item, depth+1)
				}
			}
		}
	case "array":
		if items, ok := v["items"].(map[string]interface{}); ok {
			v["items"] = removeAdditionalPropertiesWithDepth(items, depth+1)
		}
	}

	return v
}

func unescapeString(s string) (string, error) {
	var result []rune
	escaped := false
	i := 0

	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:]) // 正确解码UTF-8字符
		if r == utf8.RuneError {
			return "", fmt.Errorf("invalid UTF-8 encoding")
		}

		if escaped {
			// 如果是转义符后的字符，检查其类型
			switch r {
			case '"':
				result = append(result, '"')
			case '\\':
				result = append(result, '\\')
			case '/':
				result = append(result, '/')
			case 'b':
				result = append(result, '\b')
			case 'f':
				result = append(result, '\f')
			case 'n':
				result = append(result, '\n')
			case 'r':
				result = append(result, '\r')
			case 't':
				result = append(result, '\t')
			case '\'':
				result = append(result, '\'')
			default:
				// 如果遇到一个非法的转义字符，直接按原样输出
				result = append(result, '\\', r)
			}
			escaped = false
		} else {
			if r == '\\' {
				escaped = true // 记录反斜杠作为转义符
			} else {
				result = append(result, r)
			}
		}
		i += size // 移动到下一个字符
	}

	return string(result), nil
}

func unescapeMapOrSlice(data interface{}) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		for k, val := range v {
			v[k] = unescapeMapOrSlice(val)
		}
	case []interface{}:
		for i, val := range v {
			v[i] = unescapeMapOrSlice(val)
		}
	case string:
		if unescaped, err := unescapeString(v); err != nil {
			return v
		} else {
			return unescaped
		}
	}
	return data
}

func getResponseToolCall(item *GeminiPart) *dto.ToolCallResponse {
	var argsBytes []byte
	var err error
	if result, ok := item.FunctionCall.Arguments.(map[string]interface{}); ok {
		argsBytes, err = json.Marshal(unescapeMapOrSlice(result))
	} else {
		argsBytes, err = json.Marshal(item.FunctionCall.Arguments)
	}

	if err != nil {
		return nil
	}
	return &dto.ToolCallResponse{
		ID:   fmt.Sprintf("call_%s", common.GetUUID()),
		Type: "function",
		Function: dto.FunctionResponse{
			Arguments: string(argsBytes),
			Name:      item.FunctionCall.FunctionName,
		},
	}
}

func responseGeminiChat2OpenAI(response *GeminiChatResponse) *dto.OpenAITextResponse {
	fullTextResponse := dto.OpenAITextResponse{
		Id:      fmt.Sprintf("chatcmpl-%s", common.GetUUID()),
		Object:  "chat.completion",
		Created: common.GetTimestamp(),
		Choices: make([]dto.OpenAITextResponseChoice, 0, len(response.Candidates)),
	}
	content, _ := json.Marshal("")
	isToolCall := false
	for _, candidate := range response.Candidates {
		choice := dto.OpenAITextResponseChoice{
			Index: int(candidate.Index),
			Message: dto.Message{
				Role:    "assistant",
				Content: content,
			},
			FinishReason: constant.FinishReasonStop,
		}
		if len(candidate.Content.Parts) > 0 {
			var texts []string
			var toolCalls []dto.ToolCallResponse
			for _, part := range candidate.Content.Parts {
				if part.FunctionCall != nil {
					choice.FinishReason = constant.FinishReasonToolCalls
					if call := getResponseToolCall(&part); call != nil {
						toolCalls = append(toolCalls, *call)
					}
				} else {
					if part.ExecutableCode != nil {
						texts = append(texts, "```"+part.ExecutableCode.Language+"\n"+part.ExecutableCode.Code+"\n```")
					} else if part.CodeExecutionResult != nil {
						texts = append(texts, "```output\n"+part.CodeExecutionResult.Output+"\n```")
					} else {
						// 过滤掉空行
						if part.Text != "\n" {
							texts = append(texts, part.Text)
						}
					}
				}
			}
			if len(toolCalls) > 0 {
				choice.Message.SetToolCalls(toolCalls)
				isToolCall = true
			}

			choice.Message.SetStringContent(strings.Join(texts, "\n"))

		}
		if candidate.FinishReason != nil {
			switch *candidate.FinishReason {
			case "STOP":
				choice.FinishReason = constant.FinishReasonStop
			case "MAX_TOKENS":
				choice.FinishReason = constant.FinishReasonLength
			default:
				choice.FinishReason = constant.FinishReasonContentFilter
			}
		}
		if isToolCall {
			choice.FinishReason = constant.FinishReasonToolCalls
		}

		fullTextResponse.Choices = append(fullTextResponse.Choices, choice)
	}
	return &fullTextResponse
}

func streamResponseGeminiChat2OpenAI(geminiResponse *GeminiChatResponse) (*dto.ChatCompletionsStreamResponse, bool) {
	choices := make([]dto.ChatCompletionsStreamResponseChoice, 0, len(geminiResponse.Candidates))
	isStop := false
	for _, candidate := range geminiResponse.Candidates {
		if candidate.FinishReason != nil && *candidate.FinishReason == "STOP" {
			isStop = true
			candidate.FinishReason = nil
		}
		choice := dto.ChatCompletionsStreamResponseChoice{
			Index: int(candidate.Index),
			Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
				Role: "assistant",
			},
		}
		var texts []string
		isTools := false
		if candidate.FinishReason != nil {
			// p := GeminiConvertFinishReason(*candidate.FinishReason)
			switch *candidate.FinishReason {
			case "STOP":
				choice.FinishReason = &constant.FinishReasonStop
			case "MAX_TOKENS":
				choice.FinishReason = &constant.FinishReasonLength
			default:
				choice.FinishReason = &constant.FinishReasonContentFilter
			}
		}
		for _, part := range candidate.Content.Parts {
			if part.FunctionCall != nil {
				isTools = true
				if call := getResponseToolCall(&part); call != nil {
					call.SetIndex(len(choice.Delta.ToolCalls))
					choice.Delta.ToolCalls = append(choice.Delta.ToolCalls, *call)
				}
			} else {
				if part.ExecutableCode != nil {
					texts = append(texts, "```"+part.ExecutableCode.Language+"\n"+part.ExecutableCode.Code+"\n```\n")
				} else if part.CodeExecutionResult != nil {
					texts = append(texts, "```output\n"+part.CodeExecutionResult.Output+"\n```\n")
				} else {
					if part.Text != "\n" {
						texts = append(texts, part.Text)
					}
				}
			}
		}
		choice.Delta.SetContentString(strings.Join(texts, "\n"))
		if isTools {
			choice.FinishReason = &constant.FinishReasonToolCalls
		}
		choices = append(choices, choice)
	}

	var response dto.ChatCompletionsStreamResponse
	response.Object = "chat.completion.chunk"
	response.Choices = choices
	return &response, isStop
}

func GeminiChatStreamHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.OpenAIErrorWithStatusCode, *dto.Usage) {
	// responseText := ""
	id := fmt.Sprintf("chatcmpl-%s", common.GetUUID())
	createAt := common.GetTimestamp()
	var usage = &dto.Usage{}

	helper.StreamScannerHandler(c, resp, info,
		func(data string) bool {
			var geminiResponse GeminiChatResponse
			err := json.Unmarshal([]byte(data), &geminiResponse)
			if err != nil {
				common.LogError(c, "error unmarshalling stream response: "+err.Error())
				return false
			}

			response, isStop := streamResponseGeminiChat2OpenAI(&geminiResponse)
			response.Id = id
			response.Created = createAt
			response.Model = info.UpstreamModelName
			// responseText += response.Choices[0].Delta.GetContentString()
			if geminiResponse.UsageMetadata.TotalTokenCount != 0 {
				usage.PromptTokens = geminiResponse.UsageMetadata.PromptTokenCount
				usage.CompletionTokens = geminiResponse.UsageMetadata.CandidatesTokenCount
			}
			err = helper.ObjectData(c, response)
			if err != nil {
				common.LogError(c, err.Error())
			}
			if isStop {
				response := helper.GenerateStopResponse(id, createAt, info.UpstreamModelName, constant.FinishReasonStop)
				helper.ObjectData(c, response)
			}
			return true
		})

	var response *dto.ChatCompletionsStreamResponse

	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	usage.PromptTokensDetails.TextTokens = usage.PromptTokens
	usage.CompletionTokenDetails.TextTokens = usage.CompletionTokens

	if info.ShouldIncludeUsage {
		response = helper.GenerateFinalUsageResponse(id, createAt, info.UpstreamModelName, *usage)
		err := helper.ObjectData(c, response)
		if err != nil {
			common.SysError("send final response failed: " + err.Error())
		}
	}
	helper.Done(c)
	//resp.Body.Close()
	return nil, usage
}

func GeminiChatHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.OpenAIErrorWithStatusCode, *dto.Usage) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return service.OpenAIErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return service.OpenAIErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	var geminiResponse GeminiChatResponse
	err = json.Unmarshal(responseBody, &geminiResponse)
	if err != nil {
		return service.OpenAIErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}
	if len(geminiResponse.Candidates) == 0 {
		common.SysError(fmt.Sprintf("no candidates returned: %s", string(responseBody)))
		return &dto.OpenAIErrorWithStatusCode{
			Error: dto.OpenAIError{
				Message: "No candidates returned",
				Type:    "server_error",
				Param:   "",
				Code:    500,
			},
			StatusCode: resp.StatusCode,
		}, nil
	}
	fullTextResponse := responseGeminiChat2OpenAI(&geminiResponse)
	fullTextResponse.Model = info.UpstreamModelName
	usage := dto.Usage{
		PromptTokens:     geminiResponse.UsageMetadata.PromptTokenCount,
		CompletionTokens: geminiResponse.UsageMetadata.CandidatesTokenCount,
		TotalTokens:      geminiResponse.UsageMetadata.TotalTokenCount,
	}
	usage.CompletionTokenDetails.ReasoningTokens = geminiResponse.UsageMetadata.ThoughtsTokenCount
	fullTextResponse.Usage = usage
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return service.OpenAIErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	return nil, &usage
}
