package rtsp

import (
	"bytes"
	"strings"
	"sync"
)

type Pusher struct {
	*Session
	*Client
	players        map[string]*Player
	playersLock    sync.RWMutex
	gopCacheEnable bool
	gopCache       []*RTPPack
	gopCacheLock   sync.RWMutex

	SPSPPSInSTAPaPack bool
	cond              *sync.Cond
	queue             []*RTPPack
}

func (p *Pusher) Server() *Server {
	if p.Session != nil {
		return p.Session.options.Server
	}
	return p.Client.Server
}

func (p *Pusher) Path() string {
	if p.Session != nil {
		return p.Session.URL.Path
	}
	if p.Client.options.CustomPath != "" {
		return p.Client.options.CustomPath
	}
	return p.Client.Path
}

func (p *Pusher) Stopped() bool {
	if p.Session != nil {
		return p.Session.Stopped
	}
	return p.Client.Stopped
}

func (p *Pusher) BroadcastRTP(pack *RTPPack) *Pusher {
	for _, player := range p.GetPlayers() {
		player.QueueRTP(pack)
	}
	return p
}

func (p *Pusher) GetPlayers() (players map[string]*Player) {
	players = make(map[string]*Player)
	p.playersLock.RLock()
	for k, v := range p.players {
		players[k] = v
	}
	p.playersLock.RUnlock()
	return
}

func (p *Pusher) VCodec() string {
	if p.Session != nil {
		return p.Session.VCodec
	}
	return p.Client.VCodec
}

func (p *Pusher) ACodec() string {
	if p.Session != nil {
		return p.Session.ACodec
	}
	return p.Client.ACodec
}

func (p *Pusher) AControl() string {
	if p.Session != nil {
		return p.Session.AControl
	}
	return p.Client.AControl
}

func (p *Pusher) VControl() string {
	if p.Session != nil {
		return p.Session.VControl
	}
	return p.Client.VControl
}

func (p *Pusher) SDPRaw() string {
	if p.Session != nil {
		return p.Session.SDPRaw
	}
	return p.Client.SDPRaw
}

func (p *Pusher) HasPlayer(player *Player) bool {
	p.playersLock.Lock()
	_, ok := p.players[player.ID]
	p.playersLock.Unlock()
	return ok
}

func (p *Pusher) AddPlayer(player *Player) *Pusher {
	if p.gopCacheEnable {
		p.gopCacheLock.RLock()
		for _, pack := range p.gopCache {
			player.QueueRTP(pack)
		}
		p.gopCacheLock.RUnlock()
	}

	p.playersLock.Lock()
	if _, ok := p.players[player.ID]; !ok {
		p.players[player.ID] = player
		go player.Start()
		p.Client.Println("%v start, now player size[%d]", player, len(p.players))
	}
	p.playersLock.Unlock()
	return p
}

// NewClientPusher 创建一个客户端推流
func NewClientPusher(client *Client) *Pusher {
	pusher := &Pusher{
		Client:         client,
		players:        make(map[string]*Player),
		gopCacheEnable: true,
		gopCache:       make([]*RTPPack, 0),

		cond:  sync.NewCond(&sync.Mutex{}),
		queue: make([]*RTPPack, 0),
	}
	client.RTPHandles = append(client.RTPHandles, func(pack *RTPPack) {
		pusher.QueueRTP(pack)
	})
	client.StopHandles = append(client.StopHandles, func() {
		pusher.ClearPlayer()
		pusher.Server().RemovePusher(pusher)
		pusher.cond.Broadcast()
	})
	return pusher
}

// NewPusher 创建推流
func NewPusher(s *Session) *Pusher {
	pusher := &Pusher{
		Session:        s,
		Client:         nil,
		players:        make(map[string]*Player),
		gopCacheEnable: true,
		gopCache:       make([]*RTPPack, 0),

		cond:  sync.NewCond(&sync.Mutex{}),
		queue: make([]*RTPPack, 0),
	}
	pusher.bindSession(s)
	return pusher
}

// QueueRTP  rtp包加入缓存
func (p *Pusher) QueueRTP(pack *RTPPack) *Pusher {
	p.cond.L.Lock()
	p.queue = append(p.queue, pack)
	p.cond.Signal()
	p.cond.L.Unlock()
	return p
}

// ClearPlayer 清理播放器
func (p *Pusher) ClearPlayer() {
	p.playersLock.Lock()
	players := p.players
	p.players = make(map[string]*Player)
	p.playersLock.Unlock()
	go func() {
		for _, v := range players {
			v.Stop()
		}
	}()
}

// RemovePlayer 删除播放器
func (p *Pusher) RemovePlayer(player *Player) *Pusher {
	p.playersLock.Lock()
	if len(p.players) == 0 {
		p.playersLock.Unlock()
		return p
	}

	delete(p.players, player.ID)
	p.Client.Println("%v end, now player size[%d]\n", player, len(p.players))
	p.playersLock.Unlock()
	return p
}

func (p *Pusher) bindSession(s *Session) {
	p.Session = s
	s.RTPHandles = append(s.RTPHandles, func(pack *RTPPack) {
		if s != p.Session {
			p.Client.Println("Session recv rtp to pusher.but pusher got a new session[%v].", p.Session.ID)
			return
		}
		p.QueueRTP(pack)
	})
	s.StopHandles = append(s.StopHandles, func() {
		if s != p.Session {
			p.Client.Println("Session stop to release pusher.but pusher got a new session[%v].", p.Session.ID)
			return
		}
		p.ClearPlayer()
		p.Server().RemovePusher(p)
		p.cond.Broadcast()
	})
}

func (p *Pusher) RebindSession(session *Session) bool {
	if p.Client != nil {
		p.Client.Println("call RebindSession[%s] to a Client-Pusher. got false", session.ID)
		return false
	}

	p.bindSession(session)
	session.Pusher = p

	p.gopCacheLock.Lock()
	p.gopCache = make([]*RTPPack, 0)
	p.gopCacheLock.Unlock()

	if p.Session != nil {
		p.Session.Stop()
	}
	return true
}

// Start 启动推流
func (p *Pusher) Start() {
	for !p.Stopped() {
		var pack *RTPPack
		p.cond.L.Lock()

		if len(p.queue) == 0 {
			p.cond.Wait()
		}

		if len(p.queue) > 0 {
			pack = p.queue[0]
			p.queue = p.queue[1:]
		}
		p.cond.L.Unlock()

		if pack == nil {
			if !p.Stopped() {
				p.Client.Println("pusher not stopped, but queue take out nil pack")
			}
			continue
		}

		if p.gopCacheEnable && pack.Type == RTP_TYPE_VIDEO {
			p.gopCacheLock.Lock()
			packBuffer := pack.Buffer.Bytes()
			if rtp := ParseRTP(packBuffer); rtp != nil && p.isKeyframe(rtp) {
				p.gopCache = make([]*RTPPack, 0)
				payload := make([]byte, 0)
				if p.Client.options.IsEncrypt {
					payload = p.Client.EncryptPack(rtp.Payload[2:], uint16(rtp.SequenceNumber))
				}
				if p.Client.options.IsDecode {
					payload = p.Client.DecodePack(rtp.Payload[2:])
				}
				rtp.Payload = append(rtp.Payload[:2], payload...)
				pack.Buffer = bytes.NewBuffer(append(packBuffer[:rtp.PayloadOffset], rtp.Payload...))
			}
			p.gopCache = append(p.gopCache, pack)
			p.gopCacheLock.Unlock()
		}
		p.BroadcastRTP(pack)
	}
}

func (p *Pusher) isKeyframe(rtp *RTPInfo) bool {
	if strings.EqualFold(p.VCodec(), "h264") {
		var realNALU uint8
		payloadHeader := rtp.Payload[0]
		t := payloadHeader & 0x1F
		switch {
		case t <= 23:
			realNALU = payloadHeader
		case t == 28 || t == 29:
			realNALU = rtp.Payload[1]
			if realNALU&0x80 == 0 {
				return false
			}
		}
		//判断是否位I帧的算法位：
		//NALU类型 & 0001 1111 //5为I帧，7是sps 8是pps
		//TYPE:表示该NALU的类型是什么，见下表，由此可知7位序列参数集（SPS）,8为图像参数集（PPS）,5代表I帧，1代表非I帧。
		if realNALU&0x1F == 0x05 {
			return true
		}
		return false
	}
	return false
}
