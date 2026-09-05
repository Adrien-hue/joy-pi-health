//go:build linux

package app

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSendSystemdDatagram(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "notify.sock")
	testSystemdDatagram(t, socket)
}

func TestSendSystemdDatagramToAbstractSocket(t *testing.T) {
	socket := fmt.Sprintf("\x00joy-pi-health-%d-%d", os.Getpid(), time.Now().UnixNano())
	testSystemdDatagram(t, socket)
}

func testSystemdDatagram(t *testing.T, socket string) {
	t.Helper()
	listener, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: socket, Net: "unixgram"})
	if err != nil {
		t.Fatalf("net.ListenUnixgram() error = %v", err)
	}
	defer listener.Close()

	if err := sendSystemdDatagram(socket, []byte(readyPayload), time.Second); err != nil {
		t.Fatalf("sendSystemdDatagram() error = %v", err)
	}
	if err := listener.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	buffer := make([]byte, 32)
	count, _, err := listener.ReadFromUnix(buffer)
	if err != nil {
		t.Fatalf("ReadFromUnix() error = %v", err)
	}
	if got := string(buffer[:count]); got != readyPayload {
		t.Errorf("payload = %q, want %q", got, readyPayload)
	}
}
