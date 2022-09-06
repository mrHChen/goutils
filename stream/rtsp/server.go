package rtsp

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// Server rtsp服务端
type Server struct {
	TCPListener    *net.TCPListener
	TCPPort        int
	Stopped        bool
	pushers        map[string]*Pusher
	pushersLock    sync.RWMutex
	addPusherCh    chan *Pusher
	removePusherCh chan *Pusher
}

// NewRTSPServer 创建 rtsp 服务端实例
func NewRTSPServer(port int) *Server {
	server := &Server{
		Stopped:        true,
		TCPPort:        port,
		pushers:        make(map[string]*Pusher),
		addPusherCh:    make(chan *Pusher),
		removePusherCh: make(chan *Pusher),
	}
	return server
}

// Start 启动推流
func (s *Server) Start() error {
	var (
		err      error
		addr     *net.TCPAddr
		listener *net.TCPListener
	)
	// 定义一个TCP地址
	addr, err = net.ResolveTCPAddr("tcp", fmt.Sprintf(":%d", s.TCPPort))
	if err != nil {
		return err
	}
	// 开始监听地址
	if listener, err = net.ListenTCP("tcp", addr); err != nil {
		log.Println(err.Error())
		return err
	}

	s.Stopped = false
	s.TCPListener = listener
	// 启动rtsp服务器
	networkBuffer := 1048576
	for !s.Stopped {
		var (
			conn net.Conn
		)
		if conn, err = s.TCPListener.Accept(); err != nil {
			log.Println("建立tcp 通道")
			continue
		}

		if tcpConn, ok := conn.(*net.TCPConn); ok {
			if err = tcpConn.SetReadBuffer(networkBuffer); err != nil {
				log.Println(fmt.Errorf("rtsp server conn set read buffer error:%s ", err))
			}
			if tcpConn.SetWriteBuffer(networkBuffer); err != nil {
				log.Println(fmt.Errorf("rtsp server conn set write buffer error:%v", err))
			}
		}

		session := NewSession(
			SessionOptions{
				Server:        s,
				conn:          conn,
				CloseOld:      true,
				Authorization: false,
				Timeout:       time.Duration(10) * time.Second,
			})
		go session.Start()
	}
	return err
}

// RemovePusher 删除推流
func (s *Server) RemovePusher(pusher *Pusher) {
	s.pushersLock.Lock()
	removed := false
	if _pusher, ok := s.pushers[pusher.Path()]; ok && pusher.ID == _pusher.ID {
		delete(s.pushers, pusher.Path())
		log.Println(fmt.Sprintf("%v end, now pusher size[%d]\n", pusher, len(s.pushers)))
		removed = true
	}
	s.pushersLock.Unlock()
	if removed {
		s.removePusherCh <- pusher
	}
}

// GetPusher 获取推流
func (s *Server) GetPusher(path string) (pusher *Pusher) {
	s.pushersLock.RLock()
	pusher = s.pushers[path]
	s.pushersLock.RUnlock()
	return
}

func (s *Server) TryAttachToPusher(session *Session) (int, *Pusher) {
	s.pushersLock.Lock()
	attached := 0
	var pusher *Pusher = nil
	if _pusher, ok := s.pushers[session.URL.Path]; ok {
		if _pusher.RebindSession(session) {
			log.Println(fmt.Sprintf("Attached to a pusher"))
			attached = 1
			pusher = _pusher
		} else {
			attached = -1
		}
	}
	s.pushersLock.Unlock()
	return attached, pusher
}

// AddPusher 新增推流
func (s *Server) AddPusher(pusher *Pusher) bool {
	s.pushersLock.Lock()
	if _, ok := s.pushers[pusher.Path()]; ok {
		return false
	}
	s.pushers[pusher.Path()] = pusher
	log.Println(fmt.Sprintf("start, now pusher size[%d]", len(s.pushers)))
	s.pushersLock.Unlock()

	go pusher.Start()
	s.addPusherCh <- pusher
	return true
}
