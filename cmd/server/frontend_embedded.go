package main

import (
	"embed"
	"io/fs"
)

// embeddedFrontend contains the Vue build copied into this package before the
// Go service is compiled.
//
//go:embed all:web
var embeddedFrontend embed.FS

func embeddedFrontendFS() fs.FS {
	frontend, err := fs.Sub(embeddedFrontend, "web")
	if err != nil {
		panic("读取内嵌前端失败: " + err.Error())
	}
	return frontend
}
