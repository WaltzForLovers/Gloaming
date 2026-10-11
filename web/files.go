package web

import "embed"

//go:embed login.html register.html search.html queue.html css js
var Files embed.FS
