//go:build linux

package platform

import (
	"context"
	"encoding/binary"
	"errors"
	"syscall"
	"time"
)

const (
	netlinkHeaderSize = 16
	maximumDatagram   = 64 * 1024
	maximumDumpBytes  = 256 * 1024
)

func readLinks(ctx context.Context) ([]Link, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_RAW|syscall.SOCK_CLOEXEC, syscall.NETLINK_ROUTE)
	if err != nil {
		return nil, err
	}
	defer syscall.Close(fd)
	if err := syscall.Bind(fd, &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK}); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(100 * time.Millisecond)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}

	const sequence = 1
	request := make([]byte, netlinkHeaderSize+ifInfoMessageSize)
	binary.NativeEndian.PutUint32(request[0:4], uint32(len(request)))
	binary.NativeEndian.PutUint16(request[4:6], syscall.RTM_GETLINK)
	binary.NativeEndian.PutUint16(request[6:8], syscall.NLM_F_REQUEST|syscall.NLM_F_DUMP)
	binary.NativeEndian.PutUint32(request[8:12], sequence)
	request[16] = syscall.AF_UNSPEC
	if err := syscall.Sendto(fd, request, 0, &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK}); err != nil {
		return nil, err
	}

	links := make([]Link, 0, 8)
	totalBytes := 0
	buffer := make([]byte, maximumDatagram)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, errors.New("netlink dump timed out")
		}
		timeout := syscall.NsecToTimeval(remaining.Nanoseconds())
		if err := syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &timeout); err != nil {
			return nil, err
		}
		count, _, err := syscall.Recvfrom(fd, buffer, 0)
		if err != nil {
			return nil, err
		}
		if count == len(buffer) {
			return nil, errors.New("netlink datagram may be truncated")
		}
		totalBytes += count
		if totalBytes > maximumDumpBytes {
			return nil, errors.New("netlink dump exceeds byte limit")
		}
		messages, err := syscall.ParseNetlinkMessage(buffer[:count])
		if err != nil {
			return nil, err
		}
		for _, message := range messages {
			if message.Header.Seq != sequence {
				continue
			}
			switch message.Header.Type {
			case syscall.NLMSG_DONE:
				return links, nil
			case syscall.NLMSG_ERROR:
				if err := decodeNetlinkError(message.Data); err != nil {
					return nil, err
				}
			case syscall.RTM_NEWLINK:
				link, err := decodeLink(message.Data)
				if err != nil {
					return nil, err
				}
				links = append(links, link)
				if len(links) > maximumLinks {
					return nil, ErrTooManyLinks
				}
			}
		}
	}
}

func decodeNetlinkError(data []byte) error {
	if len(data) < 4 {
		return errors.New("malformed netlink error")
	}
	code := int32(binary.NativeEndian.Uint32(data[:4]))
	if code == 0 {
		return nil
	}
	if code < 0 {
		code = -code
	}
	return syscall.Errno(code)
}
