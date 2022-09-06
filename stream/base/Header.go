package base

import (
	"bufio"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"goutils/stream/utils"
)

const (
	headerMaxEntryCount  = 255
	headerMaxKeyLength   = 512
	headerMaxValueLength = 2048
)

func headerKeyNormalize(in string) string {
	switch strings.ToLower(in) {
	case "rtp-info":
		return "RTP-Info"

	case "www-authenticate":
		return "WWW-Authenticate"

	case "cseq":
		return "CSeq"
	}
	return http.CanonicalHeaderKey(in)
}

// HeaderValue is an header value.
type HeaderValue []string

// Header 报头是一个RTSP阅读器，存在于请求和响应中。
type Header map[string]HeaderValue

// read 读取头
func (h *Header) read(br *bufio.Reader) error {
	*h = make(Header)
	count := 0

	for {
		byt, err := br.ReadByte()
		if err != nil {
			return err
		}

		if byt == '\r' {
			err := utils.ReadByteEqual(br, '\n')
			if err != nil {
				return err
			}
			break
		}

		if count >= headerMaxEntryCount {
			return fmt.Errorf("headers count exceeds %d", headerMaxEntryCount)
		}

		key := string([]byte{byt})
		byts, err := utils.ReadBytesLimited(br, ':', headerMaxKeyLength-1)
		if err != nil {
			return fmt.Errorf("value is missing")
		}

		key += string(byts[:len(byts)-1])
		key = headerKeyNormalize(key)

		for {
			byt, err := br.ReadByte()
			if err != nil {
				return err
			}

			if byt != ' ' {
				break
			}
		}

		br.UnreadByte()
		byts, err = utils.ReadBytesLimited(br, '\r', headerMaxValueLength)
		if err != nil {
			return err
		}
		val := string(byts[:len(byts)-1])

		err = utils.ReadByteEqual(br, '\n')
		if err != nil {
			return err
		}

		(*h)[key] = append((*h)[key], val)
		count++
	}
	return nil
}

func (h Header) write(wb *bufio.Writer) error {
	keys := make([]string, len(h))
	for key := range h {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, key := range keys {
		for _, val := range h[key] {
			// 每行以 "\r\n" 结束
			_, err := wb.Write([]byte(key + ": " + val + "\r\n"))
			if err != nil {
				return err
			}
		}
	}

	// "\r\n" 为结束
	_, err := wb.Write([]byte("\r\n"))
	if err != nil {
		return err
	}
	return nil
}
