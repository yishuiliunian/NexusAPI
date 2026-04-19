// modelmap.go —— 对请求体里顶层 "model" 字段做别名替换。
//
// 只在 body 是 JSON 且含顶层 "model" 字段时生效；否则原样返回。
// 轻量实现：用 encoding/json 做一次 map[string]any round-trip，字段顺序会变，
// 但对模型 API 无影响（provider 都不依赖字段顺序）。
package proxy

import "encoding/json"

func remapModelJSON(body []byte, m map[string]string) []byte {
	if len(body) == 0 || len(m) == 0 {
		return body
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return body
	}
	model, ok := obj["model"].(string)
	if !ok {
		return body
	}
	mapped, ok := m[model]
	if !ok {
		return body
	}
	obj["model"] = mapped
	out, err := json.Marshal(obj)
	if err != nil {
		return body
	}
	return out
}
