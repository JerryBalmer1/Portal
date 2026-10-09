// Package codegen holds the oapi-codegen configuration and the go:generate
// directive that turns ../openapi.yaml into internal/gen/api. Nothing here is
// imported; the package exists so `go generate ./...` has a place to run.
//
// internal/gen is READ-ONLY. Change api/openapi.yaml and rerun
// `go generate ./api/...` (or Invoke-Build Generate).
package codegen

//go:generate go tool oapi-codegen -config api.yaml ../openapi.yaml
