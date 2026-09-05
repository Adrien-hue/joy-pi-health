//go:build linux

package platform

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	uptimePath    = "/proc/uptime"
	cpuOnlinePath = "/sys/devices/system/cpu/online"
	loadPath      = "/proc/loadavg"
	memoryPath    = "/proc/meminfo"
	networkPath   = "/sys/class/net"
	rootPath      = "/"

	maximumUptimeBytes = 128
	maximumCPUBytes    = 4 * 1024
	maximumLoadBytes   = 256
	maximumMemoryBytes = 64 * 1024
	maximumLinks       = 64
)

type LinuxSource struct{}

func NewLinuxSource() LinuxSource { return LinuxSource{} }

// NewSource returns the production source for the supported Linux runtime.
func NewSource() Source { return NewLinuxSource() }

func (LinuxSource) Hostname() (string, error)  { return os.Hostname() }
func (LinuxSource) Uptime() ([]byte, error)    { return readBounded(uptimePath, maximumUptimeBytes) }
func (LinuxSource) CPUOnline() ([]byte, error) { return readBounded(cpuOnlinePath, maximumCPUBytes) }
func (LinuxSource) LoadAverage() ([]byte, error) {
	return readBounded(loadPath, maximumLoadBytes)
}
func (LinuxSource) MemoryInfo() ([]byte, error) {
	return readBounded(memoryPath, maximumMemoryBytes)
}

func (LinuxSource) RootFilesystem() (Filesystem, error) {
	var state syscall.Statfs_t
	if err := syscall.Statfs(rootPath, &state); err != nil {
		return Filesystem{}, err
	}
	blockSize := state.Frsize
	if blockSize <= 0 {
		blockSize = state.Bsize
	}
	if blockSize <= 0 {
		return Filesystem{}, errors.New("filesystem reported a non-positive block size")
	}
	return Filesystem{
		BlockSize: uint64(blockSize), TotalBlocks: state.Blocks, AvailableBlocks: state.Bavail,
	}, nil
}

func (LinuxSource) Links(ctx context.Context) ([]Link, error) {
	links, err := readLinks(ctx)
	if err != nil {
		return nil, err
	}
	if len(links) > maximumLinks {
		return nil, ErrTooManyLinks
	}
	for index := range links {
		if links[index].Name == "" || filepath.Base(links[index].Name) != links[index].Name || strings.ContainsAny(links[index].Name, `/\\`) {
			return nil, fmt.Errorf("interface name %q cannot be resolved safely", links[index].Name)
		}
		target, err := os.Readlink(filepath.Join(networkPath, links[index].Name))
		if err != nil {
			return nil, fmt.Errorf("resolve interface %q: %w", links[index].Name, err)
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(networkPath, target)
		}
		links[index].DevicePath = filepath.ToSlash(filepath.Clean(target))
	}
	return links, nil
}

func readBounded(path string, maximum int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("%s exceeds %d bytes", path, maximum)
	}
	return data, nil
}
