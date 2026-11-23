// Package migrations встраивает SQL-миграции и сид-данные в бинарник.
package migrations

import "embed"

//go:embed sql/*.sql data/*.csv
var FS embed.FS
