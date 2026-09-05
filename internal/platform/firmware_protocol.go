package platform

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

const (
	firmwareRequestCode  = 0
	firmwareResponseOK   = 0x80000000
	firmwareTagResponse  = 0x80000000
	firmwareGetThrottled = 0x00030046
	firmwareGetCommand   = 0x00030080
	firmwareEndTag       = 0
	firmwareCommandBytes = 4096
)

func buildGetThrottledPropertyRequest() []byte {
	buffer := make([]byte, 7*4)
	putWord(buffer, 0, uint32(len(buffer)))
	putWord(buffer, 1, firmwareRequestCode)
	putWord(buffer, 2, firmwareGetThrottled)
	putWord(buffer, 3, 4)
	putWord(buffer, 4, 0)
	putWord(buffer, 5, 0)
	putWord(buffer, 6, firmwareEndTag)
	return buffer
}

func parseGetThrottledPropertyResponse(buffer []byte) (uint32, error) {
	if len(buffer) != 7*4 || word(buffer, 0) != uint32(len(buffer)) || word(buffer, 1) != firmwareResponseOK || word(buffer, 2) != firmwareGetThrottled || word(buffer, 3) != 4 || word(buffer, 6) != firmwareEndTag {
		return 0, ErrMalformedFirmwareResponse
	}
	responseLength := word(buffer, 4)
	if responseLength&firmwareTagResponse == 0 || responseLength&^firmwareTagResponse != 4 {
		return 0, ErrMalformedFirmwareResponse
	}
	return word(buffer, 5), nil
}

func buildGetThrottledCommandRequest() []byte {
	buffer := make([]byte, (7+firmwareCommandBytes/4)*4)
	putWord(buffer, 0, uint32(len(buffer)))
	putWord(buffer, 1, firmwareRequestCode)
	putWord(buffer, 2, firmwareGetCommand)
	putWord(buffer, 3, firmwareCommandBytes)
	putWord(buffer, 4, 0)
	copy(buffer[6*4:], "get_throttled")
	putWord(buffer, 6+firmwareCommandBytes/4, firmwareEndTag)
	return buffer
}

func parseGetThrottledCommandResponse(buffer []byte) (uint32, error) {
	if len(buffer) != (7+firmwareCommandBytes/4)*4 || word(buffer, 0) != uint32(len(buffer)) || word(buffer, 1) != firmwareResponseOK || word(buffer, 2) != firmwareGetCommand || word(buffer, 3) != firmwareCommandBytes || word(buffer, 6+firmwareCommandBytes/4) != firmwareEndTag {
		return 0, ErrMalformedFirmwareResponse
	}
	responseLength := word(buffer, 4)
	length := int(responseLength &^ firmwareTagResponse)
	if responseLength&firmwareTagResponse == 0 || length < 4 || length > firmwareCommandBytes || word(buffer, 5) != 0 {
		return 0, ErrMalformedFirmwareResponse
	}
	data := buffer[6*4 : 5*4+length]
	if index := bytesIndexByte(data, 0); index >= 0 {
		data = data[:index]
	}
	response := strings.TrimSpace(string(data))
	const prefix = "throttled=0x"
	if !strings.HasPrefix(response, prefix) || len(response) == len(prefix) || len(response) > len(prefix)+8 {
		return 0, ErrMalformedFirmwareResponse
	}
	mask, err := strconv.ParseUint(response[len(prefix):], 16, 32)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrMalformedFirmwareResponse, err)
	}
	return uint32(mask), nil
}

func putWord(buffer []byte, index int, value uint32) {
	binary.LittleEndian.PutUint32(buffer[index*4:], value)
}

func word(buffer []byte, index int) uint32 {
	return binary.LittleEndian.Uint32(buffer[index*4:])
}

func bytesIndexByte(buffer []byte, value byte) int {
	for index, candidate := range buffer {
		if candidate == value {
			return index
		}
	}
	return -1
}
