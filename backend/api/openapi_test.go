// Package api 存放 HTTP 契约：openapi.yaml。
// 本测试保证 openapi.yaml 的语法有效性与 $ref 完整性，避免契约静默漂移。
package api

import (
	_ "embed"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

//go:embed openapi.yaml
var openapiYAML []byte

// ---------- 基础结构 ----------

type specRoot struct {
	OpenAPI    string                     `yaml:"openapi"`
	Info       map[string]any             `yaml:"info"`
	Paths      map[string]map[string]any  `yaml:"paths"`
	Components componentsBlock            `yaml:"components"`
	Tags       []map[string]any           `yaml:"tags"`
}

type componentsBlock struct {
	Schemas         map[string]any `yaml:"schemas"`
	Responses       map[string]any `yaml:"responses"`
	Parameters      map[string]any `yaml:"parameters"`
	SecuritySchemes map[string]any `yaml:"securitySchemes"`
}

// parseSpec 解析 openapi.yaml；解析失败即 FailNow。
func parseSpec(t *testing.T) *specRoot {
	t.Helper()
	var spec specRoot
	if err := yaml.Unmarshal(openapiYAML, &spec); err != nil {
		t.Fatalf("openapi.yaml parse: %v", err)
	}
	return &spec
}

// ---------- 测试 ----------

func TestOpenAPI_TopLevel(t *testing.T) {
	spec := parseSpec(t)
	if !strings.HasPrefix(spec.OpenAPI, "3.") {
		t.Errorf("expected 3.x openapi version, got %q", spec.OpenAPI)
	}
	if spec.Info["title"] != "NexusAPI" {
		t.Errorf("info.title=%v", spec.Info["title"])
	}
	if len(spec.Paths) == 0 {
		t.Fatal("spec has no paths")
	}
}

// 契约关键端点必须存在：缺失即视为破坏契约。
func TestOpenAPI_CoreEndpointsPresent(t *testing.T) {
	spec := parseSpec(t)
	required := []string{
		"/healthz",
		"/v1/chat/completions",
		"/v1/models",
		"/api/auth/login",
		"/api/auth/register",
		"/api/user/me",
		"/api/user/apikeys",
		"/api/user/apikeys/{id}",
		"/api/user/usages",
		"/api/billing/redeem",
	}
	for _, p := range required {
		if _, ok := spec.Paths[p]; !ok {
			t.Errorf("missing required path: %s", p)
		}
	}
}

// 所有关键 schema 必须被定义。
func TestOpenAPI_CoreSchemasDefined(t *testing.T) {
	spec := parseSpec(t)
	required := []string{
		"Error", "EmailPassword", "UserProfile", "ApiKey",
		"Usage", "Ledger", "ChatCompletionRequest", "ChatMessage",
		"ChatCompletionResponse", "TokenUsage", "OK",
	}
	for _, name := range required {
		if _, ok := spec.Components.Schemas[name]; !ok {
			t.Errorf("missing schema: %s", name)
		}
	}
}

// 每个 securityScheme 与 response 都必须存在。
func TestOpenAPI_SecuritySchemesDefined(t *testing.T) {
	spec := parseSpec(t)
	for _, want := range []string{"apiKeyAuth", "sessionCookie"} {
		if _, ok := spec.Components.SecuritySchemes[want]; !ok {
			t.Errorf("missing security scheme: %s", want)
		}
	}
}

// 扫描整个文件里的 `$ref: "#/components/X/Y"`，确保 Y 在 components.X 里定义。
// 这是最容易出错的点：改名未同步。
func TestOpenAPI_AllRefsResolve(t *testing.T) {
	spec := parseSpec(t)
	text := string(openapiYAML)

	// components 子集映射
	sections := map[string]map[string]any{
		"schemas":         spec.Components.Schemas,
		"responses":       spec.Components.Responses,
		"parameters":      spec.Components.Parameters,
		"securitySchemes": spec.Components.SecuritySchemes,
	}

	const prefix = `$ref: "#/components/`
	seen := map[string]bool{}
	start := 0
	for {
		i := strings.Index(text[start:], prefix)
		if i < 0 {
			break
		}
		i += start
		rest := text[i+len(prefix):]
		end := strings.Index(rest, `"`)
		if end < 0 {
			break
		}
		ref := rest[:end]
		start = i + len(prefix) + end
		if seen[ref] {
			continue
		}
		seen[ref] = true

		parts := strings.Split(ref, "/")
		if len(parts) != 2 {
			t.Errorf("malformed $ref: %q", ref)
			continue
		}
		section := parts[0]
		name := parts[1]
		m, ok := sections[section]
		if !ok {
			t.Errorf("$ref points to unknown section %q (full: %q)", section, ref)
			continue
		}
		if _, ok := m[name]; !ok {
			t.Errorf("$ref %q not defined in components.%s", name, section)
		}
	}
	if len(seen) == 0 {
		t.Error("no $ref found — spec likely malformed")
	}
}

// 鉴权路径必须带 security 字段；否则前端会以为是公开接口。
func TestOpenAPI_AuthenticatedPathsHaveSecurity(t *testing.T) {
	spec := parseSpec(t)
	needsAuth := map[string]string{
		"/v1/chat/completions": "apiKeyAuth",
		"/v1/models":           "apiKeyAuth",
		"/api/user/me":         "sessionCookie",
		"/api/user/apikeys":    "sessionCookie",
		"/api/billing/redeem":  "sessionCookie",
	}
	for p, scheme := range needsAuth {
		pi, ok := spec.Paths[p]
		if !ok {
			continue // 已由 CoreEndpointsPresent 报错
		}
		for verb, op := range pi {
			if verb == "parameters" || verb == "$ref" {
				continue
			}
			opMap, _ := op.(map[string]any)
			if opMap == nil {
				continue
			}
			sec, _ := opMap["security"].([]any)
			if len(sec) == 0 {
				t.Errorf("%s %s: missing security block (expected %s)", strings.ToUpper(verb), p, scheme)
				continue
			}
			found := false
			for _, item := range sec {
				m, _ := item.(map[string]any)
				if _, ok := m[scheme]; ok {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s %s: security block missing %s", strings.ToUpper(verb), p, scheme)
			}
		}
	}
}

// 错误响应必须至少覆盖 400/401/402 三种常见场景中的一部分。
// /v1/chat/completions 是最复杂的端点，用它做代表性校验。
func TestOpenAPI_ChatErrorResponsesCovered(t *testing.T) {
	spec := parseSpec(t)
	post, ok := spec.Paths["/v1/chat/completions"]["post"].(map[string]any)
	if !ok {
		t.Fatal("/v1/chat/completions POST not defined")
	}
	responses, ok := post["responses"].(map[string]any)
	if !ok {
		t.Fatal("responses missing")
	}
	for _, code := range []string{"200", "401", "402", "502"} {
		if _, ok := responses[code]; !ok {
			t.Errorf("chat response missing status %s", code)
		}
	}
}
