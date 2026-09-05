package observe

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Adrien-hue/joy-pi-health/internal/platform"
)

const (
	ethernetHardwareType = 1
	loopbackFlag         = 1 << 3
	maximumInterfaces    = 64
)

type networkObservation struct {
	interfaces []networkInterfaceObservation
}

type networkInterfaceObservation struct {
	name    string
	state   outcome[string]
	rxBytes outcome[uint64]
	txBytes outcome[uint64]
}

func collectNetwork(ctx context.Context, source platform.Source) outcome[networkObservation] {
	if err := ctx.Err(); err != nil {
		return unavailable[networkObservation](reasonTemporarilyUnavailable, err)
	}
	links, err := source.Links(ctx)
	if err != nil {
		if errors.Is(err, platform.ErrTooManyLinks) {
			return unavailable[networkObservation](reasonTemporarilyUnavailable, err)
		}
		return acquisitionFailure[networkObservation](err, reasonUnsupported)
	}
	if len(links) > maximumInterfaces {
		return unavailable[networkObservation](reasonTemporarilyUnavailable, platform.ErrTooManyLinks)
	}

	topology, err := newLinkTopology(links)
	if err != nil {
		return unavailable[networkObservation](reasonTemporarilyUnavailable, err)
	}
	interfaces := make([]networkInterfaceObservation, 0, len(links))
	for _, link := range links {
		external, err := topology.externallyCapable(link.Index)
		if err != nil {
			return unavailable[networkObservation](reasonTemporarilyUnavailable, err)
		}
		if !external {
			continue
		}
		interfaces = append(interfaces, networkInterfaceObservation{
			name:    link.Name,
			state:   operationalState(link.OperState),
			rxBytes: counterValue(link.RXBytes, "receive byte counter"),
			txBytes: counterValue(link.TXBytes, "transmit byte counter"),
		})
	}
	sort.Slice(interfaces, func(left, right int) bool {
		return interfaces[left].name < interfaces[right].name
	})
	return present(networkObservation{interfaces: interfaces})
}

type linkTopology struct {
	links    map[int32]platform.Link
	members  map[int32][]int32
	resolved map[int32]bool
	visiting map[int32]bool
}

func newLinkTopology(links []platform.Link) (*linkTopology, error) {
	topology := &linkTopology{
		links:    make(map[int32]platform.Link, len(links)),
		members:  make(map[int32][]int32),
		resolved: make(map[int32]bool, len(links)),
		visiting: make(map[int32]bool, len(links)),
	}
	names := make(map[string]struct{}, len(links))
	for _, link := range links {
		if link.Index <= 0 || link.Name == "" || strings.ContainsRune(link.Name, '\x00') || !utf8.ValidString(link.Name) {
			return nil, errors.New("interface identity is invalid")
		}
		if _, duplicate := topology.links[link.Index]; duplicate {
			return nil, errors.New("interface index is duplicated")
		}
		if _, duplicate := names[link.Name]; duplicate {
			return nil, errors.New("interface name is duplicated")
		}
		if !validDevicePath(link.DevicePath) {
			return nil, fmt.Errorf("interface %q has an invalid device path", link.Name)
		}
		topology.links[link.Index] = link
		names[link.Name] = struct{}{}
	}
	for _, link := range links {
		if link.MasterIndex != nil {
			if _, exists := topology.links[*link.MasterIndex]; !exists {
				return nil, fmt.Errorf("interface %q refers to an unknown master", link.Name)
			}
			topology.members[*link.MasterIndex] = append(topology.members[*link.MasterIndex], link.Index)
		}
		if link.Kind == "vlan" {
			if link.LinkIndex == nil {
				return nil, fmt.Errorf("VLAN interface %q lacks its lower link", link.Name)
			}
			if _, exists := topology.links[*link.LinkIndex]; !exists {
				return nil, fmt.Errorf("VLAN interface %q refers to an unknown lower link", link.Name)
			}
		}
	}
	return topology, nil
}

func (topology *linkTopology) externallyCapable(index int32) (bool, error) {
	if result, ok := topology.resolved[index]; ok {
		return result, nil
	}
	if topology.visiting[index] {
		return false, errors.New("interface topology contains a cycle")
	}
	link, ok := topology.links[index]
	if !ok {
		return false, errors.New("interface topology refers to an unknown link")
	}
	topology.visiting[index] = true
	defer delete(topology.visiting, index)

	result := false
	if link.Flags&loopbackFlag == 0 && link.HardwareType == ethernetHardwareType {
		switch {
		case physicalDevicePath(link.DevicePath):
			result = true
		case link.Kind == "vlan":
			var err error
			result, err = topology.externallyCapable(*link.LinkIndex)
			if err != nil {
				return false, err
			}
		case link.Kind == "bond" || link.Kind == "bridge":
			for _, member := range topology.members[index] {
				external, err := topology.externallyCapable(member)
				if err != nil {
					return false, err
				}
				if external {
					result = true
					break
				}
			}
		}
	}
	topology.resolved[index] = result
	return result, nil
}

func validDevicePath(path string) bool {
	return strings.HasPrefix(path, "/sys/devices/") && path != "/sys/devices/"
}

func physicalDevicePath(path string) bool {
	return !strings.HasPrefix(path, "/sys/devices/virtual/")
}

func operationalState(value *uint8) outcome[string] {
	if value == nil {
		return unavailable[string](reasonTemporarilyUnavailable, errors.New("operational state is missing"))
	}
	switch *value {
	case 0:
		return present("unknown")
	case 6:
		return present("up")
	case 1, 2, 3, 4, 5:
		return present("down")
	default:
		return unavailable[string](reasonTemporarilyUnavailable, errors.New("operational state is invalid"))
	}
}

func counterValue(value *uint64, description string) outcome[uint64] {
	if value == nil {
		return unavailable[uint64](reasonTemporarilyUnavailable, fmt.Errorf("%s is missing", description))
	}
	return present(*value)
}
