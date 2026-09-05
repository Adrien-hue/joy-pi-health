package app

import (
	"errors"
	"io"

	"github.com/Adrien-hue/joy-pi-health/internal/config"
)

// Run is the process-level application boundary.
//
// This phase validates configuration but does not start runtime services yet.
func Run(args []string, environment []string, stdout, stderr io.Writer) int {
	_, err := config.Resolve(args, environment)
	if errors.Is(err, config.ErrHelp) {
		_, _ = io.WriteString(stdout, config.HelpText)
		return 0
	}
	if err != nil {
		_, _ = io.WriteString(stderr, "joy-pi-health: configuration error: "+err.Error()+"\n")
		return 2
	}

	return 0
}
