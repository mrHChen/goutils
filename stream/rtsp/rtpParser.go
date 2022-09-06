package rtsp

import (
	"encoding/binary"
)

const (
	RTPFixedHeaderLength = 12
)

// RTPInfo rtp 结构
type RTPInfo struct {
	Version        int
	Padding        bool
	Extension      bool
	CSRCCnt        int
	Marker         bool
	PayloadType    int
	SequenceNumber int
	Timestamp      int
	SSRC           int
	Payload        []byte
	PayloadOffset  int
}

/**

  0               1                 2               3             4
  +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
  |V=2|P|X|  CC |M|     PT          | sequence number             |
  +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
  | timestamp                                                     |
  +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
  | synchronization source (SSRC) identifier                      |
  +=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+
  | contributing source (CSRC) identifiers                        |
  | ....                                                          |
  +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
*/

// ParseRTP 解析rtp
func ParseRTP(rtpBytes []byte) *RTPInfo {
	if len(rtpBytes) < RTPFixedHeaderLength {
		return nil
	}

	firstByte := rtpBytes[0]
	secondByte := rtpBytes[1]
	info := &RTPInfo{
		Version:   int(firstByte >> 6),
		Padding:   (firstByte>>5)&1 == 1,
		Extension: (firstByte>>4)&1 == 1,
		CSRCCnt:   int(firstByte & 0x0f),

		Marker:         secondByte>>7 == 1,
		PayloadType:    int(secondByte & 0x7f),
		SequenceNumber: int(binary.BigEndian.Uint16(rtpBytes[2:])),
		Timestamp:      int(binary.BigEndian.Uint32(rtpBytes[4:])),
		SSRC:           int(binary.BigEndian.Uint32(rtpBytes[8:])),
	}

	offset := RTPFixedHeaderLength
	end := len(rtpBytes)
	if end-offset >= 4*info.CSRCCnt {
		offset += 4 * info.CSRCCnt
	}
	if info.Extension && end-offset >= 4 {
		extLen := 4 * int(binary.BigEndian.Uint16(rtpBytes[offset+2:]))
		offset += 4
		if end-offset >= extLen {
			offset += extLen
		}
	}
	if info.Padding && end-offset > 0 {
		paddingLen := int(rtpBytes[end-1])
		if end-offset >= paddingLen {
			end -= paddingLen
		}
	}
	info.Payload = rtpBytes[offset:end]
	info.PayloadOffset = offset
	if end-offset < 1 {
		return nil
	}

	return info
}
