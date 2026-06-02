package migrations

import "embed"

// FS contains all up/down SQL migration files.
//
//go:embed *.sql
var FS embed.FS
