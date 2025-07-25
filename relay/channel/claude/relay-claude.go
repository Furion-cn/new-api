package claude

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/dto"
	relaycommon "one-api/relay/common"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/setting/model_setting"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func stopReasonClaude2OpenAI(reason string) string {
	switch reason {
	case "stop_sequence":
		return "stop"
	case "end_turn":
		return "stop"
	case "max_tokens":
		return "max_tokens"
	default:
		return reason
	}
}

func RequestOpenAI2ClaudeComplete(textRequest dto.GeneralOpenAIRequest) *ClaudeRequest {
	claudeRequest := ClaudeRequest{
		Model:         textRequest.Model,
		Prompt:        "",
		StopSequences: nil,
		Temperature:   textRequest.Temperature,
		TopP:          textRequest.TopP,
		TopK:          textRequest.TopK,
		Stream:        textRequest.Stream,
	}
	if claudeRequest.MaxTokensToSample == 0 {
		claudeRequest.MaxTokensToSample = 4096
	}
	prompt := ""
	for _, message := range textRequest.Messages {
		if message.Role == "user" {
			prompt += fmt.Sprintf("\n\nHuman: %s", message.Content)
		} else if message.Role == "assistant" {
			prompt += fmt.Sprintf("\n\nAssistant: %s", message.Content)
		} else if message.Role == "system" {
			if prompt == "" {
				prompt = message.StringContent()
			}
		}
	}
	prompt += "\n\nAssistant:"
	claudeRequest.Prompt = prompt
	return &claudeRequest
}

func RequestOpenAI2ClaudeMessage(c *gin.Context, textRequest dto.GeneralOpenAIRequest) (*ClaudeRequest, error) {
	claudeTools := make([]Tool, 0, len(textRequest.Tools))

	for _, tool := range textRequest.Tools {

		if params, ok := tool.Function.Parameters.(map[string]any); ok {
			claudeTool := Tool{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
			}
			claudeTool.InputSchema = make(map[string]interface{})
			claudeTool.InputSchema["type"] = params["type"].(string)
			claudeTool.InputSchema["properties"] = params["properties"]
			claudeTool.InputSchema["required"] = params["required"]
			for s, a := range params {
				if s == "type" || s == "properties" || s == "required" {
					continue
				}
				claudeTool.InputSchema[s] = a
			}
			claudeTools = append(claudeTools, claudeTool)
		}
	}

	claudeRequest := ClaudeRequest{
		Model:         textRequest.Model,
		MaxTokens:     textRequest.MaxTokens,
		StopSequences: nil,
		Temperature:   textRequest.Temperature,
		TopP:          textRequest.TopP,
		TopK:          textRequest.TopK,
		Stream:        textRequest.Stream,
		Tools:         claudeTools,
	}

	if claudeRequest.MaxTokens == 0 {
		claudeRequest.MaxTokens = uint(model_setting.GetClaudeSettings().GetDefaultMaxTokens(textRequest.Model))
	}

	if model_setting.GetClaudeSettings().ThinkingAdapterEnabled &&
		strings.HasSuffix(textRequest.Model, "-thinking") {

		// 因为BudgetTokens 必须大于1024
		if claudeRequest.MaxTokens < 1280 {
			claudeRequest.MaxTokens = 1280
		}
		claudeRequest.Thinking = &Thinking{Type: "enabled"}
		// 支持用户覆盖 budget tokens
		if textRequest.Thinking != nil {
			// claude 对于 budget tokens 最小限制为 1024
			if textRequest.Thinking.BudgetTokens < 1024 {
				textRequest.Thinking.BudgetTokens = 1024
				common.LogInfo(c, fmt.Sprintf("传入的 budget tokens %d 小于 1024，已设为 1024 ", textRequest.Thinking.BudgetTokens))
			}
			claudeRequest.Thinking.BudgetTokens = textRequest.Thinking.BudgetTokens
			common.LogInfo(c, fmt.Sprintf("用户自定义 budget tokens 长度: %d", claudeRequest.Thinking.BudgetTokens))
		} else {
			// BudgetTokens 为 max_tokens 的 80%
			claudeRequest.Thinking.BudgetTokens = int(float64(claudeRequest.MaxTokens) * model_setting.GetClaudeSettings().ThinkingAdapterBudgetTokensPercentage)
			common.LogInfo(c, fmt.Sprintf("budget tokens 使用系统 max tokens 的 80%%: %d", claudeRequest.Thinking.BudgetTokens))
		}

		// TODO: 临时处理
		// https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking#important-considerations-when-using-extended-thinking
		claudeRequest.TopP = 0
		claudeRequest.Temperature = common.GetPointer[float64](1.0)
		claudeRequest.Model = strings.TrimSuffix(textRequest.Model, "-thinking")
	}

	if textRequest.Stop != nil {
		// stop maybe string/array string, convert to array string
		switch textRequest.Stop.(type) {
		case string:
			claudeRequest.StopSequences = []string{textRequest.Stop.(string)}
		case []interface{}:
			stopSequences := make([]string, 0)
			for _, stop := range textRequest.Stop.([]interface{}) {
				stopSequences = append(stopSequences, stop.(string))
			}
			claudeRequest.StopSequences = stopSequences
		}
	}
	formatMessages := make([]dto.Message, 0)
	lastMessage := dto.Message{
		Role: "tool",
	}
	for i, message := range textRequest.Messages {
		if message.Role == "" {
			textRequest.Messages[i].Role = "user"
		}
		fmtMessage := dto.Message{
			Role:    message.Role,
			Content: message.Content,
		}
		if message.Role == "tool" {
			fmtMessage.ToolCallId = message.ToolCallId
		}
		if message.Role == "assistant" && message.ToolCalls != nil {
			fmtMessage.ToolCalls = message.ToolCalls
		}
		if lastMessage.Role == message.Role && lastMessage.Role != "tool" {
			if lastMessage.IsStringContent() && message.IsStringContent() {
				content, _ := json.Marshal(strings.Trim(fmt.Sprintf("%s %s", lastMessage.StringContent(), message.StringContent()), "\""))
				fmtMessage.Content = content
				// delete last message
				formatMessages = formatMessages[:len(formatMessages)-1]
			}
		}
		if fmtMessage.Content == nil {
			content, _ := json.Marshal("...")
			fmtMessage.Content = content
		}
		formatMessages = append(formatMessages, fmtMessage)
		lastMessage = fmtMessage
	}

	claudeMessages := make([]ClaudeMessage, 0)
	isFirstMessage := true
	for _, message := range formatMessages {
		if message.Role == "system" {
			if message.IsStringContent() {
				claudeRequest.System = message.StringContent()
			} else {
				contents := message.ParseContent()
				content := ""
				for _, ctx := range contents {
					if ctx.Type == "text" {
						content += ctx.Text
					}
				}
				claudeRequest.System = content
			}
		} else {
			if isFirstMessage {
				isFirstMessage = false
				if message.Role != "user" {
					// fix: first message is assistant, add user message
					claudeMessage := ClaudeMessage{
						Role: "user",
						Content: []ClaudeMediaMessage{
							{
								Type: "text",
								Text: "...",
							},
						},
					}
					claudeMessages = append(claudeMessages, claudeMessage)
				}
			}
			claudeMessage := ClaudeMessage{
				Role: message.Role,
			}
			if message.Role == "tool" {
				if len(claudeMessages) > 0 && claudeMessages[len(claudeMessages)-1].Role == "user" {
					lastMessage := claudeMessages[len(claudeMessages)-1]
					if content, ok := lastMessage.Content.(string); ok {
						lastMessage.Content = []ClaudeMediaMessage{
							{
								Type: "text",
								Text: content,
							},
						}
					}
					lastMessage.Content = append(lastMessage.Content.([]ClaudeMediaMessage), ClaudeMediaMessage{
						Type:      "tool_result",
						ToolUseId: message.ToolCallId,
						Content:   message.StringContent(),
					})
					claudeMessages[len(claudeMessages)-1] = lastMessage
					continue
				} else {
					claudeMessage.Role = "user"
					claudeMessage.Content = []ClaudeMediaMessage{
						{
							Type:      "tool_result",
							ToolUseId: message.ToolCallId,
							Content:   message.StringContent(),
						},
					}
				}
			} else if message.IsStringContent() && message.ToolCalls == nil {
				claudeMessage.Content = message.StringContent()
			} else {
				claudeMediaMessages := make([]ClaudeMediaMessage, 0)
				for _, mediaMessage := range message.ParseContent() {
					claudeMediaMessage := ClaudeMediaMessage{
						Type: mediaMessage.Type,
					}
					if mediaMessage.Type == "text" {
						claudeMediaMessage.Text = mediaMessage.Text
					} else {
						imageUrl := mediaMessage.ImageUrl.(dto.MessageImageUrl)
						claudeMediaMessage.Type = "image"
						claudeMediaMessage.Source = &ClaudeMessageSource{
							Type: "base64",
						}
						// 判断是否是url
						if strings.HasPrefix(imageUrl.Url, "http") {
							// 是url，获取图片的类型和base64编码的数据
							fileData, err := service.GetFileBase64FromUrl(imageUrl.Url)
							if err != nil {
								return nil, fmt.Errorf("get file base64 from url failed: %s", err.Error())
							}
							claudeMediaMessage.Source.MediaType = fileData.MimeType
							claudeMediaMessage.Source.Data = fileData.Base64Data
						} else {
							_, format, base64String, err := service.DecodeBase64ImageData(imageUrl.Url)
							if err != nil {
								return nil, err
							}
							claudeMediaMessage.Source.MediaType = "image/" + format
							claudeMediaMessage.Source.Data = base64String
						}
					}
					claudeMediaMessages = append(claudeMediaMessages, claudeMediaMessage)
				}
				if message.ToolCalls != nil {
					for _, toolCall := range message.ParseToolCalls() {
						inputObj := make(map[string]any)
						if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &inputObj); err != nil {
							common.SysError("tool call function arguments is not a map[string]any: " + fmt.Sprintf("%v", toolCall.Function.Arguments))
							continue
						}
						claudeMediaMessages = append(claudeMediaMessages, ClaudeMediaMessage{
							Type:  "tool_use",
							Id:    toolCall.ID,
							Name:  toolCall.Function.Name,
							Input: inputObj,
						})
					}
				}
				claudeMessage.Content = claudeMediaMessages
			}
			claudeMessages = append(claudeMessages, claudeMessage)
		}
	}
	claudeRequest.Prompt = ""
	claudeRequest.Messages = claudeMessages
	return &claudeRequest, nil
}

func StreamResponseClaude2OpenAI(reqMode int, claudeResponse *ClaudeResponse) (*dto.ChatCompletionsStreamResponse, *ClaudeUsage) {
	var response dto.ChatCompletionsStreamResponse
	var claudeUsage *ClaudeUsage
	response.Object = "chat.completion.chunk"
	response.Model = claudeResponse.Model
	response.Choices = make([]dto.ChatCompletionsStreamResponseChoice, 0)
	tools := make([]dto.ToolCallResponse, 0)
	var choice dto.ChatCompletionsStreamResponseChoice
	if reqMode == RequestModeCompletion {
		choice.Delta.SetContentString(claudeResponse.Completion)
		finishReason := stopReasonClaude2OpenAI(claudeResponse.StopReason)
		if finishReason != "null" {
			choice.FinishReason = &finishReason
		}
	} else {
		if claudeResponse.Type == "message_start" {
			response.Id = claudeResponse.Message.Id
			response.Model = claudeResponse.Message.Model
			claudeUsage = &claudeResponse.Message.Usage
			choice.Delta.SetContentString("")
			choice.Delta.Role = "assistant"
		} else if claudeResponse.Type == "content_block_start" {
			if claudeResponse.ContentBlock != nil {
				//choice.Delta.SetContentString(claudeResponse.ContentBlock.Text)
				if claudeResponse.ContentBlock.Type == "tool_use" {
					tools = append(tools, dto.ToolCallResponse{
						ID:   claudeResponse.ContentBlock.Id,
						Type: "function",
						Function: dto.FunctionResponse{
							Name:      claudeResponse.ContentBlock.Name,
							Arguments: "",
						},
					})
				}
			} else {
				return nil, nil
			}
		} else if claudeResponse.Type == "content_block_delta" {
			if claudeResponse.Delta != nil {
				choice.Index = claudeResponse.Index
				choice.Delta.SetContentString(claudeResponse.Delta.Text)
				switch claudeResponse.Delta.Type {
				case "input_json_delta":
					tools = append(tools, dto.ToolCallResponse{
						Function: dto.FunctionResponse{
							Arguments: claudeResponse.Delta.PartialJson,
						},
					})
				case "signature_delta":
					// 加密的不处理
					signatureContent := "\n"
					choice.Delta.ReasoningContent = &signatureContent
				case "thinking_delta":
					thinkingContent := claudeResponse.Delta.Thinking
					choice.Delta.ReasoningContent = &thinkingContent
				}
			}
		} else if claudeResponse.Type == "message_delta" {
			finishReason := stopReasonClaude2OpenAI(*claudeResponse.Delta.StopReason)
			if finishReason != "null" {
				choice.FinishReason = &finishReason
			}
			claudeUsage = &claudeResponse.Usage
		} else if claudeResponse.Type == "message_stop" {
			return nil, nil
		} else {
			return nil, nil
		}
	}
	if claudeUsage == nil {
		claudeUsage = &ClaudeUsage{}
	}
	if len(tools) > 0 {
		choice.Delta.Content = nil // compatible with other OpenAI derivative applications, like LobeOpenAICompatibleFactory ...
		choice.Delta.ToolCalls = tools
	}
	response.Choices = append(response.Choices, choice)

	return &response, claudeUsage
}

func ResponseClaude2OpenAI(reqMode int, claudeResponse *ClaudeResponse) *dto.OpenAITextResponse {
	choices := make([]dto.OpenAITextResponseChoice, 0)
	fullTextResponse := dto.OpenAITextResponse{
		Id:      fmt.Sprintf("chatcmpl-%s", common.GetUUID()),
		Object:  "chat.completion",
		Created: common.GetTimestamp(),
	}
	var responseText string
	if len(claudeResponse.Content) > 0 {
		responseText = claudeResponse.Content[0].Text
	}
	tools := make([]dto.ToolCallResponse, 0)
	thinkingContent := ""

	if reqMode == RequestModeCompletion {
		content, _ := json.Marshal(strings.TrimPrefix(claudeResponse.Completion, " "))
		choice := dto.OpenAITextResponseChoice{
			Index: 0,
			Message: dto.Message{
				Role:    "assistant",
				Content: content,
				Name:    nil,
			},
			FinishReason: stopReasonClaude2OpenAI(claudeResponse.StopReason),
		}
		choices = append(choices, choice)
	} else {
		fullTextResponse.Id = claudeResponse.Id
		for _, message := range claudeResponse.Content {
			switch message.Type {
			case "tool_use":
				args, _ := json.Marshal(message.Input)
				tools = append(tools, dto.ToolCallResponse{
					ID:   message.Id,
					Type: "function", // compatible with other OpenAI derivative applications
					Function: dto.FunctionResponse{
						Name:      message.Name,
						Arguments: string(args),
					},
				})
			case "thinking":
				// 加密的不管， 只输出明文的推理过程
				thinkingContent = message.Thinking
			case "text":
				responseText = message.Text
			}
		}
	}
	choice := dto.OpenAITextResponseChoice{
		Index: 0,
		Message: dto.Message{
			Role: "assistant",
		},
		FinishReason: stopReasonClaude2OpenAI(claudeResponse.StopReason),
	}
	choice.SetStringContent(responseText)
	if len(tools) > 0 {
		choice.Message.SetToolCalls(tools)
	}
	choice.Message.ReasoningContent = thinkingContent
	fullTextResponse.Model = claudeResponse.Model
	choices = append(choices, choice)
	fullTextResponse.Choices = choices
	return &fullTextResponse
}

func ClaudeStreamHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, requestMode int) (*dto.OpenAIErrorWithStatusCode, *dto.Usage) {
	responseId := fmt.Sprintf("chatcmpl-%s", common.GetUUID())
	var usage *dto.Usage
	usage = &dto.Usage{}
	responseText := ""
	createdTime := common.GetTimestamp()

	helper.StreamScannerHandler(c, resp, info, func(data string) bool {
		var claudeResponse ClaudeResponse
		err := json.Unmarshal([]byte(data), &claudeResponse)
		if err != nil {
			common.SysError("error unmarshalling stream response: " + err.Error())
			return true
		}

		response, claudeUsage := StreamResponseClaude2OpenAI(requestMode, &claudeResponse)
		if response == nil {
			return true
		}
		if requestMode == RequestModeCompletion {
			responseText += claudeResponse.Completion
			responseId = response.Id
		} else {
			if claudeResponse.Type == "message_start" {
				// message_start, 获取usage
				responseId = claudeResponse.Message.Id
				info.UpstreamModelName = claudeResponse.Message.Model
				usage.PromptTokens = claudeUsage.InputTokens
			} else if claudeResponse.Type == "content_block_delta" {
				responseText += claudeResponse.Delta.Text
			} else if claudeResponse.Type == "message_delta" {
				usage.CompletionTokens = claudeUsage.OutputTokens
				usage.TotalTokens = claudeUsage.InputTokens + claudeUsage.OutputTokens
			} else if claudeResponse.Type == "content_block_start" {
				return true
			} else {
				return true
			}
		}
		//response.Id = responseId
		response.Id = responseId
		response.Created = createdTime
		response.Model = info.UpstreamModelName

		err = helper.ObjectData(c, response)
		if err != nil {
			common.LogError(c, "send_stream_response_failed: "+err.Error())
		}
		return true
	})

	if requestMode == RequestModeCompletion {
		usage, _ = service.ResponseText2Usage(responseText, info.UpstreamModelName, info.PromptTokens)
	} else {
		if usage.PromptTokens == 0 {
			usage.PromptTokens = info.PromptTokens
		}
		if usage.CompletionTokens == 0 {
			usage, _ = service.ResponseText2Usage(responseText, info.UpstreamModelName, usage.PromptTokens)
		}
	}
	if info.ShouldIncludeUsage {
		response := helper.GenerateFinalUsageResponse(responseId, createdTime, info.UpstreamModelName, *usage)
		err := helper.ObjectData(c, response)
		if err != nil {
			common.SysError("send final response failed: " + err.Error())
		}
	}
	helper.Done(c)
	//resp.Body.Close()
	return nil, usage
}

func ClaudeHandler(c *gin.Context, resp *http.Response, requestMode int, info *relaycommon.RelayInfo) (*dto.OpenAIErrorWithStatusCode, *dto.Usage) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return service.OpenAIErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return service.OpenAIErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	var claudeResponse ClaudeResponse
	err = json.Unmarshal(responseBody, &claudeResponse)
	if err != nil {
		return service.OpenAIErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}
	if claudeResponse.Error.Type != "" {
		return &dto.OpenAIErrorWithStatusCode{
			Error: dto.OpenAIError{
				Message: claudeResponse.Error.Message,
				Type:    claudeResponse.Error.Type,
				Param:   "",
				Code:    claudeResponse.Error.Type,
			},
			StatusCode: resp.StatusCode,
		}, nil
	}
	fullTextResponse := ResponseClaude2OpenAI(requestMode, &claudeResponse)
	completionTokens, err := service.CountTextToken(claudeResponse.Completion, info.OriginModelName)
	if err != nil {
		return service.OpenAIErrorWrapper(err, "count_token_text_failed", http.StatusInternalServerError), nil
	}
	usage := dto.Usage{}
	if requestMode == RequestModeCompletion {
		usage.PromptTokens = info.PromptTokens
		usage.CompletionTokens = completionTokens
		usage.TotalTokens = info.PromptTokens + completionTokens
	} else {
		usage.PromptTokens = claudeResponse.Usage.InputTokens
		usage.CompletionTokens = claudeResponse.Usage.OutputTokens
		usage.TotalTokens = claudeResponse.Usage.InputTokens + claudeResponse.Usage.OutputTokens
	}
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

func ClaudeMessage2OpenAIRequest(claudeReq *ClaudeRequest) (*dto.GeneralOpenAIRequest, error) {
	openaiReq := &dto.GeneralOpenAIRequest{
		Model:         claudeReq.Model,
		MaxTokens:     claudeReq.MaxTokens,
		Temperature:   claudeReq.Temperature,
		TopP:          claudeReq.TopP,
		Stream:        claudeReq.Stream,
		StreamOptions: &dto.StreamOptions{IncludeUsage: true},
	}

	if claudeReq.Thinking != nil {
		openaiReq.Thinking = &dto.ThinkingOptions{
			Type:         claudeReq.Thinking.Type,
			BudgetTokens: claudeReq.Thinking.BudgetTokens,
		}
	}

	if claudeReq.Tools != nil {
		openaiTools := make([]dto.ToolCallRequest, 0)

		switch tools := claudeReq.Tools.(type) {
		case []Tool:
			for _, claudeTool := range tools {
				params := make(map[string]interface{}, 3)
				for _, key := range []string{"type", "properties", "required"} {
					if val, exist := claudeTool.InputSchema[key]; exist {
						params[key] = val
					}
				}

				openaiTools = append(openaiTools, dto.ToolCallRequest{
					Type: "function",
					Function: dto.FunctionRequest{
						Name:        claudeTool.Name,
						Description: claudeTool.Description,
						Parameters:  params,
					},
				})
			}

		case []interface{}: // 处理通用类型
			for _, rawTool := range tools {
				tool, ok := rawTool.(map[string]interface{})
				if !ok {
					continue
				}

				// 检查是否有 input_schema 字段
				if _, hasInputSchema := tool["input_schema"].(map[string]interface{}); !hasInputSchema {
					// 如果没有 input_schema 字段，直接使用原始格式
					maxUses := 0
					if maxUsesVal, exists := tool["max_uses"]; exists {
						switch v := maxUsesVal.(type) {
						case int:
							maxUses = v
						case float64:
							maxUses = int(v)
						}
					}
					openaiTools = append(openaiTools, dto.ToolCallRequest{
						Type:    tool["type"].(string),
						Name:    tool["name"].(string),
						MaxUses: maxUses,
					})
				} else {
					// 如果没有 function 字段，按照 Claude 格式处理
					name, _ := tool["name"].(string)
					desc, _ := tool["description"].(string)
					schema, _ := tool["input_schema"].(map[string]interface{})

					params := make(map[string]interface{}, 3)
					if schema != nil {
						for _, key := range []string{"type", "properties", "required"} {
							if val, exist := schema[key]; exist {
								params[key] = val
							}
						}
					}

					openaiTools = append(openaiTools, dto.ToolCallRequest{
						Type: "function",
						Function: dto.FunctionRequest{
							Name:        name,
							Description: desc,
							Parameters:  params,
						},
					})
				}
			}
		}

		openaiReq.Tools = openaiTools
	}

	if claudeReq.System != "" {
		systemMsg := dto.Message{
			Role:    "system",
			Content: json.RawMessage([]byte(strconv.Quote(claudeReq.System))),
		}
		openaiReq.Messages = append(openaiReq.Messages, systemMsg)
	}

	// 多模态
	for _, claudeMsg := range claudeReq.Messages {
		openaiMsg := dto.Message{Role: claudeMsg.Role}

		switch content := claudeMsg.Content.(type) {
		case string: // 纯文本
			openaiMsg.SetStringContent(content)

		case []ClaudeMediaMessage: // 复杂消息类型
			var mediaContents []dto.MediaContent
			var toolCalls []dto.ToolCallRequest

			for _, media := range content {
				switch media.Type {
				case "text":
					mediaContents = append(mediaContents, dto.MediaContent{
						Type: dto.ContentTypeText,
						Text: media.Text,
					})

				case "image":
					if media.Source != nil {
						mediaContents = append(mediaContents, dto.MediaContent{
							Type: dto.ContentTypeImageURL,
							ImageUrl: dto.MessageImageUrl{
								Url:    fmt.Sprintf("data:%s;base64,%s", media.Source.MediaType, media.Source.Data),
								Detail: "auto",
							},
						})
					}

				case "tool_use":
					args, _ := json.Marshal(media.Input)
					toolCalls = append(toolCalls, dto.ToolCallRequest{
						ID: media.Id,
						Function: dto.FunctionRequest{
							Name:      media.Name,
							Arguments: string(args),
						},
					})

				case "tool_result":
					openaiMsg.ToolCallId = media.ToolUseId
					openaiMsg.SetStringContent(media.Content)
				}
			}

			if len(toolCalls) > 0 {
				toolCallData, _ := json.Marshal(toolCalls)
				openaiMsg.ToolCalls = toolCallData
			}

			if len(mediaContents) > 0 {
				openaiMsg.SetMediaContent(mediaContents)
			}
		}

		openaiReq.Messages = append(openaiReq.Messages, openaiMsg)
	}

	if len(claudeReq.StopSequences) > 0 {
		if len(claudeReq.StopSequences) == 1 {
			openaiReq.Stop = claudeReq.StopSequences[0]
		} else {
			stopSeq := make([]string, len(claudeReq.StopSequences))
			copy(stopSeq, claudeReq.StopSequences)
			openaiReq.Stop = stopSeq
		}
	}

	if strings.Contains(claudeReq.Model, "claude-3") {
		openaiReq.ResponseFormat = &dto.ResponseFormat{
			Type: "json_object",
			JsonSchema: &dto.FormatJsonSchema{
				Schema: map[string]interface{}{
					"type":       "object",
					"properties": make(map[string]interface{}),
				},
			},
		}
	}

	return openaiReq, nil
}
