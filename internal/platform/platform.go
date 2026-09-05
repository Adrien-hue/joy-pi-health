// Package platform owns narrow, read-only operating-system access for host observation.
package platform

import (
	"context"
	"errors"
)

var ErrTooManyLinks = errors.New("network interface limit exceeded")

var ErrMalformedFirmwareResponse = errors.New("malformed firmware response")

// Source is the finite platform boundary required by the v0.1 generic collectors.
type Source interface {
	Hostname() (string, error)
	Uptime() ([]byte, error)
	CPUOnline() ([]byte, error)
	LoadAverage() ([]byte, error)
	MemoryInfo() ([]byte, error)
	RootFilesystem() (Filesystem, error)
	Links(context.Context) ([]Link, error)
}

// CPUSource is the finite platform boundary required by the trailing CPU
// utilization observer.
type CPUSource interface {
	CPUStat() ([]byte, error)
	BootID() ([]byte, error)
	Uptime() ([]byte, error)
	CPUOnline() ([]byte, error)
}

// ProductionSource is the complete read-only platform surface used by the
// production application.
type ProductionSource interface {
	Source
	CPUSource
}

// ThermalSource is the bounded platform surface used to locate and read the
// Raspberry Pi CPU thermal zone.
type ThermalSource interface {
	ThermalZoneNames() ([]string, error)
	ThermalZoneType(string) ([]byte, error)
	ThermalZoneTemperature(string) ([]byte, error)
}

// FirmwareTransaction performs one coherent, read-only firmware health query.
type FirmwareTransaction interface {
	GetThrottled() (uint32, error)
}

type Filesystem struct {
	BlockSize       uint64
	TotalBlocks     uint64
	AvailableBlocks uint64
}

// Link is one raw kernel network-link observation. Optional attributes use pointers.
type Link struct {
	Index        int32
	Name         string
	HardwareType uint16
	Flags        uint32
	Kind         string
	LinkIndex    *int32
	MasterIndex  *int32
	OperState    *uint8
	RXBytes      *uint64
	TXBytes      *uint64
	DevicePath   string
}
