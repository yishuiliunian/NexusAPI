// providerChecker 实现 admin.ProviderChecker，桥接到 infra/provider 的全局注册表。
package main

import (
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/relay"
	"github.com/yishuiliunian/nexusapi/backend/internal/infra/provider"
)

type providerChecker struct{}

func (providerChecker) Exists(name string) bool {
	return provider.Sync(name) != nil || provider.Task(name) != nil
}

func (providerChecker) Names() []string { return provider.Names() }

// Lister 返回 provider 的 ModelLister。未注册或未实现则 nil。
func (providerChecker) Lister(name string) relay.ModelLister {
	return provider.Lister(name)
}
