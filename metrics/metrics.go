package metrics

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	Namespace = "new_api"
)

func RegisterMetrics(registry prometheus.Registerer) {
	// channel
	registry.MustRegister(relayRequestTotalCounter)
	registry.MustRegister(relayRequestSuccessCounter)
	registry.MustRegister(relayRequestFailedCounter)
	registry.MustRegister(relayRequestRetryCounter)
	registry.MustRegister(relayRequestDurationObsever)
	// e2e
	registry.MustRegister(relayRequestE2ETotalCounter)
	registry.MustRegister(relayRequestE2ESuccessCounter)
	registry.MustRegister(relayRequestE2EFailedCounter)
	registry.MustRegister(relayRequestE2EDurationObsever)
	// batch
	registry.MustRegister(batchRequestCounter)
	registry.MustRegister(batchRequestDurationObsever)
	// token metrics
	registry.MustRegister(inputTokensCounter)
	registry.MustRegister(outputTokensCounter)
	registry.MustRegister(cacheHitTokensCounter)
	registry.MustRegister(inferenceTokensCounter)
	registry.MustRegister(promptTokensZeroOrNegativeCounter)
	registry.MustRegister(completionTokensZeroOrNegativeCounter)
	registry.MustRegister(thinkingTokensZeroOrNegativeCounter)
	// error log metrics
	registry.MustRegister(errorLogCounter)
	// consume log traffic metrics
	registry.MustRegister(consumeLogTrafficTotalCounter)
	registry.MustRegister(consumeLogTrafficFailedCounter)
	registry.MustRegister(consumeLogTrafficSuccessCounter)
}

var (
	relayRequestTotalCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: Namespace,
			Name:      "relay_request_total",
			Help:      "Total number of relay request total",
		}, []string{"channel", "channel_name", "tag", "base_url", "model", "group", "user_id", "user_name"})
	relayRequestSuccessCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: Namespace,
			Name:      "relay_request_success",
			Help:      "Total number of relay request success",
		}, []string{"channel", "channel_name", "tag", "base_url", "model", "group", "code", "user_id", "user_name"})

	relayRequestFailedCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: Namespace,
			Name:      "relay_request_failed",
			Help:      "Total number of relay request failed",
		}, []string{"channel", "channel_name", "tag", "base_url", "model", "group", "code", "user_id", "user_name", "error_message"})
	relayRequestRetryCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: Namespace,
			Name:      "relay_request_retry",
			Help:      "Total number of relay request retry",
		}, []string{"channel", "channel_name", "tag", "base_url", "model", "group", "user_id", "user_name"})
	relayRequestDurationObsever = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: Namespace,
			Name:      "relay_request_duration",
			Help:      "Duration of relay request",
			Buckets:   prometheus.ExponentialBuckets(1, 2, 12),
		},
		[]string{"channel", "channel_name", "tag", "base_url", "model", "group", "user_id", "user_name"},
	)
	relayRequestE2ETotalCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: Namespace,
			Name:      "relay_request_e2e_total",
			Help:      "Total number of relay request e2e total",
		}, []string{"channel", "channel_name", "model", "group", "token_key", "token_name", "user_id", "user_name"})
	relayRequestE2ESuccessCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: Namespace,
			Name:      "relay_request_e2e_success",
			Help:      "Total number of relay request e2e success",
		}, []string{"channel", "channel_name", "model", "group", "token_key", "token_name", "user_id", "user_name"})
	relayRequestE2EFailedCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: Namespace,
			Name:      "relay_request_e2e_failed",
			Help:      "Total number of relay request e2e failed",
		}, []string{"channel", "channel_name", "model", "group", "code", "token_key", "token_name", "user_id", "user_name", "error_message"})
	relayRequestE2EDurationObsever = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: Namespace,
			Name:      "relay_request_e2e_duration",
			Help:      "Duration of relay request e2e",
			Buckets:   prometheus.ExponentialBuckets(1, 2, 12),
		},
		[]string{"channel", "channel_name", "model", "group", "token_key", "token_name", "user_id", "user_name", "code"},
	)
	// Batch request metrics
	batchRequestCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: Namespace,
			Name:      "batch_request_total",
			Help:      "Total number of batch requests by status code",
		}, []string{"channel", "channel_name", "tag", "base_url", "model", "group", "code", "retry_header", "user_id", "user_name"})
	batchRequestDurationObsever = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: Namespace,
			Name:      "batch_request_duration",
			Help:      "Duration of batch request",
			Buckets:   prometheus.ExponentialBuckets(1, 2, 12),
		},
		[]string{"channel", "channel_name", "tag", "base_url", "model", "group", "code", "retry_header", "user_id", "user_name"},
	)
	// Token metrics
	inputTokensCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: Namespace,
			Name:      "input_tokens_total",
			Help:      "Total number of input tokens processed",
		}, []string{"channel", "channel_name", "model", "group", "user_id", "user_name", "token_name"})

	outputTokensCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: Namespace,
			Name:      "output_tokens_total",
			Help:      "Total number of output tokens generated",
		}, []string{"channel", "channel_name", "model", "group", "user_id", "user_name", "token_name"})

	cacheHitTokensCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: Namespace,
			Name:      "cache_hit_tokens_total",
			Help:      "Total number of tokens served from cache",
		}, []string{"channel", "channel_name", "model", "group", "user_id", "user_name", "token_name"})

	inferenceTokensCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: Namespace,
			Name:      "inference_tokens_total",
			Help:      "Total number of tokens processed during inference",
		}, []string{"channel", "channel_name", "model", "group", "user_id", "user_name", "token_name"})

	// Zero or negative token counters
	promptTokensZeroOrNegativeCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: Namespace,
			Name:      "prompt_tokens_zero_or_negative_total",
			Help:      "Total number of times prompt tokens are zero or negative",
		}, []string{"channel", "channel_name", "model", "group", "user_id", "user_name", "token_name"})

	completionTokensZeroOrNegativeCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: Namespace,
			Name:      "completion_tokens_zero_or_negative_total",
			Help:      "Total number of times completion tokens are zero or negative",
		}, []string{"channel", "channel_name", "model", "group", "user_id", "user_name", "token_name"})

	thinkingTokensZeroOrNegativeCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: Namespace,
			Name:      "thinking_tokens_zero_or_negative_total",
			Help:      "Total number of times thinking tokens are zero or negative",
		}, []string{"channel", "channel_name", "model", "group", "user_id", "user_name", "token_name"})

	errorLogCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: Namespace,
			Name:      "error_log_total",
			Help:      "Total number of error logs",
		}, []string{"channel", "channel_name", "error_code", "error_type", "model", "group", "token_name", "user_id", "user_name"})

	// Consume log traffic metrics
	consumeLogTrafficTotalCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: Namespace,
			Name:      "consume_log_traffic_total",
			Help:      "Total traffic count for consume logs",
		}, []string{"channel", "channel_name", "model", "group", "user_id", "user_name", "token_name"})

	consumeLogTrafficFailedCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: Namespace,
			Name:      "consume_log_traffic_failed_total",
			Help:      "Total failed traffic count for consume logs",
		}, []string{"channel", "channel_name", "model", "group", "user_id", "user_name", "token_name", "error_code"})

	consumeLogTrafficSuccessCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: Namespace,
			Name:      "consume_log_traffic_success_total",
			Help:      "Total successful traffic count for consume logs",
		}, []string{"channel", "channel_name", "model", "group", "user_id", "user_name", "token_name"})
)

func IncrementRelayRequestTotalCounter(channel, channelName, tag, baseURL, model, group, userId, userName string, add float64) {
	relayRequestTotalCounter.WithLabelValues(channel, channelName, tag, baseURL, model, group, userId, userName).Add(add)
}

func IncrementRelayRequestSuccessCounter(channel, channelName, tag, baseURL, model, group, statusCode, userId, userName string, add float64) {
	relayRequestSuccessCounter.WithLabelValues(channel, channelName, tag, baseURL, model, group, statusCode, userId, userName).Add(add)
}

func IncrementRelayRequestFailedCounter(channel, channelName, tag, baseURL, model, group, code, userId, userName, errorMessage string, add float64) {
	errorMessage = errorMessageToCode(errorMessage)
	relayRequestFailedCounter.WithLabelValues(channel, channelName, tag, baseURL, model, group, code, userId, userName, errorMessage).Add(add)
}

func IncrementRelayRetryCounter(channel, channelName, tag, baseURL, model, group, userId, userName string, add float64) {
	relayRequestRetryCounter.WithLabelValues(channel, channelName, tag, baseURL, model, group, userId, userName).Add(add)
}

func ObserveRelayRequestDuration(channel, channelName, tag, baseURL, model, group, userId, userName string, duration float64) {
	relayRequestDurationObsever.WithLabelValues(channel, channelName, tag, baseURL, model, group, userId, userName).Observe(duration)
}

func IncrementRelayRequestE2ETotalCounter(channel, channelName, model, group, tokenKey, tokenName, userId, userName string, add float64) {
	relayRequestE2ETotalCounter.WithLabelValues(channel, channelName, model, group, tokenKey, tokenName, userId, userName).Add(add)
}

func IncrementRelayRequestE2ESuccessCounter(channel, channelName, model, group, tokenKey, tokenName, userId, userName string, add float64) {
	relayRequestE2ESuccessCounter.WithLabelValues(channel, channelName, model, group, tokenKey, tokenName, userId, userName).Add(add)
}

func IncrementRelayRequestE2EFailedCounter(channel, channelName, model, group, code, tokenKey, tokenName, userId, userName, errorMessage string, add float64) {
	errorMessage = errorMessageToCode(errorMessage)
	relayRequestE2EFailedCounter.WithLabelValues(channel, channelName, model, group, code, tokenKey, tokenName, userId, userName, errorMessage).Add(add)
}

func ObserveRelayRequestE2EDuration(channel, channelName, model, group, tokenKey, tokenName, userId, userName, code string, duration float64) {
	relayRequestE2EDurationObsever.WithLabelValues(channel, channelName, model, group, tokenKey, tokenName, userId, userName, code).Observe(duration)
}

// Batch request metrics functions
func IncrementBatchRequestCounter(channel, channelName, tag, baseURL, model, group, code, retryHeader, userId, userName string, add float64) {
	batchRequestCounter.WithLabelValues(channel, channelName, tag, baseURL, model, group, code, retryHeader, userId, userName).Add(add)
}

func ObserveBatchRequestDuration(channel, channelName, tag, baseURL, model, group, code, retryHeader, userId, userName string, duration float64) {
	batchRequestDurationObsever.WithLabelValues(channel, channelName, tag, baseURL, model, group, code, retryHeader, userId, userName).Observe(duration)
}

// Token metrics functions
func IncrementInputTokens(channel, channelName, model, group, userId, userName, tokenName string, add float64) {
	inputTokensCounter.WithLabelValues(channel, channelName, model, group, userId, userName, tokenName).Add(add)
}

func IncrementOutputTokens(channel, channelName, model, group, userId, userName, tokenName string, add float64) {
	outputTokensCounter.WithLabelValues(channel, channelName, model, group, userId, userName, tokenName).Add(add)
}

func IncrementCacheHitTokens(channel, channelName, model, group, userId, userName, tokenName string, add float64) {
	cacheHitTokensCounter.WithLabelValues(channel, channelName, model, group, userId, userName, tokenName).Add(add)
}

func IncrementInferenceTokens(channel, channelName, model, group, userId, userName, tokenName string, add float64) {
	inferenceTokensCounter.WithLabelValues(channel, channelName, model, group, userId, userName, tokenName).Add(add)
}

// Zero or negative token metrics functions
func IncrementPromptTokensZeroOrNegative(channel, channelName, model, group, userId, userName, tokenName string, add float64) {
	promptTokensZeroOrNegativeCounter.WithLabelValues(channel, channelName, model, group, userId, userName, tokenName).Add(add)
}

func IncrementCompletionTokensZeroOrNegative(channel, channelName, model, group, userId, userName, tokenName string, add float64) {
	completionTokensZeroOrNegativeCounter.WithLabelValues(channel, channelName, model, group, userId, userName, tokenName).Add(add)
}

func IncrementThinkingTokensZeroOrNegative(channel, channelName, model, group, userId, userName, tokenName string, add float64) {
	thinkingTokensZeroOrNegativeCounter.WithLabelValues(channel, channelName, model, group, userId, userName, tokenName).Add(add)
}

// Error log metrics function
func IncrementErrorLog(channel, channelName, errorCode, errorType, model, group, tokenName, userId, userName string, add float64) {
	errorLogCounter.WithLabelValues(channel, channelName, errorCode, errorType, model, group, tokenName, userId, userName).Add(add)
}

// Consume log traffic metrics functions
func IncrementConsumeLogTrafficTotal(channel, channelName, model, group, userId, userName, tokenName string, add float64) {
	consumeLogTrafficTotalCounter.WithLabelValues(channel, channelName, model, group, userId, userName, tokenName).Add(add)
}

func IncrementConsumeLogTrafficFailed(channel, channelName, model, group, userId, userName, tokenName, errorCode string, add float64) {
	consumeLogTrafficFailedCounter.WithLabelValues(channel, channelName, model, group, userId, userName, tokenName, errorCode).Add(add)
}

func IncrementConsumeLogTrafficSuccess(channel, channelName, model, group, userId, userName, tokenName string, add float64) {
	consumeLogTrafficSuccessCounter.WithLabelValues(channel, channelName, model, group, userId, userName, tokenName).Add(add)
}

/*
cat  oneapi-20251112155737.log  | grep '\[ERR\]' | grep -v -E 'write: connection timed out|The caller does not have permission|do request failed|no candidates returned|has been suspended|The caller does not havepermission|Quota exceeded for metric|Resource has been exhausted|The model is overloaded|bad_response_status_code|当前分组上游负载已饱和|An internal error has occurred|have exceeded thecall rate limit for your current AIServices S0 pricing tier|upstream error with status 408|failed to get model resp|The operation was timeout|exceeded for UserConcurrentRequests. Please wait|API Key not found|context deadline exceeded (Client.Timeout exceeded while awaiting headers)|无可用渠道（distributor）|API key expired. Please renew the API key.|API key not valid. Please pass a valid API key.|该令牌状态不可用|write: broken pipe|Internal error encountered|Quota exceeded for quota metric |Client.Timeout exceeded while awaiting headers|Your API key was reported as leaked. Please use another API key.|err mess is \{\}|Too many tokens, please wait before trying again.|Generative Language API has not been used in project|failed to record log|Failed to unmarshal response: invalid character|total tokens is 0|fail to decode image config|error response body is empty|You exceeded your current quota, please check your plan and billing details|无效的令牌|read: connection reset by peer|无可用渠道|must be followed by tool messages responding to each|Content Exists Risk|Please reducethe length of the messages or completion|Too many requests, please wait before trying again.'
*/
func errorMessageToCode(errorMessage string) string {
	switch {
	case errorMessage == "":
		errorMessage = "none"
	case strings.Contains(errorMessage, "write: connection timed out"):
		errorMessage = "connection_timeout"
	case strings.Contains(errorMessage, "do request failed"):
		errorMessage = "do_request_failed"
	case strings.Contains(errorMessage, "no candidates returned"):
		errorMessage = "no_candidates_returned"
	case strings.Contains(errorMessage, "has been suspended"):
		errorMessage = "key_suspended"
	case strings.Contains(errorMessage, "The caller does not have permission"):
		errorMessage = "caller_permission_denied"
	case strings.Contains(errorMessage, "Quota exceeded for metric"):
		errorMessage = "quota_exceeded"
	case strings.Contains(errorMessage, "Resource has been exhausted"):
		errorMessage = "resource_exhausted"
	case strings.Contains(errorMessage, "The model is overloaded"):
		errorMessage = "model_overloaded"
	case strings.Contains(errorMessage, "bad_response_status_code"):
		errorMessage = "bad_response_status_code"
	case strings.Contains(errorMessage, "当前分组上游负载已饱和"):
		errorMessage = "ups_overload"
	case strings.Contains(errorMessage, "An internal error has occurred"):
		errorMessage = "internal_error_google"
	case strings.Contains(errorMessage, "have exceeded the call rate limit for your current AIServices S0 pricing tier"):
		errorMessage = "rate_limit_exceeded_azure"
	case strings.Contains(errorMessage, "upstream error with status 408"):
		errorMessage = "upstream_timeout_408"
	case strings.Contains(errorMessage, "failed to get model resp"):
		errorMessage = "failed_to_get_model_resp"
	case strings.Contains(errorMessage, "The operation was timeout"):
		errorMessage = "operation_timeout"
	case strings.Contains(errorMessage, "exceeded for UserConcurrentRequests. Please wait"):
		errorMessage = "user_concurrent_requests_exceeded"
	case strings.Contains(errorMessage, "API Key not found"):
		errorMessage = "api_key_not_found"
	case strings.Contains(errorMessage, "context deadline exceeded (Client.Timeout exceeded while awaiting headers"):
		errorMessage = "context_deadline_exceeded_while_awaiting_headers"
	case strings.Contains(errorMessage, "无可用渠道"):
		errorMessage = "no_available_channel"
	case strings.Contains(errorMessage, "API key expired. Please renew the API key."):
		errorMessage = "api_key_expired"
	case strings.Contains(errorMessage, "bad_response_status_code"):
		errorMessage = "bad_response_status_code"
	case strings.Contains(errorMessage, "当前分组上游负载已饱和"):
		errorMessage = "ups_overload"
	case strings.Contains(errorMessage, "An internal error has occurred"):
		errorMessage = "internal_error_google"
	case strings.Contains(errorMessage, "API key not valid. Please pass a valid API key."):
		errorMessage = "api_key_invalid"
	case strings.Contains(errorMessage, "该令牌状态不可用"):
		errorMessage = "token_disabled"
	case strings.Contains(errorMessage, "write: broken pipe"):
		errorMessage = "write_broken_pipe"
	case strings.Contains(errorMessage, "Internal error encountered"):
		errorMessage = "internal_error_encountered"
	case strings.Contains(errorMessage, "Quota exceeded for quota metric 'Generate Content API requests per minute' and limit 'GenerateContent request limit per minute for a region' of service"):
		errorMessage = "quota_exceeded_for_generate_content_api_requests_region"
	case strings.Contains(errorMessage, "Your API key was reported as leaked. Please use another API key."):
		errorMessage = "api_key_reported_as_leaked"
	case strings.Contains(errorMessage, "error response body is empty"):
		errorMessage = "error_response_body_is_empty"
	case strings.Contains(errorMessage, "无可用渠道"):
		errorMessage = "no_available_channel"
	case strings.Contains(errorMessage, "Client.Timeout or context cancellation while reading body"):
		errorMessage = "client_timeout_or_context_cancellation_while_reading_body"
	case strings.Contains(errorMessage, "failed to get request body"):
		errorMessage = "failed_to_get_request_body"
	case strings.Contains(errorMessage, "read: connection reset by peer, code is do_request_failed"):
		errorMessage = "read_connection_reset_by_peer_do_request_failed"
	case strings.Contains(errorMessage, "Please reduce the length of the messages or completion"):
		errorMessage = "please_reduce_the_length_of_the_messages_or_completion"
	case strings.Contains(errorMessage, "insufficient tool messages following tool_calls message"):
		errorMessage = "insufficient_tool_messages_following_tool_calls_message"
	case strings.Contains(errorMessage, "Content Exists Risk"):
		errorMessage = "content_exists_risk"
	case strings.Contains(errorMessage, "Too many tokens, please wait before trying again."):
		errorMessage = "too_many_tokens_please_wait_before_trying_again"
	case strings.Contains(errorMessage, "Generative Language API has not been used in project"):
		errorMessage = "generative_language_api_has_not_been_used_in_project"
	case strings.Contains(errorMessage, "failed to record log"):
		errorMessage = "failed_to_record_log"
	case strings.Contains(errorMessage, "Failed to unmarshal response: invalid character"):
		errorMessage = "failed_to_unmarshal_response_invalid_character"
	case strings.Contains(errorMessage, "total tokens is 0"):
		errorMessage = "total_tokens_is_0"
	case strings.Contains(errorMessage, "fail to decode image config"):
		errorMessage = "fail_to_decode_image_config"
	case strings.Contains(errorMessage, "You exceeded your current quota, please check your plan and billing details"):
		errorMessage = "exceeded_your_current_quota"
	case strings.Contains(errorMessage, "无效的令牌"):
		errorMessage = "invalid_token"
	case strings.Contains(errorMessage, "An assistant message with 'tool_calls' must be followed by tool messages responding to each 'tool_call_id'."):
		errorMessage = "invalid_tool_calls_without_tool_messages"
	case strings.Contains(errorMessage, "Please reduce the length of the messages or completion"):
		errorMessage = "reduce_the_length_of_the_messages"
	case strings.Contains(errorMessage, "Too many requests, please wait before trying again."):
		errorMessage = "too_many_requests"
	default:
		errorMessage = "unknown"
	}
	return errorMessage
}
