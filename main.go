package main

import (
	"embed"
	"os"

	"github.com/dongwlin/dan-tick/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	os.Exit(app.Run(assets))
}
