//go:build !linux

package app

import "time"

func sendSystemdDatagram(string, []byte, time.Duration) error {
	return nil
}
