// Package migrations embeds the SQL schema history in the binary.
//
// Embedding rather than reading from disk means a deployed binary always
// carries the exact schema it was built and tested against, and that a
// migration cannot be altered on a running host without replacing the binary.
package migrations

import "embed"

// Files holds every migration, applied in lexical filename order.
//
//go:embed *.sql
var Files embed.FS
