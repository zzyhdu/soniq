package migrations

import "embed"

// UpFiles contains all application up migrations.
//
//go:embed *.up.sql
var UpFiles embed.FS
