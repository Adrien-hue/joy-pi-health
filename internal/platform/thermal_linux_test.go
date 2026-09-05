//go:build linux

package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxThermalSourceEnumeratesAndBoundsReads(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	zone := filepath.Join(root, "thermal_zone2")
	if err := os.Mkdir(zone, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(zone, "type"), []byte("cpu-thermal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(zone, "temp"), []byte("42500\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := linuxThermalSource{root: root}
	names, err := source.ThermalZoneNames()
	if err != nil || len(names) != 1 || names[0] != "thermal_zone2" {
		t.Fatalf("names = %v, %v", names, err)
	}
	if value, err := source.ThermalZoneType(names[0]); err != nil || string(value) != "cpu-thermal\n" {
		t.Fatalf("type = %q, %v", value, err)
	}
	if value, err := source.ThermalZoneTemperature(names[0]); err != nil || string(value) != "42500\n" {
		t.Fatalf("temperature = %q, %v", value, err)
	}
	if err := os.WriteFile(filepath.Join(zone, "temp"), make([]byte, maximumThermalValue+1), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := source.ThermalZoneTemperature(names[0]); err == nil {
		t.Fatal("oversized temperature was accepted")
	}
}
