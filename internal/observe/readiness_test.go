package observe

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/Adrien-hue/joy-pi-health/internal/platform"
)

func TestProbeReadinessUsesUptimeWithoutRootFallback(t *testing.T) {
	t.Parallel()
	source := sourceStub{hostname: "raspberrypi", uptime: []byte("123.4 1.0"), fsErr: errors.New("must not read root filesystem")}
	if err := ProbeReadiness(context.Background(), source); err != nil {
		t.Fatal(err)
	}
}

func TestProbeReadinessUsesRootFallback(t *testing.T) {
	t.Parallel()
	source := sourceStub{
		hostname: "raspberrypi", uptimeErr: fs.ErrNotExist,
		filesystem: platform.Filesystem{BlockSize: 4096, TotalBlocks: 10, AvailableBlocks: 4},
	}
	if err := ProbeReadiness(context.Background(), source); err != nil {
		t.Fatal(err)
	}
}

func TestProbeReadinessRejectsMissingEnvelopeOrUsefulMetric(t *testing.T) {
	t.Parallel()
	tests := []sourceStub{
		{hostErr: fs.ErrNotExist, uptime: []byte("1 1"), filesystem: platform.Filesystem{BlockSize: 1, TotalBlocks: 1}},
		{hostname: "raspberrypi", uptimeErr: fs.ErrNotExist, fsErr: fs.ErrNotExist},
	}
	for _, source := range tests {
		if err := ProbeReadiness(context.Background(), source); err == nil {
			t.Fatal("readiness probe unexpectedly succeeded")
		}
	}
}

func TestProbeReadinessHonorsCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ProbeReadiness(ctx, sourceStub{hostname: "raspberrypi", uptime: []byte("1 1")}); err == nil {
		t.Fatal("cancelled readiness probe succeeded")
	}
}
