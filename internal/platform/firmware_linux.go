//go:build linux

package platform

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

const (
	restrictedFirmwareDevice = "/dev/vcio_gencmd"
	generalFirmwareDevice    = "/dev/vcio"
)

type linuxFirmwareTransaction struct{}

func NewFirmwareTransaction() FirmwareTransaction { return linuxFirmwareTransaction{} }

func (linuxFirmwareTransaction) GetThrottled() (uint32, error) {
	restricted, restrictedErr := transactFirmware(restrictedFirmwareDevice, buildGetThrottledCommandRequest(), parseGetThrottledCommandResponse)
	if restrictedErr == nil {
		return restricted, nil
	}
	if !errors.Is(restrictedErr, fs.ErrNotExist) && !errors.Is(restrictedErr, fs.ErrPermission) {
		return 0, restrictedErr
	}
	general, generalErr := transactFirmware(generalFirmwareDevice, buildGetThrottledPropertyRequest(), parseGetThrottledPropertyResponse)
	if generalErr == nil {
		return general, nil
	}
	if errors.Is(restrictedErr, fs.ErrPermission) || errors.Is(generalErr, fs.ErrPermission) {
		return 0, fmt.Errorf("firmware devices denied access: %w", fs.ErrPermission)
	}
	if errors.Is(restrictedErr, fs.ErrNotExist) && errors.Is(generalErr, fs.ErrNotExist) {
		return 0, fmt.Errorf("firmware devices unavailable: %w", fs.ErrNotExist)
	}
	return 0, generalErr
}

func transactFirmware(path string, buffer []byte, parse func([]byte) (uint32, error)) (uint32, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	if err := firmwarePropertyIOCTL(file, buffer); err != nil {
		return 0, err
	}
	return parse(buffer)
}
