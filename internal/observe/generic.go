package observe

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Adrien-hue/joy-pi-health/internal/platform"
)

const (
	maximumUptimeInput = 128
	maximumCPUInput    = 4 * 1024
	maximumLoadInput   = 256
	maximumMemoryInput = 64 * 1024
)

type hostObservation struct {
	hostname outcome[string]
	uptime   outcome[uint64]
}

type loadObservation struct {
	oneMinute      float64
	fiveMinutes    float64
	fifteenMinutes float64
}

type cpuLoadObservation struct {
	logicalCPUCount outcome[uint64]
	load            outcome[loadObservation]
}

type memoryObservation struct {
	totalBytes     uint64
	availableBytes uint64
	usedBytes      uint64
}

type filesystemObservation memoryObservation

func collectHost(ctx context.Context, source platform.Source) hostObservation {
	if err := ctx.Err(); err != nil {
		return hostObservation{hostname: unavailable[string](reasonTemporarilyUnavailable, err), uptime: unavailable[uint64](reasonTemporarilyUnavailable, err)}
	}
	hostname, err := source.Hostname()
	hostnameResult := present(hostname)
	if err != nil {
		hostnameResult = acquisitionFailure[string](err, reasonUnsupported)
	} else if hostname == "" || !utf8.ValidString(hostname) {
		hostnameResult = unavailable[string](reasonTemporarilyUnavailable, errors.New("hostname is empty or invalid UTF-8"))
	}

	uptimeData, err := source.Uptime()
	if err != nil {
		return hostObservation{hostname: hostnameResult, uptime: acquisitionFailure[uint64](err, reasonUnsupported)}
	}
	uptime, err := parseUptime(uptimeData)
	if err != nil {
		return hostObservation{hostname: hostnameResult, uptime: unavailable[uint64](reasonTemporarilyUnavailable, err)}
	}
	return hostObservation{hostname: hostnameResult, uptime: present(uptime)}
}

func collectCPULoad(ctx context.Context, source platform.Source) cpuLoadObservation {
	if err := ctx.Err(); err != nil {
		return cpuLoadObservation{
			logicalCPUCount: unavailable[uint64](reasonTemporarilyUnavailable, err),
			load:            unavailable[loadObservation](reasonTemporarilyUnavailable, err),
		}
	}
	cpuData, err := source.CPUOnline()
	cpuResult := outcome[uint64]{}
	if err != nil {
		cpuResult = acquisitionFailure[uint64](err, reasonUnsupported)
	} else if count, parseErr := parseCPUList(cpuData); parseErr != nil {
		cpuResult = unavailable[uint64](reasonTemporarilyUnavailable, parseErr)
	} else {
		cpuResult = present(count)
	}

	loadData, err := source.LoadAverage()
	if err != nil {
		return cpuLoadObservation{logicalCPUCount: cpuResult, load: acquisitionFailure[loadObservation](err, reasonUnsupported)}
	}
	load, err := parseLoad(loadData)
	if err != nil {
		return cpuLoadObservation{logicalCPUCount: cpuResult, load: unavailable[loadObservation](reasonTemporarilyUnavailable, err)}
	}
	return cpuLoadObservation{logicalCPUCount: cpuResult, load: present(load)}
}

func collectMemory(ctx context.Context, source platform.Source) outcome[memoryObservation] {
	if err := ctx.Err(); err != nil {
		return unavailable[memoryObservation](reasonTemporarilyUnavailable, err)
	}
	data, err := source.MemoryInfo()
	if err != nil {
		return acquisitionFailure[memoryObservation](err, reasonUnsupported)
	}
	memory, err := parseMemory(data)
	if err != nil {
		return unavailable[memoryObservation](reasonTemporarilyUnavailable, err)
	}
	return present(memory)
}

func collectRootFilesystem(ctx context.Context, source platform.Source) outcome[filesystemObservation] {
	if err := ctx.Err(); err != nil {
		return unavailable[filesystemObservation](reasonTemporarilyUnavailable, err)
	}
	value, err := source.RootFilesystem()
	if err != nil {
		return acquisitionFailure[filesystemObservation](err, reasonUnsupported)
	}
	if value.BlockSize == 0 || value.TotalBlocks > math.MaxUint64/value.BlockSize || value.AvailableBlocks > math.MaxUint64/value.BlockSize {
		return unavailable[filesystemObservation](reasonTemporarilyUnavailable, errors.New("filesystem block values overflow"))
	}
	total := value.TotalBlocks * value.BlockSize
	available := value.AvailableBlocks * value.BlockSize
	if available > total {
		return unavailable[filesystemObservation](reasonTemporarilyUnavailable, errors.New("filesystem available bytes exceed total bytes"))
	}
	return present(filesystemObservation{totalBytes: total, availableBytes: available, usedBytes: total - available})
}

func parseUptime(data []byte) (uint64, error) {
	if len(data) == 0 || len(data) > maximumUptimeInput {
		return 0, errors.New("uptime input is empty or oversized")
	}
	fields := bytes.Fields(data)
	if len(fields) < 1 {
		return 0, errors.New("uptime value is missing")
	}
	parts := bytes.Split(fields[0], []byte{'.'})
	if len(parts) > 2 || len(parts[0]) == 0 || len(parts) == 2 && (len(parts[1]) == 0 || !decimalDigits(parts[1])) || !decimalDigits(parts[0]) {
		return 0, errors.New("uptime is not a non-negative decimal")
	}
	return strconv.ParseUint(string(parts[0]), 10, 64)
}

func parseCPUList(data []byte) (uint64, error) {
	if len(data) == 0 || len(data) > maximumCPUInput {
		return 0, errors.New("CPU list is empty or oversized")
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return 0, errors.New("CPU list is empty")
	}
	var count uint64
	var previous uint64
	for index, item := range strings.Split(text, ",") {
		bounds := strings.Split(item, "-")
		if len(bounds) > 2 || bounds[0] == "" {
			return 0, errors.New("CPU list contains an invalid range")
		}
		start, err := strconv.ParseUint(bounds[0], 10, 64)
		if err != nil {
			return 0, errors.New("CPU list contains an invalid number")
		}
		end := start
		if len(bounds) == 2 {
			if bounds[1] == "" {
				return 0, errors.New("CPU list contains an invalid range")
			}
			end, err = strconv.ParseUint(bounds[1], 10, 64)
			if err != nil || end < start {
				return 0, errors.New("CPU list contains an invalid range")
			}
		}
		if index != 0 && start <= previous {
			return 0, errors.New("CPU list is overlapping or unordered")
		}
		length := end - start + 1
		if length == 0 || count > math.MaxUint64-length {
			return 0, errors.New("CPU count overflows")
		}
		count += length
		previous = end
	}
	if count == 0 {
		return 0, errors.New("CPU list contains no processors")
	}
	return count, nil
}

func parseLoad(data []byte) (loadObservation, error) {
	if len(data) == 0 || len(data) > maximumLoadInput {
		return loadObservation{}, errors.New("load input is empty or oversized")
	}
	fields := bytes.Fields(data)
	if len(fields) < 3 {
		return loadObservation{}, errors.New("load input has fewer than three values")
	}
	values := [3]float64{}
	for index := range values {
		value, err := strconv.ParseFloat(string(fields[index]), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return loadObservation{}, errors.New("load input contains an invalid value")
		}
		values[index] = value
	}
	return loadObservation{oneMinute: values[0], fiveMinutes: values[1], fifteenMinutes: values[2]}, nil
}

func parseMemory(data []byte) (memoryObservation, error) {
	if len(data) == 0 || len(data) > maximumMemoryInput {
		return memoryObservation{}, errors.New("memory input is empty or oversized")
	}
	values := map[string]uint64{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 1024), maximumMemoryInput)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}
		name := strings.TrimSuffix(fields[0], ":")
		if name != "MemTotal" && name != "MemAvailable" {
			continue
		}
		if _, duplicate := values[name]; duplicate || len(fields) != 3 || fields[2] != "kB" {
			return memoryObservation{}, fmt.Errorf("invalid or duplicate %s entry", name)
		}
		kilobytes, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil || kilobytes > math.MaxUint64/1024 {
			return memoryObservation{}, fmt.Errorf("invalid %s value", name)
		}
		values[name] = kilobytes * 1024
	}
	if err := scanner.Err(); err != nil {
		return memoryObservation{}, err
	}
	total, totalOK := values["MemTotal"]
	available, availableOK := values["MemAvailable"]
	if !totalOK || !availableOK || available > total {
		return memoryObservation{}, errors.New("required memory values are missing or inconsistent")
	}
	return memoryObservation{totalBytes: total, availableBytes: available, usedBytes: total - available}, nil
}

func decimalDigits(data []byte) bool {
	for _, value := range data {
		if value < '0' || value > '9' {
			return false
		}
	}
	return true
}
