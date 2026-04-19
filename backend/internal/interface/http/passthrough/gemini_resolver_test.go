package passthrough

import (
	"net/http"
	"net/url"
	"testing"
)

func TestGeminiModelResolver(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/v1beta/models/gemini-1.5-flash:generateContent", "gemini-1.5-flash"},
		{"/v1beta/models/gemini-1.5-pro:streamGenerateContent", "gemini-1.5-pro"},
		{"/v1beta/models/text-embedding-004:embedContent", "text-embedding-004"},
		// 非预期路径
		{"/v1beta/models/gemini-1.5-flash", ""},   // 无 action
		{"/v1/chat/completions", ""},              // 非 gemini 路径
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			r := &http.Request{URL: &url.URL{Path: tc.path}}
			got := GeminiModelResolver(r, nil)
			if got != tc.want {
				t.Errorf("path=%q got %q want %q", tc.path, got, tc.want)
			}
		})
	}
}
