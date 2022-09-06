package rtsp

import (
	"log"
	"sync"
	"time"
)

// Player 播放器
type Player struct {
	*Session
	Pusher               *Pusher
	cond                 *sync.Cond
	queue                []*RTPPack
	queueLimit           int
	dropPacketWhenPaused bool
	paused               bool
}

// NewPlayer return Player
func NewPlayer(s *Session, pusher *Pusher) *Player {
	player := &Player{
		Session:              s,
		Pusher:               pusher,
		cond:                 sync.NewCond(&sync.Mutex{}),
		queueLimit:           0,
		dropPacketWhenPaused: false,
		paused:               false,
	}
	s.StopHandles = append(s.StopHandles, func() {
		pusher.RemovePlayer(player)
		player.cond.Broadcast()
	})
	return player
}

func (p *Player) QueueRTP(pack *RTPPack) *Player {
	if pack == nil {
		p.Println("player queue enter nil pack, drop it")
		return p
	}

	if p.paused && p.dropPacketWhenPaused {
		return p
	}

	p.cond.L.Lock()
	p.queue = append(p.queue, pack)
	if oldLen := len(p.queue); p.queueLimit > 0 && oldLen > p.queueLimit {
		p.queue = p.queue[1:]
	}
	p.cond.Signal()
	p.cond.L.Unlock()
	return p
}

// Start 启动
func (p *Player) Start() {
	timer := time.Unix(0, 0)

	for !p.Stopped {
		var pack *RTPPack
		p.cond.L.Lock()
		if len(p.queue) == 0 {
			p.cond.Wait()
		}
		if len(p.queue) > 0 {
			pack = p.queue[0]
			p.queue = p.queue[1:]
		}

		queueLen := len(p.queue)
		p.cond.L.Unlock()

		if p.paused {
			continue
		}

		if pack == nil {
			if !p.Stopped {
				p.Println("player not Stopped, but queue take out nil pack")
			}
			continue
		}
		if err := p.SendRTP(pack); err != nil {
			p.Println(err)
		}

		elapsed := time.Now().Sub(timer)
		if elapsed >= 30*time.Second {
			p.Println("Player %s, Send a package.type:%d, queue.len=%d\n", p.String(), pack.Type, queueLen)
			timer = time.Now()
		}
	}
}

// Pause 暂停
func (p *Player) Pause(b bool) {
	if b {
		p.Println("Player %s, Pause\n", p.String())
	} else {
		p.Println("Player %s, Play\n", p.String())
	}

	p.cond.L.Lock()
	if b && p.dropPacketWhenPaused && len(p.queue) > 0 {
		p.queue = make([]*RTPPack, 0)
	}

	p.paused = b
	p.cond.L.Unlock()
}

//Println mini logging functions
func (p *Player) Println(v ...interface{}) {
	log.Println(v)
}
