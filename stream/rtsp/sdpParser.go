package rtsp

import (
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"strings"
)

type SDPInfo struct {
	AVType        string
	Codec         string
	TimeScale     int
	Control       string
	RtpMap        int
	Config        []byte
	ParameterSets [][]byte
	PayloadType   int
	SizeLength    int
	IndexLength   int
}

// ParseSDP 解析sdp包
func ParseSDP(sdpRaw string) map[string]*SDPInfo {
	sdpMap := make(map[string]*SDPInfo)

	var info *SDPInfo

	for _, line := range strings.Split(sdpRaw, "\n") {
		line = strings.TrimSpace(line)
		typeVal := strings.SplitN(line, "=", 2)
		if len(typeVal) == 2 {
			fields := strings.SplitN(typeVal[1], " ", 2)
			switch typeVal[0] {
			case "m": // 媒体类型
				if len(fields) > 0 {
					switch fields[0] {
					case "audio", "video":
						sdpMap[fields[0]] = &SDPInfo{AVType: fields[0]}
						info = sdpMap[fields[0]]
						mFields := strings.Split(fields[1], " ")
						if len(mFields) >= 3 {
							info.PayloadType, _ = strconv.Atoi(mFields[2])
						}
					}
				}
			case "a": // 会话级别属性
				if info != nil {
					for _, field := range fields {
						keyVal := strings.SplitN(field, ":", 2)
						if len(keyVal) >= 2 {
							key := keyVal[0]
							val := keyVal[1]
							switch key {
							case "control":
								info.Control = val
							case "rtpmap":
								info.RtpMap, _ = strconv.Atoi(val)
							}
						}

						keyVal = strings.Split(field, "/")
						if len(keyVal) >= 2 {
							key := keyVal[0]
							switch key {
							case "MPEG4-GENERIC":
								info.Codec = "acc"
							case "H264":
								info.Codec = "h264"
							case "H265":
								info.Codec = "h265"
							}
							if i, err := strconv.Atoi(keyVal[1]); err == nil {
								info.TimeScale = i
							}
						}
						keyVal = strings.Split(field, ";")
						if len(keyVal) > 1 {
							for _, field := range fields {
								keyVal := strings.SplitN(field, "=", 2)
								if len(keyVal) == 2 {
									key := strings.TrimSpace(keyVal[0])
									val := keyVal[1]
									switch key {
									case "config":
										info.Config, _ = hex.DecodeString(val)
									case "sizelength":
										info.SizeLength, _ = strconv.Atoi(val)
									case "indexlength":
										info.IndexLength, _ = strconv.Atoi(val)
									case "sprop-parameter-sets":
										fields := strings.Split(val, ",")
										for _, field := range fields {
											val, _ := base64.StdEncoding.DecodeString(field)
											info.ParameterSets = append(info.ParameterSets, val)
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return sdpMap
}
