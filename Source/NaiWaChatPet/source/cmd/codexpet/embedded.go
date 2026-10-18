package main

import (
	"embed"
	"io/fs"
)

//go:embed embedded_assets
var embeddedFiles embed.FS

func init() {
	files, err := fs.Sub(embeddedFiles, "embedded_assets")
	if err != nil {
		panic(err)
	}
	bundledAssets = files
}
