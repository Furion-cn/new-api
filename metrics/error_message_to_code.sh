#!/bin/sh

# 错误消息转错误代码函数
# 用法:
#   1. 直接转换错误消息: error_message_to_code.sh "error message"
#   2. 从标准输入读取: echo "error message" | error_message_to_code.sh
#   3. 处理指定文件: error_message_to_code.sh /path/to/file.log
#   4. 扫描当前目录: error_message_to_code.sh (无参数时自动扫描当前目录)
#   5. 扫描指定目录: error_message_to_code.sh /path/to/directory

error_message_to_code() {
    local error_message="$1"
    
    # 如果为空，返回 "none"
    if [ -z "$error_message" ]; then
        echo "none"
        return
    fi
    
    # 按优先级顺序检查各种错误模式
    # 注意：较长的、更具体的模式应该放在前面
    
    case "$error_message" in
        *"Quota exceeded for quota metric 'Generate Content API requests per minute' and limit 'GenerateContent request limit per minute for a region' of service"*)
            echo "quota_exceeded_for_generate_content_api_requests_region"
            ;;
        *"context deadline exceeded (Client.Timeout exceeded while awaiting headers"*)
            echo "context_deadline_exceeded_while_awaiting_headers"
            ;;
        *"have exceeded the call rate limit for your current AIServices S0 pricing tier"*)
            echo "rate_limit_exceeded_azure"
            ;;
        *"An assistant message with 'tool_calls' must be followed by tool messages responding to each 'tool_call_id'."*)
            echo "invalid_tool_calls_without_tool_messages"
            ;;
        *"The response was filtered due to the prompt triggering Azure OpenAI's content management policy. Please modify your prompt and retry."*)
            echo "azure_openai_content_management_policy"
            ;;
        *"This request would exceed the rate limit for your organization"*)
            echo "rate_limit_for_your_organization"
            ;;
        *"Could not finish the message because max_tokens or model output limit was reached"*)
            echo "max_tokens_or_model_output_limit_was_reached"
            ;;
        *"Client.Timeout or context cancellation while reading body"*)
            echo "client_timeout_or_context_cancellation_while_reading_body"
            ;;
        *"read: connection reset by peer, code is do_request_failed"*)
            echo "read_connection_reset_by_peer_do_request_failed"
            ;;
        *"insufficient tool messages following tool_calls message"*)
            echo "insufficient_tool_messages_following_tool_calls_message"
            ;;
        *"Please reduce the length of the messages or completion"*)
            echo "please_reduce_the_length_of_the_messages_or_completion"
            ;;
        *"Generative Language API has not been used in project"*)
            echo "generative_language_api_has_not_been_used_in_project"
            ;;
        *"Failed to unmarshal response: invalid character"*)
            echo "failed_to_unmarshal_response_invalid_character"
            ;;
        *"exceeded for UserConcurrentRequests. Please wait"*)
            echo "user_concurrent_requests_exceeded"
            ;;
        *"no algo service available now,please wait a moment"*)
            echo "no_algo_service_available"
            ;;
        *"Too many tokens, please wait before trying again."*)
            echo "too_many_tokens_please_wait_before_trying_again"
            ;;
        *"Too many requests, please wait before trying again."*)
            echo "too_many_requests"
            ;;
        *"You exceeded your current quota, please check your plan and billing details"*)
            echo "exceeded_your_current_quota"
            ;;
        *"API key expired. Please renew the API key."*)
            echo "api_key_expired"
            ;;
        *"API key not valid. Please pass a valid API key."*)
            echo "api_key_invalid"
            ;;
        *"Your API key was reported as leaked. Please use another API key."*)
            echo "api_key_reported_as_leaked"
            ;;
        *"Model tpm limit exceeded. Please try again later"*)
            echo "model_tpm_limit_exceeded"
            ;;
        *"User location is not supported for the API use"*)
            echo "user_location_not_supported_for_the_api_use"
            ;;
        *"No available channels for model"*)
            echo "no_available_channels_for_model"
            ;;
        *"The provided model identifier is invalid."*)
            echo "model_identifier_invalid"
            ;;
        *"write: connection timed out"*)
            echo "connection_timeout"
            ;;
        *"do request failed"*)
            echo "do_request_failed"
            ;;
        *"no candidates returned"*)
            echo "no_candidates_returned"
            ;;
        *"has been suspended"*)
            echo "key_suspended"
            ;;
        *"The caller does not have permission"*)
            echo "caller_permission_denied"
            ;;
        *"Quota exceeded for metric"*)
            echo "quota_exceeded"
            ;;
        *"Resource has been exhausted"*)
            echo "resource_exhausted"
            ;;
        *"The model is overloaded"*)
            echo "model_overloaded"
            ;;
        *"当前分组上游负载已饱和"*)
            echo "ups_overload"
            ;;
        *"An internal error has occurred"*)
            echo "internal_error_google"
            ;;
        *"upstream error with status 408"*)
            echo "upstream_timeout_408"
            ;;
        *"failed to get model resp"*)
            echo "failed_to_get_model_resp"
            ;;
        *"The operation was timeout"*)
            echo "operation_timeout"
            ;;
        *"API Key not found"*)
            echo "api_key_not_found"
            ;;
        *"无可用渠道"*)
            echo "no_available_channel"
            ;;
        *"该令牌状态不可用"*)
            echo "token_disabled"
            ;;
        *"write: broken pipe"*)
            echo "write_broken_pipe"
            ;;
        *"Internal error encountered"*)
            echo "internal_error_encountered"
            ;;
        *"error response body is empty"*)
            echo "error_response_body_is_empty"
            ;;
        *"err mess is {}"*)
            echo "error_message_is_empty"
            ;;
        *"failed to get request body"*)
            echo "failed_to_get_request_body"
            ;;
        *"Transport received Server's graceful shutdown GOAWAY"*)
            echo "transport_received_server_goaway"
            ;;
        *"Content Exists Risk"*)
            echo "content_exists_risk"
            ;;
        *"failed to record log"*)
            echo "failed_to_record_log"
            ;;
        *"total tokens is 0"*)
            echo "total_tokens_is_0"
            ;;
        *"fail to decode image config"*)
            echo "fail_to_decode_image_config"
            ;;
        *"无效的令牌"*)
            echo "invalid_token"
            ;;
        *"Input should be a valid list"*)
            echo "input_should_be_a_valid_list"
            ;;
        *)
            # 特殊处理：需要同时包含两个字符串的情况
            if echo "$error_message" | grep -q "nginx" && echo "$error_message" | grep -q "500 Internal Server Error"; then
                echo "nginx_error"
            elif echo "$error_message" | grep -q "该令牌额度已用尽"; then
                echo "token_quota_exhausted"
            elif echo "$error_message" | grep -q "可用渠道不存在"; then
                echo "no_available_channel"
            elif echo "$error_message" | grep -q "unexpected end of JSON input"; then
                echo "unexpected_end_of_json_input"
            elif echo "$error_message" | grep -q "bad_response_status_code"; then
                echo "bad_response_status_code_unknown"
            else
                echo "unknown"
            fi
            ;;
    esac
}

# 处理文件或目录
process_file() {
    local file="$1"
    local line_num=0
    local err_count=0
    local unmatched_count=0
    
    # 提示正在处理的文件
    echo "正在处理文件: $file" >&2
    
    while IFS= read -r line || [ -n "$line" ]; do
        line_num=$((line_num + 1))
        # 每处理10000行显示一次进度
        if [ $((line_num % 10000)) -eq 0 ]; then
            echo "  已处理 $line_num 行，找到 $err_count 个错误，未匹配 $unmatched_count 个" >&2
        fi
        
        # 只处理包含 [ERR] 但不包含 [INFO] 的行
        if echo "$line" | grep -q "\[ERR\]" && ! echo "$line" | grep -q "\[INFO\]"; then
            err_count=$((err_count + 1))
            local error_code=$(error_message_to_code "$line")
            
            # 只打印未匹配到错误代码的行（unknown或none）
            if [ "$error_code" = "unknown" ] || [ "$error_code" = "none" ]; then
                unmatched_count=$((unmatched_count + 1))
                echo "[$file:$line_num] $error_code | $line"
            fi
        fi
    done < "$file"
    
    # 显示处理完成统计
    echo "文件处理完成: 总行数=$line_num, 错误行数=$err_count, 未匹配错误数=$unmatched_count" >&2
}

# 扫描当前目录下的文件
scan_current_directory() {
    local dir="${1:-.}"
    local script_name=$(basename "$0")
    
    # 查找当前目录下的所有文件（排除脚本本身和.sh文件）
    for file in "$dir"/*; do
        # 只处理文件，跳过目录
        if [ -f "$file" ]; then
            local filename=$(basename "$file")
            # 跳过脚本本身和.sh文件
            local ext=$(echo "$filename" | sed 's/.*\.//')
            if [ "$filename" != "$script_name" ] && [ "$ext" != "sh" ]; then
                process_file "$file"
            fi
        fi
    done
}

# 主程序：如果作为脚本直接运行
# 如果提供了命令行参数
if [ $# -gt 0 ]; then
    # 如果第一个参数是文件或目录
    if [ -f "$1" ]; then
        # 是文件，处理文件
        process_file "$1"
    elif [ -d "$1" ]; then
        # 是目录，扫描目录
        scan_current_directory "$1"
    else
        # 否则当作错误消息处理
        error_message_to_code "$*"
    fi
# 否则扫描当前目录
else
    # 检查是否有标准输入
    if [ -t 0 ]; then
        # 没有标准输入，扫描当前目录
        scan_current_directory "."
    else
        # 有标准输入，从标准输入读取
        while IFS= read -r line; do
            # 只处理包含 [ERR] 但不包含 [INFO] 的行
            if echo "$line" | grep -q "\[ERR\]" && ! echo "$line" | grep -q "\[INFO\]"; then
                error_code=$(error_message_to_code "$line")
                if [ "$error_code" = "unknown" ] || [ "$error_code" = "none" ]; then
                    echo "$error_code | $line"
                fi
            fi
        done
    fi
fi

