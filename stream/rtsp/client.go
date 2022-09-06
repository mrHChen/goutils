package rtsp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mrHChen/goutils/stream/common"
	"github.com/mrHChen/goutils/stream/utils"
)

// Client rtsp 客户端 C
type Client struct {
	Server  *Server
	options ClientOptions
	// 初始化rtsp地址
	RunOnInit string

	URL     *url.URL
	Path    string
	Stopped bool

	VCodec   string
	VControl string
	ACodec   string
	AControl string

	aRtpPort  int
	aRtcpPort int
	vRtpPort  int
	vRtcpPort int

	SDPRaw string

	Conn      *ClientConn
	TransType TransType

	RTPHandles  []func(*RTPPack)
	StopHandles []func()

	EncryptPack func([]byte, uint16) []byte
	DecodePack  func([]byte) []byte
}

type ClientOptions struct {
	// 是否打印debug日志
	Debug bool
	// rtsp地址
	rtspAddress string
	// 推流自定义地址
	CustomPath string
	// 代理商
	Agent string
	// 超时
	timeout time.Duration
	// 是否加密
	isEncrypt bool
	// 是否解密
	isDecode bool
}

// NewRTSPClient 创建 rtsp 客户端实例
// return Client
func NewRTSPClient(options ClientOptions) *Client {
	if options.Agent == "" {
		options.Agent = common.GetAgent()
	}
	client := &Client{
		options:   options,
		TransType: TransTypeTcp,
		Stopped:   false,

		vRtpPort:  0,
		vRtcpPort: 1,
		aRtpPort:  2,
		aRtcpPort: 3,
	}

	return client
}

// Start 启动客户端拉流
func (c *Client) Start() error {
	defer func() {
		if err := recover(); err != nil {
			c.Println(fmt.Errorf("start err : %v", err))
			return
		}
	}()

	err := c.Run()

	if err != nil {
		c.Println(fmt.Errorf("err :%s", err))
		time.Sleep(10 * time.Second)
		c.Start()
	}

	go c.startStream()
	return nil
}

// RunInit 运行初始化一个流
func (c *Client) RunInit() error {
	if c.RunOnInit == "" {
		return nil
	}
	out, err := utils.ExecBashCmd(c.RunOnInit)
	if err != nil {
		c.Println(fmt.Errorf("exec bash command error:%v,output:%s", err, out))
		return err
	}
	return nil
}

// Run 运行客户端
func (c *Client) Run() error {
	err := c.RunInit()
	if err != nil {
		return err
	}

	url, err := url.Parse(c.options.rtspAddress)
	if err != nil {
		c.Println("Address resolution error: %s", err)
		return err
	}
	c.URL = url
	c.Path = url.Path
	// 循环建立连接
	conn, err := NewClientConn(c, url.Scheme, url.Host)
	if err != nil {
		c.Println(fmt.Errorf("Create a client channel error: %s ", err))
		return err
	}

	_, err = conn.Options(url, false)
	if err != nil {
		conn.Close()
		c.Println(fmt.Errorf("Options error: %s ", err))
		return err
	}

	sdp, _, err := conn.Describe(url)
	if err != nil {
		c.Println(fmt.Errorf("Describe error: %s ", err))
		conn.Close()
		return err
	}

	for _, media := range sdp.Media {
		_, err = conn.Setup(url, media)
		if err != nil {
			c.Println(fmt.Errorf("Setup error: %s ", err))
			conn.Close()
			return err
		}
	}

	_, err = conn.Play(url)
	if err != nil {
		c.Println(fmt.Errorf("Play error: %s ", err))
		conn.Close()
		return err
	}
	c.Conn = conn
	return nil
}

//Println mini logging functions
func (c *Client) Println(v ...interface{}) {
	if c.options.Debug {
		log.Println(v)
	}
}

func (c *Client) startStream() {
	startTime := time.Now()
	conn := c.Conn
	defer conn.doClose()
	for !c.Stopped {
		if time.Since(startTime) > time.Duration(30)*time.Second {
			startTime = time.Now()
			// 心跳保活
			if _, err := conn.Options(c.URL, true); err != nil {
				// ignore...
			}
		}
		b, err := conn.connRW.ReadByte()
		if err != nil {
			if !c.Stopped {
				c.Println(fmt.Errorf("client.connRW.ReadByte err:%v", err))
			}
			return
		}
		switch b {
		case 0x24: //rtp
			header := make([]byte, 4)
			header[0] = b
			_, err = io.ReadFull(conn.connRW, header[1:])
			if err != nil {
				if !c.Stopped {
					c.Println(fmt.Errorf("client.connRW.ReadByte err:%v", err))
				}
				return
			}
			channel := int(header[1])
			length := binary.BigEndian.Uint16(header[2:])
			content := make([]byte, length)

			_, err = io.ReadFull(conn.connRW, content)
			if err != nil {
				if !c.Stopped {
					c.Println(fmt.Errorf("io.ReadFull err:%v", err))
				}
				return
			}

			rtpBuf := bytes.NewBuffer(content)
			var pack *RTPPack
			switch channel {
			case c.vRtpPort:
				pack = &RTPPack{
					Type:   RTP_TYPE_VIDEO,
					Buffer: rtpBuf,
				}
			case c.vRtcpPort:
				pack = &RTPPack{
					Type:   RTP_TYPE_VIDEOCONTROL,
					Buffer: rtpBuf,
				}
			case c.aRtpPort:
				pack = &RTPPack{
					Type:   RTP_TYPE_AUDIO,
					Buffer: rtpBuf,
				}
			case c.aRtcpPort:
				pack = &RTPPack{
					Type:   RTP_TYPE_AUDIOCONTROL,
					Buffer: rtpBuf,
				}
			default:
				c.Println(fmt.Errorf("unknow rtp pack type, channel:%v", channel))
				continue
			}

			for _, h := range c.RTPHandles {
				h(pack)
			}
		default: // rtsp
			builder := bytes.Buffer{}
			builder.WriteByte(b)
			contentLen := 0
			for !c.Stopped {
				line, prefix, err := conn.connRW.ReadLine()
				if err != nil {
					if !c.Stopped {
						c.Println(fmt.Errorf("client.connRW.ReadLine err:%v", err))
					}
					return
				}
				if len(line) == 0 {
					if contentLen != 0 {
						content := make([]byte, contentLen)
						_, err = io.ReadFull(conn.connRW, content)
						if err != nil {
							if !c.Stopped {
								c.Println(fmt.Errorf("Read content err.ContentLength:%d ", err))
							}
							return
						}
						builder.Write(content)
					}
					c.Println(fmt.Errorf("[s->c] %v", builder.String()))
					break
				}

				s := string(line)
				builder.Write(line)
				if !prefix {
					builder.WriteString("\r\n")
				}

				if strings.Index(s, "Content-Length:") == 0 {
					splits := strings.Split(s, ":")
					contentLen, err = strconv.Atoi(strings.TrimSpace(splits[1]))
					if err != nil {
						if !c.Stopped {
							c.Println(fmt.Errorf("strconv.Atoi err:%v, str:%v", err, splits[1]))
						}
						return
					}
				}
			}
		}
	}
}
