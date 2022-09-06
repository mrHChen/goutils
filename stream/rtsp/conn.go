package rtsp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"

	"goutils/stream/base"
	"goutils/stream/headers"

	"github.com/pixelbender/go-sdp/sdp"
)

const (
	clientConnReadBufferSize  = 204800
	clientConnWriteBufferSize = 204800
)

type optionsReq struct {
	url          *url.URL
	skipResponse bool
	res          chan clientRes
}

type describeReq struct {
	url *url.URL
	res chan clientRes
}

type setupReq struct {
	forPlay bool
	media   *sdp.Media
	url     *url.URL
	res     chan clientRes
}

type playReq struct {
	url *url.URL
	res chan clientRes
}

type clientRes struct {
	session *sdp.Session
	res     *base.Response
	err     error
}

// ClientConn  客户端连接
type ClientConn struct {
	c         *Client
	scheme    string
	host      string
	ctx       context.Context
	ctxCancel context.CancelFunc

	Conn net.Conn
	cSeq int

	// 验证
	sender *headers.Sender

	connRW  *bufio.ReadWriter
	session string
	// in
	options  chan optionsReq
	describe chan describeReq
	setup    chan setupReq
	play     chan playReq

	done chan struct{}
}

func NewClientConn(c *Client, scheme string, host string) (*ClientConn, error) {
	ctx, ctxCancel := context.WithCancel(context.Background())

	cc := &ClientConn{
		c:         c,
		scheme:    scheme,
		host:      host,
		ctx:       ctx,
		ctxCancel: ctxCancel,

		options:  make(chan optionsReq),
		describe: make(chan describeReq),
		setup:    make(chan setupReq),
		play:     make(chan playReq),

		done: make(chan struct{}),
	}

	go cc.run()
	return cc, nil
}

func (cc *ClientConn) Close() error {
	cc.ctxCancel()
	<-cc.done
	return nil
}

func (cc *ClientConn) Wait() error {
	<-cc.done
	return nil
}

func (cc *ClientConn) connOpen() error {
	var err error
	if cc.scheme != "rtsp" && cc.scheme != "rtsps" {
		return fmt.Errorf("unsupported scheme '%s'", cc.scheme)
	}

	if cc.scheme == "rtsps" && cc.c.TransType != TransTypeTcp {
		return fmt.Errorf("RTSPS can be used only with TCP")
	}

	if !strings.Contains(cc.host, ":") {
		cc.host += ":554"
	}

	// 建立连接
	cc.Conn, err = net.DialTimeout(
		cc.c.TransType.String(),
		cc.host,
		cc.c.options.timeout,
	)
	if err != nil {
		log.Println(fmt.Errorf(" Failed to set up the %s link:%s", cc.c.TransType.String(), err))
		return err
	}
	cc.connRW = bufio.NewReadWriter(
		bufio.NewReaderSize(cc.Conn, clientConnReadBufferSize),
		bufio.NewWriterSize(cc.Conn, clientConnWriteBufferSize),
	)
	return nil
}

func (cc *ClientConn) run() {
	defer close(cc.done)

	for {
		select {
		case req := <-cc.options:
			res, err := cc.doOptions(req.url, req.skipResponse)
			req.res <- clientRes{res: res, err: err}

		case req := <-cc.describe:
			session, res, err := cc.doDescribe(req.url)
			req.res <- clientRes{session: session, res: res, err: err}

		case req := <-cc.setup:
			res, err := cc.doSetup(req.url, req.media)
			req.res <- clientRes{res: res, err: err}

		case req := <-cc.play:
			res, err := cc.doPlay(req.url)
			req.res <- clientRes{res: res, err: err}

		case <-cc.ctx.Done():
			cc.c.Println(" Connection terminated ")
			return
		}
	}

	cc.ctxCancel()

	cc.doClose()
}

// Options  写入一个OPTIONS请求并读取一个响应。
func (cc *ClientConn) Options(u *url.URL, skipResponse bool) (*base.Response, error) {
	cr := make(chan clientRes)
	select {
	case cc.options <- optionsReq{url: u, skipResponse: skipResponse, res: cr}:
		res := <-cr
		return res.res, res.err
	case <-cc.ctx.Done():
		return nil, errors.New(" Connection terminated  [Options] ")
	}
}

func (cc *ClientConn) doOptions(u *url.URL, skipResponse bool) (*base.Response, error) {
	res, err := cc.do(&base.Request{
		Method: base.Options,
		URL:    u,
		Header: base.Header{
			"Require": base.HeaderValue{"implicit-play"},
		},
	}, skipResponse)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (cc *ClientConn) Describe(u *url.URL) (*sdp.Session, *base.Response, error) {
	cr := make(chan clientRes)
	select {
	case cc.describe <- describeReq{url: u, res: cr}:
		res := <-cr
		return res.session, res.res, res.err
	case <-cc.ctx.Done():
		return nil, nil, errors.New(" Connection terminated [Describe]")
	}
}

func (cc *ClientConn) doDescribe(u *url.URL) (*sdp.Session, *base.Response, error) {
	res, err := cc.do(&base.Request{
		Method: base.Describe,
		URL:    u,
		Header: base.Header{
			"Accept": base.HeaderValue{"application/sdp"},
		},
	}, false)
	if err != nil {
		return nil, nil, err
	}

	_sdp, err := sdp.ParseString(string(res.Body))
	if err != nil {
		return nil, nil, err
	}
	cc.c.SDPRaw = string(res.Body)
	return _sdp, res, nil
}

func (cc *ClientConn) Setup(
	u *url.URL,
	media *sdp.Media,
) (*base.Response, error) {
	cr := make(chan clientRes)
	select {
	case cc.setup <- setupReq{
		url:   u,
		res:   cr,
		media: media,
	}:
		res := <-cr
		return res.res, res.err
	case <-cc.ctx.Done():
		return nil, errors.New(" Connection terminated  [Setup]")
	}
}

func (cc *ClientConn) doSetup(u *url.URL, media *sdp.Media) (*base.Response, error) {
	var rtpPort, rtcpPort int
	control := media.Attributes.Get("control")
	codec := media.Format[0].Name
	if media.Type == "video" {
		rtpPort, rtcpPort = cc.c.vRtpPort, cc.c.vRtcpPort
		cc.c.VControl, cc.c.VCodec = control, codec
	}
	if media.Type == "audio" {
		rtpPort, rtcpPort = cc.c.aRtpPort, cc.c.aRtcpPort
		cc.c.AControl, cc.c.ACodec = control, codec
	}

	l := ""
	if strings.Index(strings.ToLower(control), "rtsp://") == 0 {
		l = control
	} else {
		l = strings.TrimRight(u.String(), "/") + "/" + strings.TrimLeft(control, "/")
	}

	transport := fmt.Sprintf("RTP/AVP/TCP;unicast;interleaved=%d-%d", rtpPort, rtcpPort)

	cc.c.Println(
		"Parse DESCRIBE response, VIDEO control:%s, codec:%s, url:%s,Session:%s,rtpPort:%d,rtpPort:%d",
		control, codec, l, rtpPort, rtcpPort)
	ur, _ := url.Parse(l)
	res, err := cc.do(&base.Request{
		Method: base.Setup,
		URL:    ur,
		Header: base.Header{
			"Transport": base.HeaderValue{transport},
		},
	}, false)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (cc *ClientConn) Play(u *url.URL) (*base.Response, error) {
	cr := make(chan clientRes)
	select {
	case cc.play <- playReq{url: u, res: cr}:
		res := <-cr
		return res.res, res.err
	case <-cc.ctx.Done():
		return nil, errors.New(" Connection terminated [Play] ")
	}
}

func (cc *ClientConn) doPlay(u *url.URL) (*base.Response, error) {
	res, err := cc.do(&base.Request{
		Method: base.Play,
		URL:    u,
		Header: base.Header{},
	}, false)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// do 发送请求  req request  skipResponse 跳过返回
func (cc *ClientConn) do(req *base.Request, skipResponse bool) (*base.Response, error) {

	if cc.Conn == nil {
		err := cc.connOpen()
		if err != nil {
			return nil, err
		}
	}

	if req.Header == nil {
		req.Header = make(base.Header)
	}

	req.Header["User-Agent"] = base.HeaderValue{cc.c.options.Agent}

	if cc.session != "" {
		req.Header["Session"] = base.HeaderValue{cc.session}
	}

	if cc.sender != nil {
		cc.sender.AddAuthorization(req)
	}

	cc.cSeq++
	req.Header["CSeq"] = base.HeaderValue{strconv.FormatInt(int64(cc.cSeq), 10)}

	cc.c.Println("client [c->s] \n %v", req)

	err := req.Write(cc.connRW.Writer)
	if err != nil {
		return nil, err
	}

	if skipResponse {
		return nil, nil
	}

	res := &base.Response{}
	res, err = res.Read(cc.connRW.Reader)
	if err != nil {
		return nil, err
	}

	cc.c.Println("client [s->c] \n %v", res)

	if v, ok := res.Header["Session"]; ok {
		var sx headers.Session
		err := sx.Unmarshal(v)
		if err != nil {
			return nil, fmt.Errorf("invalid session header: %s", err)
		}
		cc.session = sx.Session
	}

	// if required, send request again with authentication
	if res.StatusCode == base.StatusUnauthorized && req.URL.User != nil && cc.sender == nil {
		pass, _ := req.URL.User.Password()
		user := req.URL.User.Username()

		sender, err := headers.NewSender(res.Header["WWW-Authenticate"], user, pass)
		if err != nil {
			return nil, fmt.Errorf("unable to setup authentication: %s", err)
		}
		cc.sender = sender

		return cc.do(req, false)
	}
	return res, nil
}

func (cc *ClientConn) doClose() {
	if cc.Conn != nil {
		cc.connRW.Flush()
		cc.Conn.Close()
		cc.Conn = nil
	}
}
