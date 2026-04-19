// stream_inject.go 对 OpenAI 兼容流式请求强制注入 stream_options.include_usage=true。
//
// 问题：OpenAI /v1/chat/completions 流式响应的 usage 字段需客户端显式传
// `stream_options: {include_usage: true}` 才会返回。若客户端漏传，上游不返回
// usage → 提取器拿到 0 → 计费漏洞（请求免费通过）。
//
// 方案：在网关侧对 /chat/completions 和 /responses 两条路由做请求改写：
//   - Route 显式声明 `InjectStreamUsage: true`（router.go 控制启用）
//   - body 含 "stream": true
//   - 且没有显式 stream_options.include_usage
//   - → 注入 stream_options.include_usage = true
//
// Anthropic /v1/messages 的流式 usage 在 message_start/message_delta 帧里自带，
// 且 Anthropic API 对未知字段严格校验。因此 Claude 路由**不**开启此开关。

package passthrough

import "encoding/json"

// maybeInjectStreamUsage 若 enabled 且请求是流式且未显式配置 stream_options，
// 则注入 include_usage=true。返回新 body；无需改写返回原 body。
func maybeInjectStreamUsage(body []byte, enabled bool) []byte {
	if !enabled || len(body) == 0 {
		return body
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return body
	}
	stream, _ := obj["stream"].(bool)
	if !stream {
		return body
	}
	if _, exists := obj["stream_options"]; exists {
		// 尊重客户端意图（比如故意关闭 usage）。
		return body
	}
	obj["stream_options"] = map[string]any{"include_usage": true}
	out, err := json.Marshal(obj)
	if err != nil {
		return body
	}
	return out
}
