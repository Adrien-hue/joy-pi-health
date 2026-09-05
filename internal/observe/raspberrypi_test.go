package observe

import (
	"context"
	"io/fs"
	"testing"
)

type thermalSourceStub struct {
	names []string
	types map[string][]byte
	temps map[string][]byte
	err   error
}

func (source thermalSourceStub) ThermalZoneNames() ([]string, error) { return source.names, source.err }
func (source thermalSourceStub) ThermalZoneType(name string) ([]byte, error) {
	value, ok := source.types[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return value, nil
}
func (source thermalSourceStub) ThermalZoneTemperature(name string) ([]byte, error) {
	value, ok := source.temps[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return value, nil
}

func TestCollectSoCTemperatureSelectsCPUZone(t *testing.T) {
	t.Parallel()
	source := thermalSourceStub{
		names: []string{"thermal_zone0", "thermal_zone1"},
		types: map[string][]byte{"thermal_zone0": []byte("gpu-thermal\n"), "thermal_zone1": []byte("cpu-thermal\n")},
		temps: map[string][]byte{"thermal_zone1": []byte("42750\n")},
	}
	result := collectSoCTemperature(context.Background(), source)
	if result.state != statePresent || result.value != 42.75 {
		t.Fatalf("temperature = %#v", result)
	}
}

func TestCollectSoCTemperatureAcceptsZeroAndPhysicalNegativeValues(t *testing.T) {
	t.Parallel()
	for _, value := range []struct {
		raw  string
		want float64
	}{{"0", 0}, {"-1250", -1.25}, {"-273150", -273.15}} {
		source := thermalSourceStub{
			names: []string{"thermal_zone0"},
			types: map[string][]byte{"thermal_zone0": []byte("cpu-thermal")},
			temps: map[string][]byte{"thermal_zone0": []byte(value.raw)},
		}
		result := collectSoCTemperature(context.Background(), source)
		if result.state != statePresent || result.value != value.want {
			t.Errorf("temperature %q = %#v", value.raw, result)
		}
	}
}

func TestCollectSoCTemperatureFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		source thermalSourceStub
		reason availabilityReason
	}{
		{"unsupported", thermalSourceStub{}, reasonUnsupported},
		{"permission", thermalSourceStub{err: fs.ErrPermission}, reasonPermissionDenied},
		{"malformed", thermalSourceStub{names: []string{"thermal_zone0"}, types: map[string][]byte{"thermal_zone0": []byte("cpu-thermal")}, temps: map[string][]byte{"thermal_zone0": []byte("nope")}}, reasonTemporarilyUnavailable},
		{"below absolute zero", thermalSourceStub{names: []string{"thermal_zone0"}, types: map[string][]byte{"thermal_zone0": []byte("cpu-thermal")}, temps: map[string][]byte{"thermal_zone0": []byte("-273151")}}, reasonTemporarilyUnavailable},
		{"duplicates", thermalSourceStub{names: []string{"thermal_zone0", "thermal_zone1"}, types: map[string][]byte{"thermal_zone0": []byte("cpu-thermal"), "thermal_zone1": []byte("cpu-thermal")}}, reasonTemporarilyUnavailable},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result := collectSoCTemperature(context.Background(), test.source)
			if result.state != stateUnavailable || result.reason != test.reason {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestFirmwareHealthBitMapping(t *testing.T) {
	t.Parallel()
	for combination := uint32(0); combination < 16; combination++ {
		mask := (combination&1)<<0 | (combination&2)<<1 | (combination&4)<<14 | (combination&8)<<15
		value := firmwareHealthFromMask(mask | 1<<31)
		if value.undervoltageActive != (combination&1 != 0) || value.thermalThrottlingActive != (combination&2 != 0) || value.undervoltageOccurredSinceBoot != (combination&4 != 0) || value.thermalThrottlingOccurredSinceBoot != (combination&8 != 0) {
			t.Fatalf("combination %#x from mask %#x = %#v", combination, mask, value)
		}
	}
}

func TestCollectSoCTemperatureNilSourceIsDefect(t *testing.T) {
	t.Parallel()
	if result := collectSoCTemperature(context.Background(), nil); result.state != stateDefect || result.cause == nil {
		t.Fatalf("result = %#v", result)
	}
}
