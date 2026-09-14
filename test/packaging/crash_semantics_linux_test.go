//go:build linux

package packaging_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

const crashSemanticsHelper = "JOY_PI_HEALTH_CRASH_SEMANTICS_HELPER"

func TestManagedCrashModeSeparatesIntentionalExitFromRuntimeFailure(t *testing.T) {
	t.Run("intentional exit 2 remains an exit code", func(t *testing.T) {
		status := runCrashSemanticsHelper(t, "exit2")
		if !status.Exited() || status.ExitStatus() != 2 {
			t.Fatalf("intentional exit 2 produced wait status %v", status)
		}
	})

	t.Run("unrecovered panic terminates with SIGABRT", func(t *testing.T) {
		status := runCrashSemanticsHelper(t, "panic")
		if !status.Signaled() || status.Signal() != syscall.SIGABRT {
			t.Fatalf("runtime failure produced wait status %v", status)
		}
	})
}

func TestCrashSemanticsHelper(t *testing.T) {
	switch os.Getenv(crashSemanticsHelper) {
	case "":
		t.Skip("subprocess helper")
	case "exit2":
		os.Exit(2)
	case "panic":
		go func() { panic("unexpected runtime failure fixture") }()
		select {}
	default:
		os.Exit(99)
	}
}

func runCrashSemanticsHelper(t *testing.T, mode string) syscall.WaitStatus {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "sh", "-c", `ulimit -c 0; exec "$0" -test.run '^TestCrashSemanticsHelper$'`, os.Args[0])
	command.Env = append(os.Environ(), "GOTRACEBACK=crash", crashSemanticsHelper+"="+mode)
	err := command.Run()
	if ctx.Err() != nil {
		t.Fatalf("crash-semantics helper timed out: %v", ctx.Err())
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("crash-semantics helper returned %v", err)
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok {
		t.Fatalf("crash-semantics helper returned unsupported process state %T", exitErr.Sys())
	}
	return status
}
