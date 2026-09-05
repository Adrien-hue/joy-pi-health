package platform

import (
	"errors"
	"testing"
)

func TestGetThrottledPropertyProtocol(t *testing.T) {
	t.Parallel()
	request := buildGetThrottledPropertyRequest()
	if word(request, 2) != firmwareGetThrottled || word(request, 4) != 0 || word(request, 5) != 0 {
		t.Fatalf("request = %v", request)
	}
	putWord(request, 1, firmwareResponseOK)
	putWord(request, 4, firmwareTagResponse|4)
	putWord(request, 5, 0x50005)
	mask, err := parseGetThrottledPropertyResponse(request)
	if err != nil || mask != 0x50005 {
		t.Fatalf("parse = %#x, %v", mask, err)
	}
	putWord(request, 4, 4)
	if _, err := parseGetThrottledPropertyResponse(request); !errors.Is(err, ErrMalformedFirmwareResponse) {
		t.Fatalf("malformed parse error = %v", err)
	}
}

func TestGetThrottledCommandProtocol(t *testing.T) {
	t.Parallel()
	request := buildGetThrottledCommandRequest()
	if got := string(request[24 : 24+len("get_throttled")]); got != "get_throttled" {
		t.Fatalf("command = %q", got)
	}
	putWord(request, 1, firmwareResponseOK)
	response := "throttled=0x50005\n"
	putWord(request, 4, firmwareTagResponse|uint32(4+len(response)+1))
	putWord(request, 5, 0)
	copy(request[24:], response)
	request[24+len(response)] = 0
	mask, err := parseGetThrottledCommandResponse(request)
	if err != nil || mask != 0x50005 {
		t.Fatalf("parse = %#x, %v", mask, err)
	}
	copy(request[24:], "not-a-mask\x00")
	if _, err := parseGetThrottledCommandResponse(request); !errors.Is(err, ErrMalformedFirmwareResponse) {
		t.Fatalf("malformed parse error = %v", err)
	}
}
