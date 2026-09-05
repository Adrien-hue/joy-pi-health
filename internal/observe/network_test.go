package observe

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/Adrien-hue/joy-pi-health/internal/platform"
)

func TestCollectNetworkTopologyAndOrdering(t *testing.T) {
	t.Parallel()
	up, down := uint8(6), uint8(2)
	rx, tx := uint64(10), uint64(20)
	physicalIndex, bridgeIndex := int32(2), int32(4)
	links := []platform.Link{
		{Index: 8, Name: "vlan10", HardwareType: 1, Kind: "vlan", LinkIndex: &physicalIndex, OperState: &up, RXBytes: &rx, TXBytes: &tx, DevicePath: "/sys/devices/virtual/net/vlan10"},
		{Index: 1, Name: "lo", HardwareType: 772, Flags: loopbackFlag, OperState: &up, RXBytes: &rx, TXBytes: &tx, DevicePath: "/sys/devices/virtual/net/lo"},
		{Index: 3, Name: "dummy0", HardwareType: 1, Kind: "dummy", OperState: &up, RXBytes: &rx, TXBytes: &tx, DevicePath: "/sys/devices/virtual/net/dummy0"},
		{Index: 5, Name: "eth1", HardwareType: 1, MasterIndex: &bridgeIndex, OperState: &down, RXBytes: &rx, TXBytes: &tx, DevicePath: "/sys/devices/platform/soc/ethernet1/net/eth1"},
		{Index: 4, Name: "br0", HardwareType: 1, Kind: "bridge", OperState: &up, RXBytes: &rx, TXBytes: &tx, DevicePath: "/sys/devices/virtual/net/br0"},
		{Index: 2, Name: "eth0", HardwareType: 1, OperState: &up, RXBytes: &rx, TXBytes: &tx, DevicePath: "/sys/devices/platform/soc/ethernet0/net/eth0"},
	}
	got := collectNetwork(context.Background(), sourceStub{links: links})
	if got.state != statePresent {
		t.Fatalf("collectNetwork() = %#v", got)
	}
	wantNames := []string{"br0", "eth0", "eth1", "vlan10"}
	if len(got.value.interfaces) != len(wantNames) {
		t.Fatalf("interfaces = %#v; want names %v", got.value.interfaces, wantNames)
	}
	for index, name := range wantNames {
		if got.value.interfaces[index].name != name {
			t.Errorf("interface %d name = %q; want %q", index, got.value.interfaces[index].name, name)
		}
	}
}

func TestCollectNetworkKeepsInterfaceLocalUnavailability(t *testing.T) {
	t.Parallel()
	link := platform.Link{
		Index: 1, Name: "eth0", HardwareType: 1,
		DevicePath: "/sys/devices/platform/soc/ethernet/net/eth0",
	}
	got := collectNetwork(context.Background(), sourceStub{links: []platform.Link{link}})
	if got.state != statePresent || len(got.value.interfaces) != 1 {
		t.Fatalf("collectNetwork() = %#v", got)
	}
	entry := got.value.interfaces[0]
	for name, result := range map[string]outcomeState{
		"state": entry.state.state, "rx": entry.rxBytes.state, "tx": entry.txBytes.state,
	} {
		if result != stateUnavailable {
			t.Errorf("%s state = %v; want unavailable", name, result)
		}
	}
}

func TestCollectNetworkClassifiesSourceFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		err    error
		reason availabilityReason
	}{
		{err: fs.ErrNotExist, reason: reasonUnsupported},
		{err: fs.ErrPermission, reason: reasonPermissionDenied},
		{err: errors.New("busy"), reason: reasonTemporarilyUnavailable},
		{err: platform.ErrTooManyLinks, reason: reasonTemporarilyUnavailable},
	}
	for _, test := range tests {
		got := collectNetwork(context.Background(), sourceStub{linksErr: test.err})
		if got.state != stateUnavailable || got.reason != test.reason {
			t.Errorf("collectNetwork(%v) = %#v", test.err, got)
		}
	}
}

func TestCollectNetworkRejectsMalformedTopology(t *testing.T) {
	t.Parallel()
	up, counter := uint8(6), uint64(1)
	unknown, self := int32(99), int32(1)
	tests := [][]platform.Link{
		{{Index: 1, Name: "eth0", HardwareType: 1, OperState: &up, RXBytes: &counter, TXBytes: &counter, DevicePath: "relative/path"}},
		{{Index: 1, Name: "vlan0", HardwareType: 1, Kind: "vlan", LinkIndex: &unknown, DevicePath: "/sys/devices/virtual/net/vlan0"}},
		{{Index: 1, Name: "vlan0", HardwareType: 1, Kind: "vlan", LinkIndex: &self, DevicePath: "/sys/devices/virtual/net/vlan0"}},
		{{Index: 1, Name: "eth0", HardwareType: 1, MasterIndex: &unknown, DevicePath: "/sys/devices/platform/eth0"}},
		{{Index: 1, Name: "eth0", DevicePath: "/sys/devices/platform/eth0"}, {Index: 1, Name: "eth1", DevicePath: "/sys/devices/platform/eth1"}},
	}
	for index, links := range tests {
		got := collectNetwork(context.Background(), sourceStub{links: links})
		if got.state != stateUnavailable || got.reason != reasonTemporarilyUnavailable {
			t.Errorf("case %d: collectNetwork() = %#v", index, got)
		}
	}
}

func TestOperationalState(t *testing.T) {
	t.Parallel()
	tests := []struct {
		value *uint8
		state outcomeState
		text  string
	}{
		{value: uint8Pointer(0), state: statePresent, text: "unknown"},
		{value: uint8Pointer(1), state: statePresent, text: "down"},
		{value: uint8Pointer(6), state: statePresent, text: "up"},
		{value: uint8Pointer(7), state: stateUnavailable},
		{value: nil, state: stateUnavailable},
	}
	for _, test := range tests {
		got := operationalState(test.value)
		if got.state != test.state || got.value != test.text {
			t.Errorf("operationalState(%v) = %#v", test.value, got)
		}
	}
}

func uint8Pointer(value uint8) *uint8 { return &value }
