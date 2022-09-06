package base

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"net/url"
	"strconv"

	"github.com/mrHChen/goutils/stream/utils"
)

const (
	RtspProtocol10 = "RTSP/1.0"
)

// Method RTSP请求的方法
type Method string

// 方法
const (
	Announce     Method = "ANNOUNCE"
	Describe     Method = "DESCRIBE"
	GetParameter Method = "GET_PARAMETER"
	Options      Method = "OPTIONS"
	Pause        Method = "PAUSE"
	Play         Method = "PLAY"
	Record       Method = "RECORD"
	Setup        Method = "SETUP"
	SetParameter Method = "SET_PARAMETER"
	Teardown     Method = "TEARDOWN"
)

// Request RTSP请求
type Request struct {
	// 请求方法
	Method Method

	// 请求地址
	URL *url.URL

	// 请求头
	Header Header

	Body []byte
}

// Write 写一个请求
func (req Request) Write(bw *bufio.Writer) error {
	urStr := req.URL.String()
	_, err := bw.Write([]byte(string(req.Method) + " " + urStr + " " + RtspProtocol10 + "\r\n"))
	if err != nil {
		return err
	}

	if len(req.Body) != 0 {
		req.Header["Content-Length"] = HeaderValue{strconv.FormatInt(int64(len(req.Body)), 10)}
	}

	err = req.Header.write(bw)
	if err != nil {
		return err
	}

	err = body(req.Body).write(bw)
	if err != nil {
		return err
	}

	return bw.Flush()
}

func (req Request) Read(rb *bufio.Reader, buf1 []byte) *Request {
	byts, err := utils.ReadBytesLimited(rb, ' ', 64)
	if err != nil {
		log.Println(fmt.Sprintf("readBytesLimited error:%s", err))
		return nil
	}
	req.Method = Method(fmt.Sprintf("%s%s", buf1, byts[:len(byts)-1]))

	if req.Method == "" {
		return nil
	}

	byts, err = utils.ReadBytesLimited(rb, ' ', 2048)
	if err != nil {
		return nil
	}
	rawURL := string(byts[:len(byts)-1])

	ur, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	req.URL = ur

	byts, err = utils.ReadBytesLimited(rb, '\r', 64)
	if err != nil {
		return nil
	}
	proto := byts[:len(byts)-1]

	if string(proto) != RtspProtocol10 {
		return nil
	}

	err = utils.ReadByteEqual(rb, '\n')
	if err != nil {
		return nil
	}

	err = req.Header.read(rb)
	if err != nil {
		return nil
	}

	err = (*body)(&req.Body).read(req.Header, rb)
	if err != nil {
		return nil
	}
	return &req
}

// String implements fmt.Stringer.
func (req Request) String() string {
	buf := bytes.NewBuffer(nil)
	req.Write(bufio.NewWriter(buf))
	return buf.String()
}
