package main

import (
	"os"

	"github.com/Adrien-hue/joy-pi-health/internal/app"
)

func main() {
	os.Exit(app.Run(os.Args[1:], os.Environ(), os.Stdout, os.Stderr))
}
