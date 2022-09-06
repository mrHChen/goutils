package rtsp

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"goutils/stream/base"
	"goutils/stream/headers"

	"github.com/teris-io/shortid"
)

type TransType int

const (
	TransTypeTcp TransType = iota
	TransTypeUdp
)

func (t TransType) String() string {
	switch t {
	case TransTypeTcp:
		return "tcp"
	case TransTypeUdp:
		return "udp"
	}
	return "unknown"
}

type SessionType int

const (
	SESSION_TYPE_PUSHER SessionType = iota
	SESSION_TYPE_PLAYER
)

func (st SessionType) String() string {
	switch st {
	case SESSION_TYPE_PUSHER:
		return "pusher"
	case SESSION_TYPE_PLAYER:
		return "player"
	}
	return "unknow"
}

type RTPPack struct {
	Type   RTPType
	Buffer *bytes.Buffer
}

type RTPType int

const (
	RTP_TYPE_AUDIO RTPType = iota
	RTP_TYPE_VIDEO
	RTP_TYPE_AUDIOCONTROL
	RTP_TYPE_VIDEOCONTROL
)

func (rt RTPType) String() string {
	switch rt {
	case RTP_TYPE_AUDIO:
		return "audio"
	case RTP_TYPE_VIDEO:
		return "video"
	case RTP_TYPE_AUDIOCONTROL:
		return "audio control"
	case RTP_TYPE_VIDEOCONTROL:
		return "video control"
	}
	return "unknow"
}

type Session struct {
	ID        string
	options   SessionOptions
	connRW    *bufio.ReadWriter
	connWLock sync.RWMutex
	TransType TransType

	Type SessionType
	URL  *url.URL

	VCodec   string
	VControl string
	ACodec   string
	AControl string

	cSeq          int
	sessionID     string
	authorization bool

	SDPRaw string
	SDPMap map[string]*SDPInfo

	Stopped bool
	// 新的推流器连接时，如果已有同一个推流器是否关闭
	closeOld bool

	aRtpPort  int
	aRtcpPort int
	vRtpPort  int
	vRtcpPort int

	Pusher *Pusher
	Player *Player

	RTPHandles  []func(*RTPPack)
	StopHandles []func()
}

type SessionOptions struct {
	Server *Server
	conn   net.Conn
	// 新的推流器连接时，如果已有同一个推流器是否关闭
	CloseOld bool

	Authorization bool

	Timeout time.Duration
}

func (s *Session) String() string {
	return fmt.Sprintf("session[%v][%v][%s][%s][%s]", s.Type, s.TransType, s.URL.Path, s.ID, s.options.conn.RemoteAddr().String())
}

func NewSession(options SessionOptions) *Session {
	session := &Session{
		ID:      shortid.MustGenerate(),
		options: options,
		connRW: bufio.NewReadWriter(
			bufio.NewReaderSize(options.conn, clientConnReadBufferSize),
			bufio.NewWriterSize(options.conn, clientConnWriteBufferSize),
		),
		RTPHandles:  make([]func(*RTPPack), 0),
		StopHandles: make([]func(), 0),
		vRtpPort:    -1,
		vRtcpPort:   -1,
		aRtpPort:    -1,
		aRtcpPort:   -1,
	}
	return session
}

func (s *Session) Start() {
	defer s.Stop()
	buf1 := make([]byte, 1)
	buf2 := make([]byte, 2)

	timer := time.Unix(0, 0)

	for !s.Stopped {
		if _, err := io.ReadFull(s.connRW, buf1); err != nil {
			log.Println(fmt.Errorf("session readFull error :%s", err))
			return
		}
		if buf1[0] == 0x24 { // rtp
			if _, err := io.ReadFull(s.connRW, buf1); err != nil {
				log.Println(fmt.Errorf("read rtp 1 error:%s", err))
				return
			}

			if _, err := io.ReadFull(s.connRW, buf2); err != nil {
				log.Println(fmt.Errorf("read rtp 2 error:%s", err))
				return
			}

			channel := int(buf1[0])
			rtpLen := int(binary.BigEndian.Uint16(buf2))
			rtpBytes := make([]byte, rtpLen)

			if _, err := io.ReadFull(s.connRW, rtpBytes); err != nil {
				log.Println(fmt.Errorf("read body error:%s", err))
				return
			}

			rtpBuf := bytes.NewBuffer(rtpBytes)
			var pack *RTPPack
			switch channel {
			case s.aRtpPort:
				pack = &RTPPack{
					Type:   RTP_TYPE_AUDIO,
					Buffer: rtpBuf,
				}
				elapsed := time.Now().Sub(timer)
				if elapsed >= 30*time.Second {
					log.Println("音频rtp包")
					timer = time.Now()
				}
			case s.aRtcpPort:
				pack = &RTPPack{
					Type:   RTP_TYPE_AUDIOCONTROL,
					Buffer: rtpBuf,
				}
			case s.vRtpPort:
				pack = &RTPPack{
					Type:   RTP_TYPE_VIDEO,
					Buffer: rtpBuf,
				}
				elapsed := time.Now().Sub(timer)
				if elapsed >= 30*time.Second {
					log.Println("视频rtp包")
					timer = time.Now()
				}
			case s.vRtcpPort:
				pack = &RTPPack{
					Type:   RTP_TYPE_VIDEOCONTROL,
					Buffer: rtpBuf,
				}
			default:
				log.Println(fmt.Sprintf("unknow rtp pack type,%v", channel))
				continue
			}

			for _, h := range s.RTPHandles {
				h(pack)
			}
		} else { // rtsp
			for !s.Stopped {
				req := &base.Request{}
				req = req.Read(s.connRW.Reader, buf1)
				s.handleRequest(req)
				break
			}
		}
	}
}

func (s *Session) Stop() {
	if s.Stopped {
		return
	}

	s.Stopped = true

	for _, h := range s.StopHandles {
		h()
	}

	if s.options.conn != nil {
		s.connRW.Flush()
		s.options.conn.Close()
		s.options.conn = nil
	}
}

func (s *Session) handleRequest(req *base.Request) {
	log.Println(fmt.Sprintf("server [c->s] \n %v", req))
	res := &base.Response{
		StatusCode: base.StatusOK,
	}
	defer func() {
		if err := recover(); err != nil {
			log.Println(fmt.Errorf("handleRequest err : %v", err))
			res.StatusCode = base.StatusInternalServerError
		}
		log.Println(fmt.Sprintf("server [s->c] \n %s", res))

		s.connWLock.Lock()
		res.Write(s.connRW.Writer)
		s.connWLock.Unlock()
		switch req.Method {
		case base.Play, base.Record:
			switch s.Type {
			case SESSION_TYPE_PLAYER:
				if s.Pusher.HasPlayer(s.Player) {
					s.Player.Pause(false)
				} else {
					s.Pusher.AddPlayer(s.Player)
				}
			}
		}
	}()

	if res.Header == nil {
		res.Header = make(base.Header)
	}

	res.Header["CSeq"] = req.Header["CSeq"]

	res.Header["Session"] = base.HeaderValue{s.ID}

	if req.Method != base.Options {
		if s.authorization {
			if _, ok := req.Header["Authorization"]; ok {
				if err := s.CheckAuth(req, res); err != nil {
					return
				}
			}
		}
	}
	switch req.Method {
	case base.Options:
		public := []string{
			string(base.Describe),
			string(base.Setup),
			string(base.Teardown),
			string(base.Play),
			string(base.Pause),
			string(base.Options),
			string(base.Announce),
			string(base.Record),
		}
		res.Header["Public"] = base.HeaderValue{strings.Join(public, ", ")}
	case base.Announce:
		s.Type = SESSION_TYPE_PUSHER
		s.URL = req.URL

		s.SDPRaw = string(req.Body)
		s.SDPMap = ParseSDP(s.SDPRaw)
		if sdp, ok := s.SDPMap["audio"]; ok {
			s.AControl = sdp.Control
			s.ACodec = sdp.Codec
			log.Println(fmt.Sprintf("audio codec[%s]\n", s.ACodec))
		}

		if sdp, ok := s.SDPMap["video"]; ok {
			s.VControl = sdp.Control
			s.VCodec = sdp.Codec
			log.Println(fmt.Sprintf("video codec[%s]\n", s.VCodec))
		}

		addPusher := false
		if s.closeOld {
			r, _ := s.options.Server.TryAttachToPusher(s)
			if r < -1 {
				log.Println("reject pusher.")

				res.StatusCode = base.StatusNotAcceptable
			} else if r == 0 {
				addPusher = true
			} else {
				log.Println("Attached to old pusher")
			}
		} else {
			addPusher = true
		}

		if addPusher {
			s.Pusher = NewPusher(s)
			addedToServer := s.options.Server.AddPusher(s.Pusher)
			if !addedToServer {
				log.Println("reject pusher.")
				res.StatusCode = base.StatusNotAcceptable
			}
		}

	case base.Describe:
		s.Type = SESSION_TYPE_PLAYER
		s.URL = req.URL

		pusher := s.options.Server.GetPusher(req.URL.Path)
		if pusher == nil {
			res.StatusCode = base.StatusNotFound
			return
		}

		s.Player = NewPlayer(s, pusher)
		s.Pusher = pusher
		s.AControl = pusher.AControl()
		s.ACodec = pusher.ACodec()
		s.VControl = pusher.VControl()
		s.VCodec = pusher.VCodec()
		res.Body = []byte(s.Pusher.SDPRaw())
	case base.Setup:

		if _, ok := req.Header["Transport"]; !ok {
			res.StatusCode = base.StatusInternalServerError
			return
		}

		ts := req.Header["Transport"]
		// control字段可能是`stream=1`字样，也可能是rtsp://...字样。即control可能是url的path，也可能是整个url
		// 例1：
		// a=control:streamid=1
		// 例2：
		// a=control:rtsp://192.168.1.64/trackID=1
		// 例3：
		// a=control:?ctype=video
		if req.URL.Port() == "" {
			req.URL.Host = fmt.Sprintf("%s:554", req.URL.Host)
		}

		setupPath := req.URL.String()

		if s.Pusher == nil {
			res.StatusCode = base.StatusInternalServerError
			return
		}
		vPath := ""
		if strings.Index(strings.ToLower(s.VControl), "rtsp://") == 0 {
			vControlUrl, err := url.Parse(s.VControl)
			if err != nil {
				res.StatusCode = base.StatusInternalServerError
				return
			}

			if vControlUrl.Port() == "" {
				vControlUrl.Host = fmt.Sprintf("%s:554", vControlUrl.Host)
			}
			vPath = vControlUrl.String()
		} else {
			vPath = s.VControl
		}

		aPath := ""
		if strings.Index(strings.ToLower(s.AControl), "rtsp://") == 0 {
			aControlUrl, err := url.Parse(s.AControl)
			if err != nil {
				res.StatusCode = base.StatusInternalServerError
				return
			}
			if aControlUrl.Port() == "" {
				aControlUrl.Host = fmt.Sprintf("%s:554", aControlUrl.Host)
			}
			aPath = aControlUrl.String()
		} else {
			aPath = s.AControl
		}

		mtcp := regexp.MustCompile("interleaved=(\\d+)(-(\\d+))?")
		if tcpMatch := mtcp.FindStringSubmatch(ts[0]); tcpMatch != nil {
			s.TransType = TransTypeTcp
			if setupPath == aPath || aPath != "" && strings.LastIndex(setupPath, aPath) == len(setupPath)-len(aPath) {
				s.aRtpPort, _ = strconv.Atoi(tcpMatch[1])
				s.aRtcpPort, _ = strconv.Atoi(tcpMatch[3])
			} else if setupPath == vPath || vPath != "" && strings.LastIndex(setupPath, vPath) == len(setupPath)-len(vPath) {
				s.vRtpPort, _ = strconv.Atoi(tcpMatch[1])
				s.vRtcpPort, _ = strconv.Atoi(tcpMatch[3])
			} else {
				res.StatusCode = base.StatusInternalServerError
				res.StatusMessage = fmt.Sprintf("SETUP [TCP] got UnKown control:%s", setupPath)
				log.Println(fmt.Sprintf("SETUP [TCP] got UnKown control:%s ", setupPath))
			}
		}
		res.Header["Transport"] = ts
	case base.Play:
		// error status. PLAY without ANNOUNCE or DESCRIBE.
		if s.Pusher == nil {
			res.StatusCode = base.StatusInternalServerError
			return
		}
		res.Header["Range"] = req.Header["Range"]
	case base.Record:
		// error status. RECORD without ANNOUNCE or DESCRIBE.
		if s.Pusher == nil {
			res.StatusCode = base.StatusInternalServerError
			return
		}
	case base.Pause:
		if s.Player == nil {
			res.StatusCode = base.StatusInternalServerError
			return
		}
		s.Player.Pause(true)
	}
}

func (s *Session) CheckAuth(req *base.Request, res *base.Response) error {
	pass, _ := req.URL.User.Password()
	user := req.URL.User.Username()
	sender, err := headers.NewSender(req.Header["Authorization"], user, pass)
	if err != nil {
		return err
	}
	if err = sender.CheckAuth(req.URL); err != nil {
		log.Println(fmt.Errorf("%v", err))
		res.StatusCode = base.StatusUnauthorized
		nonce := fmt.Sprintf("%x", md5.Sum([]byte(shortid.MustGenerate())))
		authenticate := fmt.Sprintf(`Digest realm="EasyDarwin", nonce="%s", algorithm="MD5"`, nonce)
		res.Header["WWW-Authenticate"] = base.HeaderValue{authenticate}
		return err
	}
	return nil
}

// SendRTP 发送rtp包
func (s *Session) SendRTP(pack *RTPPack) error {
	if pack == nil {
		return fmt.Errorf("player send rtp got nil pack")
	}
	port := 0
	switch pack.Type {
	case RTP_TYPE_AUDIO:
		port = s.aRtpPort
	case RTP_TYPE_AUDIOCONTROL:
		port = s.aRtcpPort
	case RTP_TYPE_VIDEO:
		port = s.vRtpPort
	case RTP_TYPE_VIDEOCONTROL:
		port = s.vRtcpPort
	default:
		return fmt.Errorf("session tcp send rtp got unkown pack type[%v]", pack.Type)
	}

	bufChannel := make([]byte, 2)
	bufChannel[0] = 0x24
	bufChannel[1] = byte(port)
	s.connWLock.Lock()
	s.connRW.Write(bufChannel)
	bufLen := make([]byte, 2)
	binary.BigEndian.PutUint16(bufLen, uint16(pack.Buffer.Len()))
	s.connRW.Write(bufLen)
	s.connRW.Write(pack.Buffer.Bytes())
	s.connRW.Flush()
	s.connWLock.Unlock()
	return nil
}
