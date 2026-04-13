//go:build tools

// Пакет tools фиксирует версии инструментов генерации кода в go.mod.
// Сами инструменты вызываются через make-таргеты, а не импортируются в рантайме.
package tools

import _ "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen"
