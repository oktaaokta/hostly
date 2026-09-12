package webassets

import "embed"

// Dist holds the built React SPA. Regenerate with `make frontend`.
//
//go:embed dist
var Dist embed.FS