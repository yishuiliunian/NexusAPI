package passthrough

import (
	"encoding/json"
	"testing"
)

func TestNormalizeModel(t *testing.T) {
	cases := map[string]string{
		"claude-opus-4-7[1m]":          "claude-opus-4-7",
		"claude-opus-4-7":              "claude-opus-4-7",
		"gpt-4o[verbose]":              "gpt-4o",
		"gpt-4o":                       "gpt-4o",
		"":                             "",
		"claude-opus[1m][beta]":        "claude-opus[1m]", // 只剥最尾
		"bracket-in-[middle]-name":     "bracket-in-[middle]-name",
		"claude[]":                     "claude",   // 空方括号也剥
		"unmatched[":                   "unmatched[", // 未闭合保持
	}
	for in, want := range cases {
		if got := normalizeModel(in); got != want {
			t.Errorf("normalizeModel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRewriteBodyModel(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		newName  string
		wantGot  string // 期望输出 body 里的 model 字段
		wantPass bool   // 是否应该保持原 body 不变
	}{
		{
			name:    "标准 Anthropic/OpenAI body",
			body:    `{"model":"claude-opus-4-7[1m]","max_tokens":100,"messages":[{"role":"user","content":"hi"}]}`,
			newName: "claude-opus-4-7",
			wantGot: "claude-opus-4-7",
		},
		{
			name:    "body 含 tools 等复杂字段",
			body:    `{"model":"gpt-4o[verbose]","messages":[],"tools":[{"name":"x"}],"stream":true}`,
			newName: "gpt-4o",
			wantGot: "gpt-4o",
		},
		{
			name:     "无 model 字段保持原样",
			body:     `{"input":"hi"}`,
			newName:  "claude-opus-4-7",
			wantPass: true,
		},
		{
			name:     "非 JSON 保持原样",
			body:     `not json at all`,
			newName:  "x",
			wantPass: true,
		},
		{
			name:     "空 body 保持原样",
			body:     ``,
			newName:  "x",
			wantPass: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := rewriteBodyModel([]byte(c.body), c.newName)
			if c.wantPass {
				if string(out) != c.body {
					t.Errorf("should pass through, got %q", string(out))
				}
				return
			}
			var obj map[string]any
			if err := json.Unmarshal(out, &obj); err != nil {
				t.Fatalf("output not valid JSON: %v", err)
			}
			if obj["model"] != c.wantGot {
				t.Errorf("model = %v, want %q", obj["model"], c.wantGot)
			}
		})
	}
}