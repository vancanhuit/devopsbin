// Package api embeds the OpenAPI specification document so the running binary
// can serve it (and the Swagger UI / Redoc consoles that render it) without any
// external files.
package api

import _ "embed"

// spec holds the raw OpenAPI document. It is the same file that drives code
// generation (oapi-codegen for Go, openapi-generator for the TS client), so the
// served contract can never drift from the generated handlers.
//
//go:embed openapi.yaml
var spec []byte

// Spec returns the raw OpenAPI specification document as YAML bytes.
func Spec() []byte {
	return spec
}
