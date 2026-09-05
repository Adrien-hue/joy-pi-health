//go:build linux

package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	thermalRoot         = "/sys/class/thermal"
	maximumThermalZones = 64
	maximumThermalValue = 128
)

type linuxThermalSource struct {
	root string
}

func NewThermalSource() ThermalSource { return linuxThermalSource{root: thermalRoot} }

func (source linuxThermalSource) ThermalZoneNames() ([]string, error) {
	entries, err := os.ReadDir(source.root)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0)
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "thermal_zone") {
			continue
		}
		index := strings.TrimPrefix(name, "thermal_zone")
		if index == "" {
			continue
		}
		if _, err := strconv.ParseUint(index, 10, 31); err != nil {
			continue
		}
		names = append(names, name)
		if len(names) > maximumThermalZones {
			return nil, fmt.Errorf("thermal zone count exceeds %d", maximumThermalZones)
		}
	}
	return names, nil
}

func (source linuxThermalSource) ThermalZoneType(name string) ([]byte, error) {
	path, err := source.zonePath(name, "type")
	if err != nil {
		return nil, err
	}
	return readBounded(path, maximumThermalValue)
}

func (source linuxThermalSource) ThermalZoneTemperature(name string) ([]byte, error) {
	path, err := source.zonePath(name, "temp")
	if err != nil {
		return nil, err
	}
	return readBounded(path, maximumThermalValue)
}

func (source linuxThermalSource) zonePath(name, attribute string) (string, error) {
	if !strings.HasPrefix(name, "thermal_zone") || filepath.Base(name) != name || strings.ContainsAny(name, `/\\`) {
		return "", fmt.Errorf("invalid thermal zone name %q", name)
	}
	index := strings.TrimPrefix(name, "thermal_zone")
	if index == "" {
		return "", fmt.Errorf("invalid thermal zone name %q", name)
	}
	if _, err := strconv.ParseUint(index, 10, 31); err != nil {
		return "", fmt.Errorf("invalid thermal zone name %q", name)
	}
	return filepath.Join(source.root, name, attribute), nil
}
