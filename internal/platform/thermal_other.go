//go:build !linux

package platform

import "io/fs"

type unsupportedThermalSource struct{}

func NewThermalSource() ThermalSource { return unsupportedThermalSource{} }

func (unsupportedThermalSource) ThermalZoneNames() ([]string, error)    { return nil, fs.ErrNotExist }
func (unsupportedThermalSource) ThermalZoneType(string) ([]byte, error) { return nil, fs.ErrNotExist }
func (unsupportedThermalSource) ThermalZoneTemperature(string) ([]byte, error) {
	return nil, fs.ErrNotExist
}
