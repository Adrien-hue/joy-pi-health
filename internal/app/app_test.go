package app

import (
	"bytes"
	"testing"
)

func TestRunFoundation(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if status := Run(nil, nil, &stdout, &stderr); status != 0 {
		t.Fatalf("Run() status = %d, want 0", status)
	}
	if stdout.Len() != 0 {
		t.Errorf("Run() wrote unexpected stdout: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("Run() wrote unexpected stderr: %q", stderr.String())
	}
}
