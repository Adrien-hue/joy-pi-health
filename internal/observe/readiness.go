package observe

import (
	"context"
	"errors"
	"fmt"

	"github.com/Adrien-hue/joy-pi-health/internal/platform"
)

// ProbeReadiness performs the minimum observation needed to prove that a valid
// partial snapshot envelope can be formed. It deliberately excludes network
// enumeration, firmware access, and CPU utilization sampling.
func ProbeReadiness(ctx context.Context, source platform.Source) error {
	if ctx == nil || source == nil {
		return errors.New("readiness probe dependencies are required")
	}
	host := collectHost(ctx, source)
	if err := validateHost(host); err != nil {
		return fmt.Errorf("readiness host observation: %w", err)
	}
	if host.hostname.state != statePresent {
		return errors.New("required hostname is unavailable")
	}
	if host.uptime.state == statePresent {
		return nil
	}
	root := collectRootFilesystem(ctx, source)
	if err := validateFilesystemOutcome(root); err != nil {
		return fmt.Errorf("readiness root-filesystem observation: %w", err)
	}
	if root.state == statePresent {
		return nil
	}
	return errors.New("no useful readiness metric is available")
}
