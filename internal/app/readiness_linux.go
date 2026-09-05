//go:build linux

package app

import (
	"net"
	"time"
)

func sendSystemdDatagram(socket string, payload []byte, timeout time.Duration) error {
	connection, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: socket, Net: "unixgram"})
	if err != nil {
		return err
	}
	defer connection.Close()

	if err := connection.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	_, err = connection.Write(payload)
	return err
}
