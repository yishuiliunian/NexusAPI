// normalize.go 模型名规范化。
//
// 场景：某些客户端会在模型名尾部加方括号标记表明开启某个 beta 特性，例如：
//   - Claude Code：claude-opus-4-7[1m]（启用 1M context）
//   - 个别 fork：gpt-4o[verbose]
//
// 上游（真 Anthropic / LiteLLM 价表 / 社区代理 CCProxy/ZCF）通常只收录裸名字
// （claude-opus-4-7），不带方括号。网关需要：
//   1. 内部用规范名做查价、选渠道、记账
//   2. **转发给上游的 body 里也写规范名**，避免依赖上游 fuzzy 匹配（CCProxy
//      的 availableModels probe 状态不稳定，可能时而命中时而透传给 Anthropic 404）
//
// 之前设计"body 透传原样"考虑的是保留 beta 标识。实测发现：
//   - Anthropic 真 API 启用 1M context 走 header `anthropic-beta` 而非模型后缀
//   - LiteLLM `claude-opus-4-7` 的 max_input_tokens 本身就是 1_000_000
// 所以统一改写 body 最务实。

package passthrough

import (
	"encoding/json"
	"regexp"
)

// bracketSuffix 匹配尾部 [xxx] 标记。非贪婪 + 锚定 $ 避免误伤中间出现的方括号。
var bracketSuffix = regexp.MustCompile(`\[[^\[\]]*\]$`)

// normalizeModel 规范化模型名：去掉尾部 [xxx] 后缀。
// 空串保持空串。非法格式（未闭合方括号等）保持原样。
func normalizeModel(m string) string {
	return bracketSuffix.ReplaceAllString(m, "")
}

// rewriteBodyModel 把 JSON body 里顶层 model 字段改写为 newName。
// 仅当原 body 是 JSON 对象且含 model 字段时生效；否则返回原 body。
// 用于规范化后把裸名写回 body 避免上游对 [1m] 等后缀 404。
func rewriteBodyModel(body []byte, newName string) []byte {
	if len(body) == 0 || newName == "" {
		return body
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return body
	}
	if _, ok := obj["model"]; !ok {
		return body
	}
	obj["model"] = newName
	out, err := json.Marshal(obj)
	if err != nil {
		return body
	}
	return out
}