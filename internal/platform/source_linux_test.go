//go:build linux

package platform

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxSourceReadsHostSources(t *testing.T) {
	source := NewLinuxSource()
	if hostname, err := source.Hostname(); err != nil || hostname == "" {
		t.Fatalf("Hostname() = %q, %v", hostname, err)
	}
	for name, read := range map[string]func() ([]byte, error){
		"uptime": source.Uptime, "CPU online": source.CPUOnline,
		"CPU stat": source.CPUStat, "boot ID": source.BootID,
		"load average": source.LoadAverage, "memory info": source.MemoryInfo,
	} {
		if data, err := read(); err != nil || len(data) == 0 {
			t.Errorf("%s read = %d bytes, %v", name, len(data), err)
		}
	}
	if filesystem, err := source.RootFilesystem(); err != nil || filesystem.BlockSize == 0 || filesystem.TotalBlocks == 0 {
		t.Errorf("RootFilesystem() = %#v, %v", filesystem, err)
	}
	if links, err := source.Links(context.Background()); err != nil || len(links) == 0 {
		t.Errorf("Links() = %d links, %v", len(links), err)
	}
}

func TestReadBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(path, []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := readBounded(path, 5); err != nil || string(got) != "12345" {
		t.Fatalf("readBounded() = %q, %v", got, err)
	}
	if _, err := readBounded(path, 4); err == nil {
		t.Fatal("readBounded() accepted oversized input")
	}
}

func TestReadFirstLineBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(path, []byte("first\nsecond line is ignored\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := readFirstLineBounded(path, 6); err != nil || string(got) != "first\n" {
		t.Fatalf("readFirstLineBounded() = %q, %v", got, err)
	}
	if _, err := readFirstLineBounded(path, 5); err == nil {
		t.Fatal("readFirstLineBounded() accepted oversized first line")
	}
}
