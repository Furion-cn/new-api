# 错误码说明文档

本文档说明了 New API 系统中各种错误码的含义、匹配规则和处理办法。

## 使用说明

- **例子**: 实际错误信息示例
- **错误类型**: 系统识别的错误码
- **匹配规则**: 用于匹配错误信息的字符串（使用 `strings.Contains` 匹配）
- **处理办法**: 推荐的解决方案

---

## 错误码对照表

| 例子 | 错误类型 | 匹配规则 | 处理办法 |
|------|---------|---------|---------|
| (空字符串) | `none` | `errorMessage == ""` | 检查是否是正常情况，查看详细日志 |
| `write: connection timed out` | `connection_timeout` | `write: connection timed out` | 1. 检查网络连接是否正常<br>2. 增加超时时间配置<br>3. 检查上游服务的健康状态<br>4. 考虑使用重试机制 |
| `do request failed` | `do_request_failed` | `do request failed` | 1. 检查上游服务的可用性<br>2. 验证网络连接和 DNS 配置<br>3. 检查代理设置（如果使用）<br>4. 查看详细错误日志定位具体问题 |
| `no candidates returned` | `no_candidates_returned` | `no candidates returned` | 1. 检查请求格式是否正确<br>2. 验证上游服务的状态<br>3. 查看详细的错误日志<br>4. 尝试使用其他渠道 |
| `has been suspended` | `key_suspended` | `has been suspended` | 1. 检查 API Key 的状态<br>2. 联系服务提供商了解暂停原因<br>3. 更换新的 API Key<br>4. 检查账户是否有异常活动 |
| `The caller does not have permission` | `caller_permission_denied` | `The caller does not have permission` | 1. 检查 API Key 的权限设置<br>2. 验证项目权限配置<br>3. 确认服务是否已启用<br>4. 联系服务商检查权限 |
| `Quota exceeded for metric` | `quota_exceeded` | `Quota exceeded for metric` | 1. 检查配额使用情况<br>2. 等待配额重置（如果是时间限制）<br>3. 升级服务计划增加配额<br>4. 优化请求频率和内容 |
| `Resource has been exhausted` | `resource_exhausted` | `Resource has been exhausted` | 1. 等待资源释放<br>2. 降低请求复杂度<br>3. 联系服务提供商<br>4. 使用其他服务或模型 |
| `The model is overloaded` | `model_overloaded` | `The model is overloaded` | 1. 等待一段时间后重试<br>2. 使用其他模型<br>3. 降低请求频率<br>4. 实现指数退避重试 |
| `当前分组上游负载已饱和` | `ups_overload` | `当前分组上游负载已饱和` | 1. 等待负载降低<br>2. 增加更多上游服务<br>3. 优化请求分发策略<br>4. 检查上游服务的性能 |
| `An internal error has occurred` | `internal_error_google` | `An internal error has occurred` | 1. 等待一段时间后重试<br>2. 检查 Google 服务状态页面<br>3. 实现重试机制<br>4. 联系 Google 支持（如果持续出现） |
| `have exceeded the call rate limit for your current AIServices S0 pricing tier` | `rate_limit_exceeded_azure` | `have exceeded the call rate limit for your current AIServices S0 pricing tier` | 1. 降低请求频率<br>2. 升级到更高的定价层<br>3. 实现请求队列和限流<br>4. 使用多个 Azure 资源分散请求 |
| `upstream error with status 408` | `upstream_timeout_408` | `upstream error with status 408` | 1. 优化请求内容，减少 token 数量<br>2. 增加上游服务的超时配置<br>3. 检查上游服务的负载情况<br>4. 考虑使用更快的模型或减少请求复杂度 |
| `failed to get model resp` | `failed_to_get_model_resp` | `failed to get model resp` | 1. 检查响应格式是否正确<br>2. 更新响应解析逻辑<br>3. 查看详细错误日志<br>4. 检查上游服务是否有变更 |
| `The operation was timeout` | `operation_timeout` | `The operation was timeout` | 1. 检查请求的复杂度，考虑简化<br>2. 增加超时时间配置<br>3. 检查上游服务的性能指标<br>4. 考虑使用异步处理 |
| `exceeded for UserConcurrentRequests. Please wait` | `user_concurrent_requests_exceeded` | `exceeded for UserConcurrentRequests. Please wait` | 1. 减少并发请求数<br>2. 实现请求队列<br>3. 增加并发限制配置<br>4. 优化请求处理逻辑 |
| `API Key not found` | `api_key_not_found` | `API Key not found` | 1. 验证 API Key 是否正确<br>2. 检查 API Key 是否在服务商处存在<br>3. 重新生成并配置 API Key<br>4. 确认 Key 的格式是否正确 |
| `context deadline exceeded (Client.Timeout exceeded while awaiting headers)` | `context_deadline_exceeded_while_awaiting_headers` | `context deadline exceeded (Client.Timeout exceeded while awaiting headers)` | 1. 增加客户端超时时间<br>2. 检查上游服务的响应时间<br>3. 优化网络连接<br>4. 考虑使用连接池 |
| `无可用渠道` | `no_available_channel` | `无可用渠道` | 1. 检查渠道配置和状态<br>2. 启用更多渠道<br>3. 检查渠道的健康状态<br>4. 检查渠道的负载均衡配置 |
| `API key expired. Please renew the API key.` | `api_key_expired` | `API key expired. Please renew the API key.` | 1. 在服务商处续期 API Key<br>2. 生成新的 API Key 并更新配置<br>3. 设置 Key 过期提醒<br>4. 检查 Key 的有效期设置 |
| `API key not valid. Please pass a valid API key.` | `api_key_invalid` | `API key not valid. Please pass a valid API key.` | 1. 验证 API Key 格式是否正确<br>2. 确认 Key 是否属于正确的项目<br>3. 检查 Key 的权限设置<br>4. 重新生成并配置 API Key |
| `该令牌状态不可用` | `token_disabled` | `该令牌状态不可用` | 1. 检查 Token 的状态<br>2. 联系管理员启用 Token<br>3. 创建新的 Token<br>4. 检查 Token 的过期时间 |
| `write: broken pipe` | `write_broken_pipe` | `write: broken pipe` | 1. 检查上游服务的连接管理<br>2. 实现连接重试机制<br>3. 检查网络稳定性<br>4. 验证上游服务的健康状态 |
| `Internal error encountered` | `internal_error_encountered` | `Internal error encountered` | 1. 等待后重试<br>2. 检查上游服务状态<br>3. 实现重试机制<br>4. 查看详细错误日志 |
| `Quota exceeded for quota metric 'Generate Content API requests per minute' and limit 'GenerateContent request limit per minute for a region' of service` | `quota_exceeded_for_generate_content_api_requests_region` | `Quota exceeded for quota metric 'Generate Content API requests per minute' and limit 'GenerateContent request limit per minute for a region' of service` | 1. 降低请求频率<br>2. 使用多个 API Key 分散请求<br>3. 实现请求队列和限流<br>4. 考虑使用不同的区域 |
| `Your API key was reported as leaked. Please use another API key.` | `api_key_reported_as_leaked` | `Your API key was reported as leaked. Please use another API key.` | 1. **立即更换 API Key**（紧急）<br>2. 检查是否有未授权的访问<br>3. 审查 Key 的使用日志<br>4. 启用 Key 的访问限制<br>5. 加强安全措施 |
| `error response body is empty` | `error_response_body_is_empty` | `error response body is empty` | 1. 检查上游服务的响应<br>2. 查看 HTTP 状态码<br>3. 检查响应头信息<br>4. 实现更详细的错误日志 |
| `Client.Timeout or context cancellation while reading body` | `client_timeout_or_context_cancellation_while_reading_body` | `Client.Timeout or context cancellation while reading body` | 1. 增加读取超时时间<br>2. 检查响应体大小<br>3. 优化网络连接稳定性<br>4. 检查是否有上下文取消的逻辑 |
| `failed to get request body` | `failed_to_get_request_body` | `failed to get request body` | 1. 检查请求体是否已被读取<br>2. 确保请求体格式正确<br>3. 检查中间件处理顺序<br>4. 验证请求体大小限制 |
| `read: connection reset by peer, code is do_request_failed` | `read_connection_reset_by_peer_do_request_failed` | `read: connection reset by peer, code is do_request_failed` | 1. 检查上游服务的连接策略<br>2. 实现重试机制<br>3. 检查是否有防火墙或代理干扰<br>4. 验证上游服务的稳定性 |
| `Please reduce the length of the messages or completion` | `please_reduce_the_length_of_the_messages_or_completion` | `Please reduce the length of the messages or completion` | 1. 减少输入消息的长度<br>2. 降低 max_tokens 参数<br>3. 使用更长的上下文窗口模型<br>4. 分段处理长文本 |
| `insufficient tool messages following tool_calls message` | `insufficient_tool_messages_following_tool_calls_message` | `insufficient tool messages following tool_calls message` | 1. 确保每个 tool_call 都有对应的 tool message<br>2. 检查消息序列的正确性<br>3. 验证工具调用的格式<br>4. 参考 API 文档确保格式正确 |
| `Content Exists Risk` | `content_exists_risk` | `Content Exists Risk` | 1. 修改请求内容<br>2. 检查内容是否违反服务条款<br>3. 使用不同的表达方式<br>4. 联系服务商了解具体原因 |
| `Too many tokens, please wait before trying again.` | `too_many_tokens_please_wait_before_trying_again` | `Too many tokens, please wait before trying again.` | 1. 减少单次请求的 Token 数量<br>2. 降低请求频率<br>3. 等待限流重置<br>4. 优化请求内容 |
| `Generative Language API has not been used in project` | `generative_language_api_has_not_been_used_in_project` | `Generative Language API has not been used in project` | 1. 在 Google Cloud Console 中启用 Generative Language API<br>2. 检查项目的 API 权限<br>3. 验证 API Key 是否属于正确的项目<br>4. 确认计费账户已设置 |
| `failed to record log` | `failed_to_record_log` | `failed to record log` | 1. 检查数据库连接<br>2. 验证日志表是否存在<br>3. 检查数据库权限<br>4. 查看数据库错误日志 |
| `Failed to unmarshal response: invalid character` | `failed_to_unmarshal_response_invalid_character` | `Failed to unmarshal response: invalid character` | 1. 检查响应内容格式<br>2. 验证上游服务返回的数据<br>3. 处理非 JSON 响应<br>4. 添加响应格式验证 |
| `total tokens is 0` | `total_tokens_is_0` | `total tokens is 0` | 1. 检查是否是流式响应<br>2. 验证上游服务是否正常返回<br>3. 检查 Token 计数逻辑<br>4. 查看详细的请求日志 |
| `fail to decode image config` | `fail_to_decode_image_config` | `fail to decode image config` | 1. 检查图片格式是否支持<br>2. 验证图片文件是否完整<br>3. 检查图片编码格式<br>4. 使用支持的图片格式 |
| `You exceeded your current quota, please check your plan and billing details` | `exceeded_your_current_quota` | `You exceeded your current quota, please check your plan and billing details` | 1. 检查 OpenAI 账户的配额和账单<br>2. 升级服务计划<br>3. 等待配额重置<br>4. 联系 OpenAI 支持增加配额 |
| `无效的令牌` | `invalid_token` | `无效的令牌` | 1. 验证 Token 是否正确<br>2. 检查 Token 是否在系统中存在<br>3. 重新生成 Token<br>4. 确认 Token 的格式 |
| `An assistant message with 'tool_calls' must be followed by tool messages responding to each 'tool_call_id'.` | `invalid_tool_calls_without_tool_messages` | `An assistant message with 'tool_calls' must be followed by tool messages responding to each 'tool_call_id'.` | 1. 确保每个 tool_call_id 都有对应的 tool message<br>2. 检查消息序列的完整性<br>3. 实现正确的工具调用流程<br>4. 参考 API 文档示例 |
| `Too many requests, please wait before trying again.` | `too_many_requests` | `Too many requests, please wait before trying again.` | 1. 降低请求频率<br>2. 实现指数退避重试<br>3. 使用请求队列<br>4. 检查限流配置 |
| `no algo service available now,please wait a moment` | `no_algo_service_available` | `no algo service available now,please wait a moment` | 1. 等待服务恢复<br>2. 检查服务状态<br>3. 实现重试机制<br>4. 联系服务提供商 |
| `Input should be a valid list` | `input_should_be_a_valid_list` | `Input should be a valid list` | 1. 检查输入数据的格式<br>2. 确保输入是有效的列表/数组<br>3. 验证 JSON 格式<br>4. 参考 API 文档的格式要求 |
| `Model tpm limit exceeded. Please try again later` | `model_tpm_limit_exceeded` | `Model tpm limit exceeded. Please try again later` | 1. 降低请求频率<br>2. 减少单次请求的 Token 数量<br>3. 使用多个 API Key 分散请求<br>4. 等待限流重置 |
| `nginx ... 500 Internal Server Error` | `nginx_error` | `nginx` 且 `500 Internal Server Error` | 1. 检查 Nginx 日志<br>2. 检查上游服务状态<br>3. 验证 Nginx 配置<br>4. 检查服务器资源使用情况 |
| `该令牌额度已用尽` | `token_quota_exhausted` | `该令牌额度已用尽` | 1. 检查 Token 的配额使用情况<br>2. 为 Token 充值或增加配额<br>3. 检查是否有异常消耗<br>4. 考虑创建新的 Token |
| `bad_response_status_code` | `bad_response_status_code_unknown` | `bad_response_status_code` | 1. 查看具体的状态码<br>2. 检查上游服务的文档<br>3. 实现状态码处理逻辑<br>4. 联系上游服务支持 |
| (未匹配到任何已知模式) | `unknown` | 默认情况 | 1. 查看完整的错误信息<br>2. 检查是否有新的错误类型<br>3. 更新错误码映射<br>4. 联系技术支持 |

---

## 错误分类

### 网络连接错误
- `connection_timeout`
- `do_request_failed`
- `upstream_timeout_408`
- `operation_timeout`
- `context_deadline_exceeded_while_awaiting_headers`
- `client_timeout_or_context_cancellation_while_reading_body`
- `write_broken_pipe`
- `read_connection_reset_by_peer_do_request_failed`

### API Key/Token 错误
- `key_suspended`
- `api_key_not_found`
- `api_key_expired`
- `api_key_invalid`
- `api_key_reported_as_leaked`
- `token_disabled`
- `invalid_token`
- `token_quota_exhausted`

### 配额和限流错误
- `quota_exceeded`
- `quota_exceeded_for_generate_content_api_requests_region`
- `exceeded_your_current_quota`
- `rate_limit_exceeded_azure`
- `user_concurrent_requests_exceeded`
- `too_many_requests`
- `too_many_tokens_please_wait_before_trying_again`
- `model_tpm_limit_exceeded`

### 上游服务错误
- `no_available_channel`
- `no_candidates_returned`
- `model_overloaded`
- `ups_overload`
- `internal_error_google`
- `internal_error_encountered`
- `resource_exhausted`
- `nginx_error`
- `no_algo_service_available`
- `generative_language_api_has_not_been_used_in_project`

### 请求格式错误
- `please_reduce_the_length_of_the_messages_or_completion`
- `insufficient_tool_messages_following_tool_calls_message`
- `content_exists_risk`
- `input_should_be_a_valid_list`
- `invalid_tool_calls_without_tool_messages`

### 系统内部错误
- `failed_to_get_model_resp`
- `failed_to_get_request_body`
- `failed_to_unmarshal_response_invalid_character`
- `failed_to_record_log`
- `error_response_body_is_empty`
- `total_tokens_is_0`
- `fail_to_decode_image_config`
- `caller_permission_denied`
- `bad_response_status_code_unknown`

### 其他错误
- `none`
- `unknown`

---

## 错误处理最佳实践

1. **监控和告警**: 设置监控和告警，及时发现错误
2. **重试机制**: 对临时性错误实现重试（如超时、限流）
3. **降级策略**: 当上游服务不可用时，使用备用方案
4. **日志记录**: 记录详细的错误日志，便于排查问题
5. **错误分类**: 区分临时性错误和永久性错误，采取不同策略
6. **用户体验**: 向用户提供友好的错误提示

---

## 更新日志

- 2025-01-XX: 初始版本，包含所有错误码说明

---

## 相关文档

- [API 文档](../docs/api.md)
- [配置指南](../docs/configuration.md)
- [故障排查指南](../docs/troubleshooting.md)
