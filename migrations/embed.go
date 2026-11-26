package migrations

import "embed"

//go:embed sql/*.sql data/*.csv
var FS embed.FS
