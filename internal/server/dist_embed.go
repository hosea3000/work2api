package server

import "embed"

//go:embed all:webdist/dist
var distFS embed.FS
