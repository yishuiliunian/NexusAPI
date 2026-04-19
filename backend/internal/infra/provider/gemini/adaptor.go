// Package gemini 注册 Gemini sync provider。
package gemini

import (
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/relay"
	"github.com/yishuiliunian/nexusapi/backend/internal/infra/provider"
)

// Name provider 名称。
const Name = "gemini"

// Adaptor 最小 SyncAdaptor 实现。
type Adaptor struct{}

func init() { provider.RegisterSync(&Adaptor{}) }

// Name 实现。
func (a *Adaptor) Name() string { return Name }

// Supports 返回支持的能力。
func (a *Adaptor) Supports() []relay.Capability {
	return []relay.Capability{relay.CapChat, relay.CapEmbedding}
}
