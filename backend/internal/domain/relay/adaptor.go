// Package relay 定义 provider 适配器最小契约。
//
// 历史上这里有 OpenAI 协议伞下的完整 DTO（Request/Response/Message/...），
// 用于把各家 provider 协议转成统一 OpenAI 格式再转回去。字节级透传架构下
// 这些 DTO 全部删除，只保留：
//
//   - SyncAdaptor：最小契约（Name/Supports），用于 provider 注册表及渠道
//     选路过滤；实际 HTTP 转发由 pkg/proxy 完成，不经过 adaptor
//   - TaskAdaptor：异步任务（Midjourney/Suno/视频）协议自成一派，保留
//     Submit/Query 两个方法的原有接口
//   - Upstream：传给 TaskAdaptor 的 channel DTO
//
// 新增"sync provider"只需要在 infra/provider 下写一个 init()：
//     provider.RegisterSync(&Stub{name: "foobar"})
// 然后在 channel 里配 BaseURL+Credentials+Provider=foobar 即可。
package relay

import "context"

// Capability 能力形态。仅用于 TaskAdaptor 与管理端展示。
type Capability string

const (
	CapChat              Capability = "chat"
	CapResponses         Capability = "responses"
	CapEmbedding         Capability = "embedding"
	CapRerank            Capability = "rerank"
	CapImage             Capability = "image"
	CapImageEdit         Capability = "image_edit"
	CapImageVariation    Capability = "image_variation"
	CapTTS               Capability = "tts"
	CapSTT               Capability = "stt"
	CapAudioTranslation  Capability = "audio_translation"
	CapModeration        Capability = "moderation"
	CapRealtime          Capability = "realtime"
	CapTask              Capability = "task"
)

// Upstream 传给 TaskAdaptor 的渠道信息 DTO。
// 不暴露整个 domain/channel 包。
type Upstream struct {
	ID          uint64
	Provider    string
	BaseURL     string
	Credentials string
}

// SyncAdaptor sync provider 最小契约。
// 透传架构下实际 HTTP 由 pkg/proxy 做；这里只用于"注册表里认这个 provider 名字"。
type SyncAdaptor interface {
	// Name 供应商标识，唯一。
	Name() string
	// Supports 该 provider 支持的能力集合（管理台展示 / 模型规划用）。
	Supports() []Capability
}

// ModelLister 供应商可选能力：拉上游 /v1/models（或等价端点）返回模型 ID 列表。
// 不是所有 provider 都支持（Midjourney/Suno 等异步任务 provider 没这个概念），
// 因此独立于 SyncAdaptor，遵循 ISP。管理台"同步模型"功能据此判断是否可用。
type ModelLister interface {
	ListModels(ctx context.Context, up Upstream) ([]string, error)
}

// ---------- TaskAdaptor（Midjourney / Suno / 视频任务） ----------

// TaskStatus 异步任务状态。
type TaskStatus string

const (
	TaskPending TaskStatus = "pending"
	TaskRunning TaskStatus = "running"
	TaskSuccess TaskStatus = "success"
	TaskFailed  TaskStatus = "failed"
)

// TaskResult 任务轮询结果。
type TaskResult struct {
	Status   TaskStatus
	Progress int
	Result   any
	Error    string
}

// TaskAdaptor 异步任务 provider 契约。
type TaskAdaptor interface {
	Name() string
	Submit(ctx context.Context, up Upstream, action string, input any) (externalID string, err error)
	Query(ctx context.Context, up Upstream, externalID string) (*TaskResult, error)
}
