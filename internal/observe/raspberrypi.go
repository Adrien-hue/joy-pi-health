package observe

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/Adrien-hue/joy-pi-health/internal/platform"
)

const raspberryPiThermalZoneType = "cpu-thermal"

type firmwareHealth struct {
	thermalThrottlingActive            bool
	thermalThrottlingOccurredSinceBoot bool
	undervoltageActive                 bool
	undervoltageOccurredSinceBoot      bool
}

type raspberryPiObservation struct {
	temperature outcome[float64]
	firmware    outcome[firmwareHealth]
}

type raspberryPiCollector interface {
	Collect(context.Context, *cycleToken) raspberryPiObservation
}

type productionRaspberryPiCollector struct {
	thermal  platform.ThermalSource
	firmware firmwareObservationProvider
}

func (collector productionRaspberryPiCollector) Collect(ctx context.Context, token *cycleToken) raspberryPiObservation {
	return raspberryPiObservation{
		temperature: collectSoCTemperature(ctx, collector.thermal),
		firmware:    collector.firmware.Observe(ctx, token),
	}
}

type unavailableRaspberryPiCollector struct{}

func (unavailableRaspberryPiCollector) Collect(context.Context, *cycleToken) raspberryPiObservation {
	cause := errors.New("Raspberry Pi observation is unavailable")
	return raspberryPiObservation{
		temperature: unavailable[float64](reasonTemporarilyUnavailable, cause),
		firmware:    unavailable[firmwareHealth](reasonTemporarilyUnavailable, cause),
	}
}

func collectSoCTemperature(ctx context.Context, source platform.ThermalSource) outcome[float64] {
	if source == nil {
		return defect[float64](errors.New("thermal source is nil"))
	}
	names, err := source.ThermalZoneNames()
	if err != nil {
		return acquisitionFailure[float64](err, reasonUnsupported)
	}
	match := ""
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return acquisitionFailure[float64](err, reasonTemporarilyUnavailable)
		}
		kind, err := source.ThermalZoneType(name)
		if err != nil {
			return acquisitionFailure[float64](err, reasonTemporarilyUnavailable)
		}
		if strings.TrimSpace(string(kind)) != raspberryPiThermalZoneType {
			continue
		}
		if match != "" {
			return unavailable[float64](reasonTemporarilyUnavailable, errors.New("multiple cpu-thermal zones are present"))
		}
		match = name
	}
	if match == "" {
		return unavailable[float64](reasonUnsupported, errors.New("cpu-thermal zone is absent"))
	}
	data, err := source.ThermalZoneTemperature(match)
	if err != nil {
		return acquisitionFailure[float64](err, reasonTemporarilyUnavailable)
	}
	millidegrees, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return unavailable[float64](reasonTemporarilyUnavailable, fmt.Errorf("parse SoC temperature: %w", err))
	}
	if millidegrees < -273150 {
		return unavailable[float64](reasonTemporarilyUnavailable, errors.New("SoC temperature is below absolute zero"))
	}
	temperature := float64(millidegrees) / 1000
	if math.IsNaN(temperature) || math.IsInf(temperature, 0) {
		return unavailable[float64](reasonTemporarilyUnavailable, errors.New("SoC temperature is not finite"))
	}
	return present(temperature)
}

func firmwareHealthFromMask(mask uint32) firmwareHealth {
	return firmwareHealth{
		undervoltageActive:                 mask&(1<<0) != 0,
		thermalThrottlingActive:            mask&(1<<2) != 0,
		undervoltageOccurredSinceBoot:      mask&(1<<16) != 0,
		thermalThrottlingOccurredSinceBoot: mask&(1<<18) != 0,
	}
}

func validateRaspberryPi(value raspberryPiObservation) error {
	if err := validateOutcome(value.temperature); err != nil {
		return fmt.Errorf("SoC temperature outcome: %w", err)
	}
	if value.temperature.state == statePresent && (math.IsNaN(value.temperature.value) || math.IsInf(value.temperature.value, 0) || value.temperature.value < -273.15) {
		return errors.New("present SoC temperature is invalid")
	}
	if err := validateOutcome(value.firmware); err != nil {
		return fmt.Errorf("firmware outcome: %w", err)
	}
	return nil
}
