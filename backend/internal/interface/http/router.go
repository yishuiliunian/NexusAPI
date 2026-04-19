// Package httpiface 聚合 HTTP 路由并配合 cmd/server 组装完整 Gin Engine。
//
// Deps 按领域分组以降低调用方的心智负担：
//   AuthDeps     – 认证 / 鉴权相关（auth、apikey、user 仓储）
//   RelayDeps    – 中继业务（selector、runner、billing、channel、apikey）
//   TaskDeps     – 异步任务（task service、channel）
//   AdminDeps    – 管理后台（多个仓储）
//   QueryDeps    – 只读查询（usage、ledger，跨前端/admin 复用）
//   InfraDeps    – 通用基础设施（logger）
package httpiface

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	keysapp "github.com/yishuiliunian/nexusapi/backend/internal/app/keys"
	"github.com/yishuiliunian/nexusapi/backend/internal/app/auth"
	auditapp "github.com/yishuiliunian/nexusapi/backend/internal/app/audit"
	billingapp "github.com/yishuiliunian/nexusapi/backend/internal/app/billing"
	oauthapp "github.com/yishuiliunian/nexusapi/backend/internal/app/oauth"
	paymentapp "github.com/yishuiliunian/nexusapi/backend/internal/app/payment"
	"github.com/yishuiliunian/nexusapi/backend/internal/app/redemption"
	relayapp "github.com/yishuiliunian/nexusapi/backend/internal/app/relay"
	subapp "github.com/yishuiliunian/nexusapi/backend/internal/app/subscription"
	taskapp "github.com/yishuiliunian/nexusapi/backend/internal/app/task"
	verifyapp "github.com/yishuiliunian/nexusapi/backend/internal/app/verify"
	auditdomain "github.com/yishuiliunian/nexusapi/backend/internal/domain/audit"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/billing"
	domainchannel "github.com/yishuiliunian/nexusapi/backend/internal/domain/channel"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/payment"
	domaintask "github.com/yishuiliunian/nexusapi/backend/internal/domain/task"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/user"
	"github.com/yishuiliunian/nexusapi/backend/internal/interface/http/admin"
	"github.com/yishuiliunian/nexusapi/backend/internal/interface/http/api"
	"github.com/yishuiliunian/nexusapi/backend/internal/interface/http/middleware"
	"github.com/yishuiliunian/nexusapi/backend/internal/interface/http/passthrough"
	relayhttp "github.com/yishuiliunian/nexusapi/backend/internal/interface/http/relay"
	taskhttp "github.com/yishuiliunian/nexusapi/backend/internal/interface/http/task"
	"github.com/yishuiliunian/nexusapi/backend/internal/interface/http/webhook"
	wsiface "github.com/yishuiliunian/nexusapi/backend/internal/interface/ws"
	"github.com/yishuiliunian/nexusapi/backend/pkg/proxy"
	"github.com/yishuiliunian/nexusapi/backend/pkg/proxy/extractors"
)

// InfraDeps 基础设施。
type InfraDeps struct {
	Logger *zap.Logger
}

// AuthDeps 认证相关。
type AuthDeps struct {
	Auth   *auth.Service
	ApiKey *keysapp.Service
	Users  user.Repository
	Verify *verifyapp.Service
	OAuth  *oauthapp.Service
	// OAuthRedirectURI 生成回调 URL。由 cmd/server 提供。
	OAuthRedirectURI func(provider string) string
	// OAuthPostLoginURL 登录成功后跳转目标，通常是用户端 dashboard。
	OAuthPostLoginURL string
}

// RelayDeps 中继业务。
// Runner 字段在字节级透传架构下已去除；转发由 pkg/proxy + passthrough 承担。
type RelayDeps struct {
	Selector  *relayapp.Selector
	Billing   *billingapp.Engine
	Channels  domainchannel.Repository
	RateLimit middleware.RateLimitConfig // 可留空（Limiter=nil）表示不限流
}

// TaskDeps 异步任务。Task 为 nil 时相关路由不挂载。
type TaskDeps struct {
	Service *taskapp.Service
}

// BillingDeps 计费/兑换码/充值/订阅（面向终端用户）。
type BillingDeps struct {
	Redemption *redemption.Service
	Payments   *paymentapp.Service
	Orders     payment.Repository
	Subs       *subapp.Service
}

// AdminDeps 管理后台。Providers 用于校验 channel.provider 合法性；QuotaAdj 用于走账本调整配额。
type AdminDeps struct {
	Groups    user.GroupRepository
	Channels  domainchannel.Repository
	Prices    billing.ModelPriceRepository
	Users     user.Repository
	Audits    auditdomain.Repository
	Orders    payment.Repository
	Tasks     domaintask.Repository
	Subs      *subapp.Service
	Providers admin.ProviderChecker
	QuotaAdj  admin.QuotaAdjuster
	Pricing   admin.PricingSyncer
	Recorder  *auditapp.Recorder
	// DB 供激活码批量生成等暂无独立 repo 的场景
	DB *gorm.DB
}

// QueryDeps 前端/Admin 共用的只读查询。
type QueryDeps struct {
	Usages  billing.UsageRepository
	Ledgers billing.LedgerRepository
}

// Deps 路由构造入参。
type Deps struct {
	Infra   InfraDeps
	Auth    AuthDeps
	Relay   RelayDeps
	Task    TaskDeps
	Billing BillingDeps
	Admin   AdminDeps
	Query   QueryDeps
	// Proxy 透传代理核心；nil 时只使用旧 adaptor relay。
	Proxy *proxy.Proxy
}

// NewRouter 构建完整路由树。
func NewRouter(d Deps) *gin.Engine {
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Recover(d.Infra.Logger))
	r.Use(middleware.AccessLog(d.Infra.Logger))
	r.Use(middleware.CORS())

	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/readyz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ready"}) })

	// -------- /api/* 前端接口（Session 鉴权） --------
	apiHandler := &api.Handler{
		Auth:              d.Auth.Auth,
		ApiKey:            d.Auth.ApiKey,
		Usages:            d.Query.Usages,
		Ledgers:           d.Query.Ledgers,
		Redemption:        d.Billing.Redemption,
		Payments:          d.Billing.Payments,
		Orders:            d.Billing.Orders,
		Subs:              d.Billing.Subs,
		Verify:            d.Auth.Verify,
		OAuth:             d.Auth.OAuth,
		OAuthRedirectURI:  d.Auth.OAuthRedirectURI,
		OAuthPostLoginURL: d.Auth.OAuthPostLoginURL,
		Users:             d.Auth.Users,
	}
	apiPublic := r.Group("/api")
	apiPrivate := r.Group("/api")
	// CSRF 放在 AuthSession 之前：既保护登录态的 mutating 请求（403），
	// 又放行无 session 的请求交由 AuthSession 统一处理。
	apiPrivate.Use(middleware.CSRF())
	apiPrivate.Use(middleware.AuthSession(d.Auth.Auth))
	apiHandler.Register(apiPublic, apiPrivate)

	// Task 用户端路由
	if d.Task.Service != nil {
		taskUserGroup := r.Group("/api/user")
		taskUserGroup.Use(middleware.CSRF())
		taskUserGroup.Use(middleware.AuthSession(d.Auth.Auth))
		taskH := &taskhttp.Handler{Tasks: d.Task.Service, ApiKey: d.Auth.ApiKey}
		taskH.RegisterAPI(taskUserGroup)
	}

	// -------- /api/admin/* 管理员接口 --------
	adminH := &admin.Handler{
		Users:     d.Admin.Users,
		Groups:    d.Admin.Groups,
		Channels:  d.Admin.Channels,
		Prices:    d.Admin.Prices,
		Usages:    d.Query.Usages,
		Ledgers:   d.Query.Ledgers,
		Audits:    d.Admin.Audits,
		Orders:    d.Admin.Orders,
		Tasks:     d.Admin.Tasks,
		Subs:      d.Admin.Subs,
		Providers: d.Admin.Providers,
		QuotaAdj:  d.Admin.QuotaAdj,
		Pricing:   d.Admin.Pricing,
		DB:        d.Admin.DB,
	}
	adminGroup := r.Group("/api/admin")
	adminGroup.Use(middleware.CSRF())
	adminGroup.Use(middleware.AuthSession(d.Auth.Auth))
	adminGroup.Use(middleware.RequireAdmin())
	if d.Admin.Recorder != nil {
		adminGroup.Use(d.Admin.Recorder.Middleware())
	}
	adminH.Register(adminGroup)

	// -------- /v1/* 中继接口（ApiKey 鉴权） --------
	v1 := r.Group("/v1")
	v1.Use(middleware.AuthApiKey(d.Auth.ApiKey, d.Auth.Users))
	v1.Use(middleware.CheckApiKeyIP())
	v1.Use(middleware.RateLimit(d.Relay.RateLimit))
	relayH := &relayhttp.Handler{
		Selector:  d.Relay.Selector,
		Billing:   d.Relay.Billing,
		ApiKey:    d.Auth.ApiKey,
		Channels:  d.Relay.Channels,
		RateLimit: d.Relay.RateLimit,
	}
	relayH.Register(v1)

	// -------- 字节级透传：/v1/* 原生路径 --------
	// 与 relayH 共存：relayH 只保留 GET /models 本地聚合；所有 POST 路径都走这里。
	if d.Proxy != nil {
		// OpenAI 家族（含 OpenAI 兼容 provider）：所有 POST /v1/* 路径共用一组 providers。
		// 约定：只列已在 infra/provider 注册、且路径格式与 OpenAI 标准兼容的 provider。
		//   - azure-openai：URL 结构不兼容 /v1/*，走独立 /azure/v1/* 路径组
		openaiProviders := []string{
			"openai",
			"deepseek", "moonshot", "qwen", "zhipu", "openrouter",
			"qianfan", // 百度千帆 V2 OpenAI 兼容端点，涵盖文心一言
		}
		// 约定：channel.BaseURL 为"完整到 /v1（或等价前缀）的 URL"，
		// 如 https://api.openai.com/v1、https://open.bigmodel.cn/api/paas/v4。
		// StripPathPrefix="/v1" 把客户端 /v1/xxx 前缀剥掉后拼到 BaseURL。
		mkOpenAI := func(path string, cap billing.Capability) passthrough.Route {
			// 对聊天类能力启用 stream_options 注入，确保流式响应带 usage。
			// embedding/image/rerank 无流式，无需注入。
			injectStreamUsage := cap == billing.CapChat || cap == billing.CapResponses
			return passthrough.Route{
				Path:              path,
				Providers:         openaiProviders,
				AuthMode:          proxy.AuthBearer,
				StripPathPrefix:   "/v1",
				DefaultBaseURL:    "https://api.openai.com/v1",
				Extractor:         extractors.OpenAI,
				BillingCap:        cap,
				InjectStreamUsage: injectStreamUsage,
			}
		}
		routes := []passthrough.Route{
			// Claude 原生
			{
				Path:            "/messages",
				Providers:       []string{"claude"},
				AuthMode:        proxy.AuthXApiKey,
				ExtraHeaders:    map[string]string{"anthropic-version": "2023-06-01"},
				StripPathPrefix: "/v1",
				DefaultBaseURL:  "https://api.anthropic.com/v1",
				Extractor:       extractors.Claude,
				BillingCap:      billing.CapChat,
			},
			// OpenAI 家族
			mkOpenAI("/chat/completions", billing.CapChat),
			mkOpenAI("/responses", billing.CapResponses),
			mkOpenAI("/embeddings", billing.CapEmbedding),
			mkOpenAI("/rerank", billing.CapRerank),
			mkOpenAI("/moderations", billing.CapModeration),
			mkOpenAI("/images/generations", billing.CapImage),
			mkOpenAI("/images/edits", billing.CapImageEdit),
			mkOpenAI("/images/variations", billing.CapImageVariation),
			mkOpenAI("/audio/speech", billing.CapTTS),
			mkOpenAI("/audio/transcriptions", billing.CapSTT),
			mkOpenAI("/audio/translations", billing.CapAudioTranslation),
		}
		passH := &passthrough.Handler{
			Proxy:    d.Proxy,
			Selector: d.Relay.Selector,
			Billing:  d.Relay.Billing,
			ApiKey:   d.Auth.ApiKey,
			Routes:   routes,
		}
		passH.Register(v1)

		// -------- Azure OpenAI 独立路径 /azure/v1/* --------
		//
		// 约定（与 OpenAI 不同）：
		//   客户端：POST /azure/v1/chat/completions?api-version=2024-02-01
		//   channel.BaseURL：含 deployment 部分，例如
		//     https://myres.openai.azure.com/openai/deployments/my-gpt4-deployment
		//   auth：x-api-key
		//   path 剥掉 "/azure/v1" 后追加到 BaseURL。
		//
		// 未采用共享 /v1/* 的原因：Azure 的 URL 结构与 deployment 绑定，而且
		// auth header 用 api-key（不是 Bearer）。独立 Route 避免混淆。
		azureGroup := r.Group("")
		azureGroup.Use(middleware.AuthApiKey(d.Auth.ApiKey, d.Auth.Users))
		azureGroup.Use(middleware.CheckApiKeyIP())
		azureGroup.Use(middleware.RateLimit(d.Relay.RateLimit))
		mkAzure := func(path string, cap billing.Capability) passthrough.Route {
			return passthrough.Route{
				Path:            path,
				Providers:       []string{"azure-openai"},
				AuthMode:        proxy.AuthXApiKey,
				StripPathPrefix: "/azure/v1",
				Extractor:       extractors.OpenAI,
				BillingCap:      cap,
			}
		}
		azureH := &passthrough.Handler{
			Proxy:    d.Proxy,
			Selector: d.Relay.Selector,
			Billing:  d.Relay.Billing,
			ApiKey:   d.Auth.ApiKey,
			Routes: []passthrough.Route{
				mkAzure("/azure/v1/chat/completions", billing.CapChat),
				mkAzure("/azure/v1/embeddings", billing.CapEmbedding),
				mkAzure("/azure/v1/images/generations", billing.CapImage),
				mkAzure("/azure/v1/audio/speech", billing.CapTTS),
				mkAzure("/azure/v1/audio/transcriptions", billing.CapSTT),
			},
		}
		azureH.Register(azureGroup)

		// -------- Gemini 原生 /v1beta/models/{model}:{action} --------
		// 独立 group（不挂在 /v1 下）。
		geminiGroup := r.Group("")
		geminiGroup.Use(middleware.AuthApiKey(d.Auth.ApiKey, d.Auth.Users))
		geminiGroup.Use(middleware.CheckApiKeyIP())
		geminiGroup.Use(middleware.RateLimit(d.Relay.RateLimit))
		geminiH := &passthrough.Handler{
			Proxy:    d.Proxy,
			Selector: d.Relay.Selector,
			Billing:  d.Relay.Billing,
			ApiKey:   d.Auth.ApiKey,
			Routes: []passthrough.Route{
				{
					Path:           "/v1beta/models/*action",
					Method:         "POST",
					Providers:      []string{"gemini"},
					AuthMode:       proxy.AuthGoogleKey,
					DefaultBaseURL: "https://generativelanguage.googleapis.com",
					Extractor:      extractors.Gemini,
					BillingCap:     billing.CapChat,
					ModelResolver:  passthrough.GeminiModelResolver,
				},
			},
		}
		geminiH.Register(geminiGroup)
	}

	// -------- /mj /suno /video 任务中继 --------
	if d.Task.Service != nil {
		taskRelay := r.Group("/")
		taskRelay.Use(middleware.AuthApiKey(d.Auth.ApiKey, d.Auth.Users))
		taskRelay.Use(middleware.CheckApiKeyIP())
		taskRelay.Use(middleware.RateLimit(d.Relay.RateLimit))
		taskRelayH := &taskhttp.Handler{Tasks: d.Task.Service, ApiKey: d.Auth.ApiKey}
		taskRelayH.RegisterRelay(taskRelay)
	}

	// -------- /realtime WebSocket（ApiKey 鉴权） --------
	wsGroup := r.Group("/")
	wsGroup.Use(middleware.AuthApiKey(d.Auth.ApiKey, d.Auth.Users))
	wsGroup.Use(middleware.CheckApiKeyIP())
	wsH := &wsiface.Handler{
		Selector: d.Relay.Selector,
		ApiKey:   d.Auth.ApiKey,
		Log:      d.Infra.Logger,
	}
	wsH.Register(wsGroup)

	// -------- /api/webhook/* 外部网关回调（公开，仅靠签名自证） --------
	if d.Billing.Payments != nil {
		wh := r.Group("/api/webhook")
		(&webhook.Handler{Payments: d.Billing.Payments}).Register(wh)
	}

	return r
}
