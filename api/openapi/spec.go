package openapi

import "embed"

// Files содержит встроенные OpenAPI-ресурсы, которые API отдаёт как статические файлы.
//
//go:embed *.yaml
var Files embed.FS
