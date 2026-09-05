//go:build !linux

package platform

import "io/fs"

type unsupportedFirmwareTransaction struct{}

func NewFirmwareTransaction() FirmwareTransaction { return unsupportedFirmwareTransaction{} }

func (unsupportedFirmwareTransaction) GetThrottled() (uint32, error) { return 0, fs.ErrNotExist }
