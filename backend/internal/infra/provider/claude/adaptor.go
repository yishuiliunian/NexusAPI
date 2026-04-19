// Package claude 注册 Claude sync provider。
//
// 透传架构下 adaptor 只承担"注册 provider name"的职责，不再转换协议。
package claude

import (
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/relay"
	"github.com/yishuiliunian/nexusapi/backend/internal/infra/provider"
)

// Name provider 名称。
const Name = "claude"

// Adaptor 最小 SyncAdaptor 实现。
type Adaptor struct{}

func init() { provider.RegisterSync(&Adaptor{}) }

// Name 实现。
func (a *Adaptor) Name() string { return Name }

// Supports 返回支持的能力。
func (a *Adaptor) Supports() []relay.Capability {
	return []relay.Capability{relay.CapChat}
}
