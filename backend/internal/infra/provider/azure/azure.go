// Package azure 注册 Azure OpenAI sync provider。
//
// 注意：Azure 的 URL 结构特殊（/openai/deployments/{dep}/... + api-version 查询参数），
// 与 OpenAI 标准路径 /v1/chat/completions 不兼容。透传架构下 Azure 暂不支持；
// 如需支持，可：
//   1) channel.BaseURL 直接填完整 Azure endpoint（包含 deployments/{dep}）
//   2) 客户端把 api-version 作为 query 参数带来
//   3) 或新建一个 /azure/* 命名空间 + 专用 Route 做路径映射
package azure

import (
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/relay"
	"github.com/yishuiliunian/nexusapi/backend/internal/infra/provider"
)

// Name provider 名称。
const Name = "azure-openai"

// Adaptor 最小 SyncAdaptor 实现。
type Adaptor struct{}

func init() { provider.RegisterSync(&Adaptor{}) }

// Name 实现。
func (a *Adaptor) Name() string { return Name }

// Supports 返回支持的能力。
func (a *Adaptor) Supports() []relay.Capability {
	return []relay.Capability{
		relay.CapChat,
		relay.CapEmbedding,
		relay.CapImage,
		relay.CapTTS,
		relay.CapSTT,
	}
}
