// Package providers 通过空导入注册所有内置 provider。
//
// 在 main.go 中 blank-import 本包即可激活全部适配器：
//
//	import _ "github.com/yishuiliunian/nexusapi/backend/internal/infra/provider/providers"
package providers

import (
	// Sync providers
	_ "github.com/yishuiliunian/nexusapi/backend/internal/infra/provider/azure"
	_ "github.com/yishuiliunian/nexusapi/backend/internal/infra/provider/claude"
	_ "github.com/yishuiliunian/nexusapi/backend/internal/infra/provider/gemini"
	_ "github.com/yishuiliunian/nexusapi/backend/internal/infra/provider/openai"
	_ "github.com/yishuiliunian/nexusapi/backend/internal/infra/provider/openaicompat"

	// Task providers
	_ "github.com/yishuiliunian/nexusapi/backend/internal/infra/provider/midjourney"
	_ "github.com/yishuiliunian/nexusapi/backend/internal/infra/provider/suno"
)
