//go:build linux && arm64

package platform

import (
	"errors"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

const firmwarePropertyIOCTLRequest = 0xc0086400

func firmwarePropertyIOCTL(file *os.File, buffer []byte) error {
	if len(buffer) == 0 {
		return errors.New("firmware ioctl buffer is empty")
	}
	// The Linux bcm2835 vcio ABI requires ioctl's third argument to be the
	// address of the mutable mailbox-property buffer. This is the single
	// approved unsafe.Pointer conversion at that syscall boundary.
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), firmwarePropertyIOCTLRequest, uintptr(unsafe.Pointer(&buffer[0])))
	runtime.KeepAlive(buffer)
	if errno != 0 {
		return errno
	}
	return nil
}
