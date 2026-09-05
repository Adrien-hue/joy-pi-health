package app

import "io"

// Run is the process-level application boundary.
//
// The repository foundation does not start the service yet. Subsequent
// implementation phases will add startup and lifecycle behavior here.
func Run(_ []string, _ []string, _ io.Writer, _ io.Writer) int {
	return 0
}
