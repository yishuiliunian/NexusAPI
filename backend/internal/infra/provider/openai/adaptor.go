// Package openai 注册 OpenAI sync provider。
//
// 透传架构下 adaptor 只承担"注册 provider name"的职责，不再转换协议。
package openai

import (
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/relay"
	"github.com/yishuiliunian/nexusapi/backend/internal/infra/provider"
)

// Name provider 名称。
const Name = "openai"

// Adaptor 最小 SyncAdaptor 实现。
type Adaptor struct {
	ProviderName string
}

// New 构造。支持别名（openaicompat 包用来注册 deepseek/moonshot/... 等）。
// baseURL 参数保留向后兼容，实际透传时用的是 channel.BaseURL。
func New(name, _ string) *Adaptor {
	if name == "" {
		name = Name
	}
	return &Adaptor{ProviderName: name}
}

func init() { provider.RegisterSync(New(Name, "")) }

// Name 实现。
func (a *Adaptor) Name() string {
	if a.ProviderName == "" {
		return Name
	}
	return a.ProviderName
}

// Supports 返回支持的能力。
func (a *Adaptor) Supports() []relay.Capability {
	return []relay.Capability{
		relay.CapChat,
		relay.CapResponses,
		relay.CapEmbedding,
		relay.CapImage,
		relay.CapImageEdit,
		relay.CapImageVariation,
		relay.CapTTS,
		relay.CapSTT,
		relay.CapAudioTranslation,
		relay.CapModeration,
	}
}
