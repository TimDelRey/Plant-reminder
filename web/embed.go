// Package web встраивает собранный React-фронтенд (web/dist) в Go-бинарник.
package web

import "embed"

//go:embed all:dist
var Dist embed.FS
