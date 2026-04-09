package openapi

import "embed"

// Files contains embedded OpenAPI assets served by the API.
//
//go:embed *.yaml
var Files embed.FS

