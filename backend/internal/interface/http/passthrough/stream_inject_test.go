package passthrough

import (
	"encoding/json"
	"testing"
)

func TestMaybeInjectStreamUsage(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		enabled bool
		wantMod bool
		check   func(t *testing.T, out map[string]any)
	}{
		{
			name:    "启用 + 流式未配 → 注入",
			body:    `{"model":"gpt-4o","stream":true,"messages":[]}`,
			enabled: true,
			wantMod: true,
			check: func(t *testing.T, out map[string]any) {
				so, ok := out["stream_options"].(map[string]any)
				if !ok {
					t.Fatalf("stream_options 缺失: %v", out)
				}
				if so["include_usage"] != true {
					t.Errorf("include_usage 应为 true, got %v", so["include_usage"])
				}
			},
		},
		{
			name:    "未启用 → 不改（Anthropic 路由）",
			body:    `{"model":"claude-3-5","stream":true}`,
			enabled: false,
			wantMod: false,
		},
		{
			name:    "非流式 → 不改",
			body:    `{"model":"gpt-4o","messages":[]}`,
			enabled: true,
			wantMod: false,
		},
		{
			name:    "stream:false → 不改",
			body:    `{"model":"gpt-4o","stream":false}`,
			enabled: true,
			wantMod: false,
		},
		{
			name:    "已显式配置 stream_options → 不改（尊重客户端）",
			body:    `{"model":"gpt-4o","stream":true,"stream_options":{"include_usage":false}}`,
			enabled: true,
			wantMod: false,
		},
		{
			name:    "非 JSON body → 不改",
			body:    `not json`,
			enabled: true,
			wantMod: false,
		},
		{
			name:    "空 body → 不改",
			body:    ``,
			enabled: true,
			wantMod: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := maybeInjectStreamUsage([]byte(c.body), c.enabled)
			modified := string(out) != c.body
			if modified != c.wantMod {
				t.Fatalf("want modified=%v, got %v\n原: %s\n后: %s", c.wantMod, modified, c.body, string(out))
			}
			if c.check != nil {
				var obj map[string]any
				if err := json.Unmarshal(out, &obj); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				c.check(t, obj)
			}
		})
	}
}
