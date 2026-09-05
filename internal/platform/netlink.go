package platform

import (
	"encoding/binary"
	"errors"
)

const (
	ifInfoMessageSize = 16
	iflaIfName        = 3
	iflaLink          = 5
	iflaMaster        = 10
	iflaOperState     = 16
	iflaLinkInfo      = 18
	iflaStats64       = 23
	iflaInfoKind      = 1
)

func decodeLink(data []byte) (Link, error) {
	if len(data) < ifInfoMessageSize {
		return Link{}, errors.New("truncated interface message")
	}
	link := Link{
		HardwareType: binary.NativeEndian.Uint16(data[2:4]),
		Index:        int32(binary.NativeEndian.Uint32(data[4:8])),
		Flags:        binary.NativeEndian.Uint32(data[8:12]),
	}
	attributes, err := parseAttributes(data[ifInfoMessageSize:])
	if err != nil {
		return Link{}, err
	}
	for _, attribute := range attributes {
		switch attribute.kind {
		case iflaIfName:
			link.Name = netlinkString(attribute.data)
		case iflaLink:
			value, err := attributeInt32(attribute.data)
			if err != nil {
				return Link{}, err
			}
			link.LinkIndex = &value
		case iflaMaster:
			value, err := attributeInt32(attribute.data)
			if err != nil {
				return Link{}, err
			}
			link.MasterIndex = &value
		case iflaOperState:
			if len(attribute.data) != 1 {
				return Link{}, errors.New("invalid operational state attribute")
			}
			value := attribute.data[0]
			link.OperState = &value
		case iflaLinkInfo:
			kind, err := decodeLinkKind(attribute.data)
			if err != nil {
				return Link{}, err
			}
			link.Kind = kind
		case iflaStats64:
			if len(attribute.data) < 32 {
				return Link{}, errors.New("truncated 64-bit link statistics")
			}
			rx := binary.NativeEndian.Uint64(attribute.data[16:24])
			tx := binary.NativeEndian.Uint64(attribute.data[24:32])
			link.RXBytes, link.TXBytes = &rx, &tx
		}
	}
	if link.Index <= 0 || link.Name == "" {
		return Link{}, errors.New("interface message lacks identity")
	}
	return link, nil
}

type netlinkAttribute struct {
	kind uint16
	data []byte
}

func parseAttributes(data []byte) ([]netlinkAttribute, error) {
	attributes := make([]netlinkAttribute, 0, 8)
	for len(data) != 0 {
		if len(data) < 4 {
			return nil, errors.New("truncated netlink attribute")
		}
		length := int(binary.NativeEndian.Uint16(data[:2]))
		if length < 4 || length > len(data) {
			return nil, errors.New("invalid netlink attribute length")
		}
		kind := binary.NativeEndian.Uint16(data[2:4]) & 0x3fff
		attributes = append(attributes, netlinkAttribute{kind: kind, data: data[4:length]})
		aligned := (length + 3) &^ 3
		if aligned > len(data) {
			return nil, errors.New("truncated netlink attribute padding")
		}
		data = data[aligned:]
	}
	return attributes, nil
}

func decodeLinkKind(data []byte) (string, error) {
	attributes, err := parseAttributes(data)
	if err != nil {
		return "", err
	}
	for _, attribute := range attributes {
		if attribute.kind == iflaInfoKind {
			return netlinkString(attribute.data), nil
		}
	}
	return "", nil
}

func attributeInt32(data []byte) (int32, error) {
	if len(data) != 4 {
		return 0, errors.New("invalid link index attribute")
	}
	return int32(binary.NativeEndian.Uint32(data)), nil
}

func netlinkString(data []byte) string {
	if len(data) != 0 && data[len(data)-1] == 0 {
		data = data[:len(data)-1]
	}
	return string(data)
}
