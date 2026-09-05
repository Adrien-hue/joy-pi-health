package platform

import (
	"encoding/binary"
	"testing"
)

func TestDecodeLink(t *testing.T) {
	t.Parallel()
	linkIndex, masterIndex := int32(2), int32(7)
	statistics := make([]byte, 32)
	binary.NativeEndian.PutUint64(statistics[16:24], 1<<63+5)
	binary.NativeEndian.PutUint64(statistics[24:32], ^uint64(0)-1)
	linkInfo := netlinkAttributeBytes(iflaInfoKind, append([]byte("vlan"), 0))
	data := make([]byte, ifInfoMessageSize)
	binary.NativeEndian.PutUint16(data[2:4], 1)
	binary.NativeEndian.PutUint32(data[4:8], 9)
	binary.NativeEndian.PutUint32(data[8:12], 3)
	data = append(data,
		netlinkAttributeBytes(iflaIfName, append([]byte("vlan10"), 0))...)
	data = append(data, netlinkAttributeBytes(iflaLink, int32Bytes(linkIndex))...)
	data = append(data, netlinkAttributeBytes(iflaMaster, int32Bytes(masterIndex))...)
	data = append(data, netlinkAttributeBytes(iflaOperState, []byte{6})...)
	data = append(data, netlinkAttributeBytes(iflaLinkInfo, linkInfo)...)
	data = append(data, netlinkAttributeBytes(iflaStats64, statistics)...)

	got, err := decodeLink(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Index != 9 || got.Name != "vlan10" || got.HardwareType != 1 || got.Flags != 3 || got.Kind != "vlan" {
		t.Fatalf("decodeLink() = %#v", got)
	}
	if got.LinkIndex == nil || *got.LinkIndex != linkIndex || got.MasterIndex == nil || *got.MasterIndex != masterIndex {
		t.Fatalf("decodeLink() indices = %#v, %#v", got.LinkIndex, got.MasterIndex)
	}
	if got.OperState == nil || *got.OperState != 6 || got.RXBytes == nil || *got.RXBytes != 1<<63+5 || got.TXBytes == nil || *got.TXBytes != ^uint64(0)-1 {
		t.Fatalf("decodeLink() optional values = %#v", got)
	}
}

func TestDecodeLinkRejectsMalformedMessages(t *testing.T) {
	t.Parallel()
	validHeader := make([]byte, ifInfoMessageSize)
	binary.NativeEndian.PutUint32(validHeader[4:8], 1)
	tests := [][]byte{
		make([]byte, ifInfoMessageSize-1),
		validHeader,
		append(append([]byte(nil), validHeader...), []byte{3, 0, iflaIfName, 0}...),
		append(append([]byte(nil), validHeader...), netlinkAttributeBytes(iflaOperState, []byte{1, 2})...),
		append(append([]byte(nil), validHeader...), netlinkAttributeBytes(iflaStats64, make([]byte, 31))...),
	}
	for index, data := range tests {
		if _, err := decodeLink(data); err == nil {
			t.Errorf("case %d: decodeLink() succeeded", index)
		}
	}
}

func netlinkAttributeBytes(kind uint16, data []byte) []byte {
	length := 4 + len(data)
	attribute := make([]byte, (length+3)&^3)
	binary.NativeEndian.PutUint16(attribute[0:2], uint16(length))
	binary.NativeEndian.PutUint16(attribute[2:4], kind)
	copy(attribute[4:], data)
	return attribute
}

func int32Bytes(value int32) []byte {
	data := make([]byte, 4)
	binary.NativeEndian.PutUint32(data, uint32(value))
	return data
}
