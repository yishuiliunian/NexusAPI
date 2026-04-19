// Package i18n 提供最小的多语言错误消息查找。
//
// 用途：后端返回给前端的错误 code 是稳定的，但可读 message 按客户端语言变化。
// 用法：
//   msg := i18n.T(lang, i18n.KeyInsufficientQuota)
//
// 扩充：新增 key → 在 messages 里 zh/en/ja 都加一条。
package i18n

import "strings"

// Key 消息键；使用 snake_case。
type Key string

const (
	KeyInsufficientQuota Key = "insufficient_quota"
	KeyRateLimited       Key = "rate_limited"
	KeyUnauthenticated   Key = "unauthenticated"
	KeyInvalidArgument   Key = "invalid_argument"
	KeyNotFound          Key = "not_found"
	KeyForbidden         Key = "forbidden"
	KeyUpstreamError     Key = "upstream_error"
	KeyInternal          Key = "internal"
)

// messages 以 key → lang → 文本 组织。未知 lang 退化到 en。
var messages = map[Key]map[string]string{
	KeyInsufficientQuota: {
		"zh": "余额不足",
		"en": "Insufficient quota",
		"ja": "残高が不足しています",
	},
	KeyRateLimited: {
		"zh": "请求频率超限",
		"en": "Too many requests",
		"ja": "リクエスト過多",
	},
	KeyUnauthenticated: {
		"zh": "未登录或凭据无效",
		"en": "Unauthenticated",
		"ja": "認証されていません",
	},
	KeyInvalidArgument: {
		"zh": "参数错误",
		"en": "Invalid argument",
		"ja": "引数が無効です",
	},
	KeyNotFound: {
		"zh": "资源不存在",
		"en": "Not found",
		"ja": "見つかりません",
	},
	KeyForbidden: {
		"zh": "权限不足",
		"en": "Forbidden",
		"ja": "権限がありません",
	},
	KeyUpstreamError: {
		"zh": "上游服务异常",
		"en": "Upstream error",
		"ja": "アップストリームエラー",
	},
	KeyInternal: {
		"zh": "系统内部错误",
		"en": "Internal error",
		"ja": "内部エラー",
	},
}

// T 查找翻译。未知 key → 原 key 字符串；未知 lang → 英文。
func T(lang string, key Key) string {
	m, ok := messages[key]
	if !ok {
		return string(key)
	}
	lang = normalize(lang)
	if s, ok := m[lang]; ok {
		return s
	}
	return m["en"]
}

// ParseAcceptLanguage 从 Accept-Language header 选出首选语言（zh/en/ja），未匹配返回 en。
func ParseAcceptLanguage(header string) string {
	for _, part := range strings.Split(header, ",") {
		tag := strings.ToLower(strings.TrimSpace(strings.SplitN(part, ";", 2)[0]))
		if strings.HasPrefix(tag, "zh") {
			return "zh"
		}
		if strings.HasPrefix(tag, "ja") {
			return "ja"
		}
		if strings.HasPrefix(tag, "en") {
			return "en"
		}
	}
	return "en"
}

func normalize(lang string) string {
	switch strings.ToLower(lang) {
	case "zh", "zh-cn", "zh-tw", "zh-hk":
		return "zh"
	case "ja", "ja-jp":
		return "ja"
	default:
		return "en"
	}
}
