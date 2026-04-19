// Package openaicompat 注册所有 OpenAI 兼容 API 的供应商（只有 baseURL 不同）。
//
// 替代原先每家一个包的复制写法。新增兼容供应商只需在 defaults 表中加一行。
//
// 若个别供应商需要定制协议（例如 Anthropic/Gemini），就独立一个子包实现 SyncAdaptor。
package openaicompat

import (
	"github.com/yishuiliunian/nexusapi/backend/internal/infra/provider"
	"github.com/yishuiliunian/nexusapi/backend/internal/infra/provider/openai"
)

// defaults 声明式注册表：name → baseURL。
//
// 只含"OpenAI 兼容协议 + 可直接用 Authorization: Bearer"的 provider。
// 需要特殊鉴权（Claude/Gemini/Azure）或协议（文心原生）不在此列。
//
// 透传架构下 baseURL 字段实际不再使用（上游 URL 来自 channel.BaseURL），
// 此处保留只作为管理员录入时的默认值参考。
var defaults = map[string]string{
	"deepseek":   "https://api.deepseek.com/v1",
	"moonshot":   "https://api.moonshot.cn/v1",
	"qwen":       "https://dashscope.aliyuncs.com/compatible-mode/v1",
	"zhipu":      "https://open.bigmodel.cn/api/paas/v4",
	"openrouter": "https://openrouter.ai/api/v1",
	// 百度千帆 V2 OpenAI 兼容端点，文心一言 / ERNIE-Bot 系列走此入口。
	"qianfan":    "https://qianfan.baidubce.com/v2",
}

func init() {
	for name, baseURL := range defaults {
		provider.RegisterSync(openai.New(name, baseURL))
	}
}
