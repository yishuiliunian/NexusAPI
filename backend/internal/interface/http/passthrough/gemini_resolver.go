// gemini_resolver.go —— 从 URL 路径抽取 Gemini 模型名。
//
// Gemini 路径形如 /v1beta/models/{model}:{action}，比如：
//   /v1beta/models/gemini-1.5-flash:generateContent
//   /v1beta/models/gemini-1.5-pro:streamGenerateContent?alt=sse
//   /v1beta/models/text-embedding-004:embedContent
package passthrough

import (
	"net/http"
	"strings"
)

// GeminiModelResolver 从 /v1beta/models/{model}:{action} 中提取 model 名。
// 非此形态路径返回空字符串。
func GeminiModelResolver(r *http.Request, _ []byte) string {
	path := r.URL.Path
	const prefix = "/v1beta/models/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(path, prefix)
	// 模型名到第一个 ":"
	if idx := strings.Index(rest, ":"); idx > 0 {
		return rest[:idx]
	}
	return ""
}
