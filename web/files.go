package web

import "embed"

//go:embed login.html register.html forgot.html reset.html search.html queue.html title.html css js
var Files embed.FS
