package portal

import _ "embed"

// BaseYAML is portal.base.yaml, the product defaults every tenant's overlay
// is merged over (internal/tenant). Embedded: a tenant never edits it.
//
//go:embed portal.base.yaml
var BaseYAML []byte

// ConfigSchema is schema/portal.schema.json, which validates the base and
// every tenant overlay on boot.
//
//go:embed schema/portal.schema.json
var ConfigSchema []byte
