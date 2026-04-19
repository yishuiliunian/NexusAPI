package audit

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeMeta_RedactsSensitive(t *testing.T) {
	in := []byte(`{"email":"a@x","password":"hunter2","secret":"k","new_password":"y","nested":{"password":"ignored"}}`)
	out := sanitizeMeta(in, 4096)

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("非法 JSON: %v", err)
	}
	for _, k := range []string{"password", "secret", "new_password"} {
		if got[k] != "***" {
			t.Errorf("%s 未脱敏: %v", k, got[k])
		}
	}
	// nested password 当前实现不递归；只 redact 顶层——测试明确这一点
	nested, _ := got["nested"].(map[string]any)
	if nested["password"] == "***" {
		t.Error("当前实现仅 redact 顶层（测试防止未来悄悄改行为）")
	}
	// 非敏感字段保持
	if got["email"] != "a@x" {
		t.Errorf("email 被误改: %v", got["email"])
	}
}

func TestSanitizeMeta_TruncatesLong(t *testing.T) {
	in := strings.Repeat("A", 2000)
	out := sanitizeMeta([]byte(in), 100)
	if len(out) != 100 {
		t.Errorf("截断后长度: %d, want 100", len(out))
	}
}

func TestSanitizeMeta_NonJSONReturnsAsIs(t *testing.T) {
	in := []byte("not json but arbitrary text")
	out := sanitizeMeta(in, 4096)
	if string(out) != string(in) {
		t.Errorf("非 JSON 应原样返回: %q", out)
	}
}

func TestSanitizeMeta_Empty(t *testing.T) {
	if got := sanitizeMeta(nil, 4096); got != nil {
		t.Errorf("nil input 应返回 nil, got %q", got)
	}
}
