// Package schemas embeds Sure's published JSON schemas.
package schemas

import "embed"

//go:embed *.schema.json
var FS embed.FS
