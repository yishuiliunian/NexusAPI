// Package passthrough 实现字节级透明中转 handler。
//
// 与 internal/interface/http/relay 的区别：relay 做 OpenAI 协议伞下的
// payload/response 转换；passthrough 只改 URL + 替换 auth header，body 全透传。
//
// 新架构下所有 AI provider 原生路径（/v1/messages、/v1beta/models/*:*、
// /v1/chat/completions、/v1/embeddings ...）都走这里。
//
// 每次请求：
//  1. auth/rate-limit 由 middleware 前置
//  2. 从 body 摘 model；按 ApiKey 白名单过滤；按 provider 过滤 channel 候选
//  3. Reserve 预占
//  4. 逐个 candidate 调 pkg/proxy.Forward；preflight/4xx/5xx 失败换下一个
//  5. 成功后用 UsageExtractor 结果 Settle + TouchUsed；失败全退款
package passthrough

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	keysapp "github.com/yishuiliunian/nexusapi/backend/internal/app/keys"
	billingapp "github.com/yishuiliunian/nexusapi/backend/internal/app/billing"
	relayapp "github.com/yishuiliunian/nexusapi/backend/internal/app/relay"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/billing"
	domainchannel "github.com/yishuiliunian/nexusapi/backend/internal/domain/channel"
	"github.com/yishuiliunian/nexusapi/backend/internal/interface/http/middleware"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httperr"
	"github.com/yishuiliunian/nexusapi/backend/pkg/proxy"
)

// Route 描述一条透传路由。
type Route struct {
	// Path Gin 路由 path，如 "/v1/messages"。
	Path string
	// Method 默认 POST（模型推理绝大多数是 POST）。空字符串等同 POST。
	Method string
	// Provider 只选 channel.Provider == Provider 的渠道走。
	// 支持多值：例如 OpenAI 兼容路径可接受 openai/openai-compat/deepseek/... 等。
	Providers []string
	// AuthMode 上游鉴权风格。
	AuthMode proxy.AuthMode
	// ExtraHeaders 每次转发追加的上游头（如 anthropic-version）。
	ExtraHeaders map[string]string
	// StripPathPrefix 剥除客户端 path 前缀（仅当命名空间路径如 /anthropic/... 用）。
	StripPathPrefix string
	// DefaultBaseURL channel.BaseURL 为空时用这个。
	DefaultBaseURL string
	// Extractor 从响应摘 usage。
	Extractor proxy.UsageExtractor
	// BillingCap 用于 billing.Usage.Capability（决定定价）。
	BillingCap billing.Capability
	// ReserveMicro 预占金额（micro）。0 表示不预占（按次任务用不上）。
	ReserveMicro int64
	// ModelResolver 自定义"从请求里读 model 名"。nil 时默认从 body 顶层 model 字段读。
	// 对于把 model 放在 URL 路径里的 provider（如 Gemini），必须自定义。
	ModelResolver func(r *http.Request, body []byte) string
	// InjectStreamUsage 为 true 时，对含 stream:true 的 body 注入
	// stream_options.include_usage=true（避免 OpenAI 流式缺 usage 导致计费漏洞）。
	// 仅对 OpenAI 协议路由启用（chat/completions、responses），Anthropic/Gemini 勿开。
	InjectStreamUsage bool
}

// Handler 承载若干 Route。
type Handler struct {
	Proxy    *proxy.Proxy
	Selector *relayapp.Selector
	Billing  *billingapp.Engine
	ApiKey   *keysapp.Service
	Routes   []Route
}

// defaultReserveMicro 默认预占：0.01 元 = 10_000 micro。
const defaultReserveMicro int64 = 10_000

// Register 把所有 Route 挂到 group 上（group 需前置 AuthApiKey/CheckApiKeyIP/RateLimit）。
func (h *Handler) Register(g *gin.RouterGroup) {
	for i := range h.Routes {
		r := &h.Routes[i]
		method := r.Method
		if method == "" {
			method = http.MethodPost
		}
		g.Handle(method, r.Path, h.handle(r))
	}
}

// handle 生成单条 Route 的 Gin handler。
func (h *Handler) handle(route *Route) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := middleware.CurrentApiKey(c)
		if key == nil {
			httperr.Unauthorized(c, "")
			return
		}

		// 先把 body 拷一份：failover 重试时要重新喂给 Forward
		rawBody, _ := io.ReadAll(c.Request.Body)
		_ = c.Request.Body.Close()

		model := ""
		if route.ModelResolver != nil {
			model = route.ModelResolver(c.Request, rawBody)
		} else {
			model = extractModel(rawBody)
		}
		if model == "" {
			httperr.BadRequest(c, "请求未能解析出 model")
			return
		}
		// 规范化模型名：剥尾部 [xxx] 后缀（如 Claude Code 的 [1m]）。
		// 内部查价/选渠道/记账用规范名；**body 里的 model 字段也一并改写**，
		// 避免依赖上游 fuzzy 匹配（CCProxy 的 availableModels probe 状态不稳定），
		// 裸名对任何上游（真 Anthropic / CCProxy / 其他 proxy）都稳定。
		normalizedModel := normalizeModel(model)
		if normalizedModel != model {
			rawBody = rewriteBodyModel(rawBody, normalizedModel)
		}
		if !key.AllowModel(normalizedModel) {
			httperr.Forbidden(c, "此密钥不允许使用模型 "+model)
			return
		}

		// 对 OpenAI 兼容流式请求强制注入 stream_options.include_usage=true，
		// 避免客户端漏传导致 usage=0 的计费漏洞。Anthropic/Gemini 路由不开启此开关。
		rawBody = maybeInjectStreamUsage(rawBody, route.InjectStreamUsage)

		ctx := c.Request.Context()

		// 价格预检：避免缺价请求被免费转发。
		// StrictPricing=false 时 EnsurePriced 永远 nil，保持开发环境兼容。
		if err := h.Billing.EnsurePriced(ctx, normalizedModel, route.BillingCap); err != nil {
			httperr.AbortStatus(c, http.StatusPaymentRequired, err)
			return
		}

		candidates, err := h.pickCandidates(ctx, route, normalizedModel, key.UserID)
		if err != nil {
			httperr.Abort(c, err)
			return
		}

		reserve := route.ReserveMicro
		if reserve == 0 {
			reserve = defaultReserveMicro
		}
		if reserve == 0 {
			reserve = defaultReserveMicro
		}
		rid, err := h.Billing.Reserve(ctx, key.UserID, reserve)
		if err != nil {
			httperr.Abort(c, err)
			return
		}

		pc := h.Billing.BuildContext(ctx, middleware.CurrentUser(c))

		// 逐个 candidate 尝试
		var used *domainchannel.Channel
		var result *proxy.Result
		var lastErr error
		breaker := h.Selector.Breaker()
		affinity := h.Selector.Affinity()
		for _, ch := range candidates {
			// 每次重置 body
			c.Request.Body = io.NopCloser(bytes.NewReader(rawBody))

			up := &proxy.Upstream{
				BaseURL:         baseURLOf(ch, route.DefaultBaseURL),
				APIKey:          ch.Credentials,
				AuthMode:        route.AuthMode,
				ExtraHeaders:    route.ExtraHeaders,
				StripPathPrefix: route.StripPathPrefix,
			}
			r, err := h.Proxy.Forward(c.Writer, c.Request, up, route.Extractor)
			if err == nil {
				used = ch
				result = r
				breaker.RecordSuccess(ctx, ch.ID)
				affinity.Set(ctx, key.UserID, model, ch.ID)
				break
			}
			// 只有"尚未写回客户端"的错误才能 failover
			if shouldFailover(err) {
				breaker.RecordFailure(ctx, ch.ID)
				lastErr = err
				continue
			}
			// 写流程错误（客户端断开等）：已 commit，停止
			lastErr = err
			if r != nil {
				used = ch
				result = r
			}
			break
		}

		if used == nil || result == nil {
			_ = h.Billing.Refund(ctx, rid)
			writeFailoverError(c, lastErr)
			return
		}

		// 计费
		usage := &billing.Usage{
			UserID:     key.UserID,
			ApiKeyID:   key.ID,
			ChannelID:  used.ID,
			Model:      normalizedModel, // 记账用规范名便于聚合
			Capability: route.BillingCap,
			Status:     billing.StatusSuccess,
			RequestID:  c.GetString(middleware.CtxReqID),
			LatencyMs:  int(result.UpstreamLat.Milliseconds()),
		}
		if result.Usage != nil {
			usage.PromptTokens = result.Usage.PromptTokens
			usage.CompletionTokens = result.Usage.CompletionTokens
			usage.CacheTokens = result.Usage.CacheReadTokens
			// 5m TTL 和 1h TTL 分别计费（billing.Compute 按不同溢价系数）。
			usage.CacheWriteTokens = result.Usage.CacheWriteTokens
			usage.CacheWrite1hTokens = result.Usage.CacheWrite1hTokens
			usage.ReasoningTokens = result.Usage.ReasoningTokens
		}
		cost, _ := h.Billing.Compute(ctx, pc,
			billingapp.ChannelPricing{PriceMultiplier: used.PriceMultiplier}, usage)
		_ = h.Billing.Settle(ctx, rid, cost, usage)
		_ = h.ApiKey.TouchUsed(ctx, key.ID)
	}
}

// pickCandidates 取合法渠道并按亲和排序把优先的放前面。
func (h *Handler) pickCandidates(
	ctx context.Context,
	route *Route,
	model string,
	userID uint64,
) ([]*domainchannel.Channel, error) {
	cands, err := h.Selector.Candidates(ctx, model, 0)
	if err != nil {
		return nil, derrors.New(derrors.CodeNotFound, "无可用渠道支持模型 "+model)
	}
	if len(route.Providers) > 0 {
		filtered := make([]*domainchannel.Channel, 0, len(cands))
		allow := map[string]bool{}
		for _, p := range route.Providers {
			allow[p] = true
		}
		for _, ch := range cands {
			if allow[ch.Provider] {
				filtered = append(filtered, ch)
			}
		}
		cands = filtered
	}
	if len(cands) == 0 {
		return nil, derrors.New(derrors.CodeNotFound, "模型 "+model+" 无匹配 provider 的渠道")
	}
	// 亲和优先
	if primary := h.Selector.PickAffine(ctx, userID, model, cands); primary != nil {
		ordered := make([]*domainchannel.Channel, 0, len(cands))
		ordered = append(ordered, primary)
		for _, c := range cands {
			if c.ID != primary.ID {
				ordered = append(ordered, c)
			}
		}
		cands = ordered
	}
	return cands, nil
}

// baseURLOf channel.BaseURL 为空时回落到 route 默认。
func baseURLOf(ch *domainchannel.Channel, fallback string) string {
	if ch.BaseURL != "" {
		return ch.BaseURL
	}
	return fallback
}

// shouldFailover 判断错误是否发生在"客户端尚未收到字节"阶段，可以换 channel 重试。
func shouldFailover(err error) bool {
	if errors.Is(err, proxy.ErrSSEPreflight) {
		return true
	}
	var up *proxy.ErrUpstreamStatus
	if errors.As(err, &up) {
		return true
	}
	return false
}

// writeFailoverError 所有 candidate 失败时：优先把上游 4xx 错误体直返给客户端（否则返回 502）。
func writeFailoverError(c *gin.Context, err error) {
	var up *proxy.ErrUpstreamStatus
	if errors.As(err, &up) {
		// 直接把上游 4xx/5xx 返回给客户端，保留 body（通常是 JSON 错误）
		contentType := "application/json"
		if !json.Valid(up.Body) {
			contentType = "text/plain; charset=utf-8"
		}
		c.Data(up.Status, contentType, up.Body)
		return
	}
	msg := "所有渠道均失败"
	if err != nil {
		msg = err.Error()
	}
	httperr.AbortStatus(c, http.StatusBadGateway,
		derrors.New(derrors.CodeUpstream, msg))
}

// extractModel 从请求 body 顶层抽 model 字段。非 JSON / 无 model 返回 ""。
func extractModel(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var obj struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &obj); err != nil {
		return ""
	}
	return obj.Model
}
