package fightnet

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"codexpet/reconstruction/play"
)

type ServerConfig struct {
	Rules                    *play.CombatRules
	MaxRooms, MaxConnections int
	ReconnectGrace           time.Duration
	ManualTicks              bool
}
type Server struct {
	mu       sync.Mutex
	config   ServerConfig
	rooms    map[string]*room
	peers    map[*peer]bool
	listener net.Listener
	closed   bool
	wg       sync.WaitGroup
}
type room struct {
	resumeState string
	code        string
	slots       [2]*slot
	combat      *play.Combat
	countdown   int
	state       string
	touched     time.Time
	ticks       uint64
	match       uint64
}
type slot struct {
	token        string
	peer         *peer
	ready        bool
	input        play.Input
	seq          uint64
	disconnected time.Time
	inputAt      time.Time
}
type peer struct {
	conn   net.Conn
	out    chan Update
	room   *room
	player int
	done   chan struct{}
	once   sync.Once
	tokens float64
	refill time.Time
}

func NewServer(config ServerConfig) *Server {
	if config.MaxRooms <= 0 {
		config.MaxRooms = 128
	}
	if config.MaxConnections <= 0 {
		config.MaxConnections = 256
	}
	if config.ReconnectGrace <= 0 {
		config.ReconnectGrace = 30 * time.Second
	}
	return &Server{config: config, rooms: map[string]*room{}, peers: map[*peer]bool{}}
}
func randomID(bytes int) string {
	b := make([]byte, bytes)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return strings.ToUpper(hex.EncodeToString(b))
}
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return net.ErrClosed
	}
	s.listener = listener
	s.mu.Unlock()
	go func() { <-ctx.Done(); s.Close() }()
	if !s.config.ManualTicks {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			ticker := time.NewTicker(time.Second / play.CombatHz)
			defer ticker.Stop()
			last := time.Now()
			accumulator := 0.0
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					now := time.Now()
					accumulator += min(.25, now.Sub(last).Seconds())
					last = now
					for accumulator+1e-12 >= play.CombatDT {
						s.Tick(now)
						accumulator -= play.CombatDT
					}
					s.mu.Lock()
					closed := s.closed
					s.mu.Unlock()
					if closed {
						return
					}
				}
			}
		}()
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		s.mu.Lock()
		if s.closed || len(s.peers) >= s.config.MaxConnections {
			s.mu.Unlock()
			conn.Close()
			continue
		}
		host, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
		sameIP := 0
		for p := range s.peers {
			h, _, _ := net.SplitHostPort(p.conn.RemoteAddr().String())
			if h == host {
				sameIP++
			}
		}
		if sameIP >= 8 {
			s.mu.Unlock()
			conn.Close()
			continue
		}
		p := &peer{conn: conn, out: make(chan Update, 4), done: make(chan struct{}), player: -1, tokens: 60, refill: time.Now()}
		s.peers[p] = true
		s.wg.Add(1)
		s.mu.Unlock()
		go s.runPeer(p)
	}
}
func (s *Server) runPeer(p *peer) {
	defer s.wg.Done()
	defer s.disconnect(p)
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		encoder := json.NewEncoder(p.conn)
		for {
			select {
			case <-p.done:
				return
			case u := <-p.out:
				p.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
				if encoder.Encode(u) != nil {
					p.conn.Close()
					return
				}
			}
		}
	}()
	defer func() { p.stop(); <-writerDone }()
	scanner := bufio.NewScanner(p.conn)
	scanner.Buffer(make([]byte, 1024), MaxMessageBytes)
	for {
		p.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		if !scanner.Scan() {
			return
		}
		now := time.Now()
		p.tokens = min(60, p.tokens+now.Sub(p.refill).Seconds()*120)
		p.refill = now
		if p.tokens < 1 {
			s.mu.Lock()
			s.send(p, Update{Type: "error", Error: "rate_limit"})
			s.mu.Unlock()
			return
		}
		p.tokens--
		var msg Message
		decoder := json.NewDecoder(strings.NewReader(scanner.Text()))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&msg); err != nil {
			s.mu.Lock()
			s.send(p, Update{Type: "error", Error: "invalid_message"})
			s.mu.Unlock()
			continue
		}
		if msg.Version != ProtocolVersion {
			s.mu.Lock()
			s.send(p, Update{Type: "error", Error: "version_mismatch"})
			s.mu.Unlock()
			continue
		}
		s.handle(p, msg, now)
	}
}
func (p *peer) stop() { p.once.Do(func() { close(p.done); p.conn.Close() }) }
func (s *Server) send(p *peer, u Update) {
	u.Version = ProtocolVersion
	u.Player = p.player
	select {
	case <-p.done:
		return
	default:
	}
	select {
	case p.out <- u:
	default: // Drop an obsolete snapshot, never block the simulation.
		select {
		case <-p.out:
		default:
		}
		select {
		case p.out <- u:
		default:
		}
	}
}
func (s *Server) handle(p *peer, m Message, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fail := func(e string) { s.send(p, Update{Type: "error", Error: e}) }
	switch m.Type {
	case "ping":
		s.send(p, Update{Type: "pong"})
		return
	case "create":
		if p.room != nil {
			fail("already_in_room")
			return
		}
		if len(s.rooms) >= s.config.MaxRooms {
			fail("room_limit")
			return
		}
		r := &room{code: randomID(3), state: "waiting", touched: now}
		for s.rooms[r.code] != nil {
			r.code = randomID(3)
		}
		s.rooms[r.code] = r
		s.attach(p, r, 0, now)
	case "join":
		if p.room != nil {
			fail("already_in_room")
			return
		}
		code := strings.ToUpper(strings.TrimSpace(m.Room))
		r := s.rooms[code]
		if r == nil || len(code) != 6 {
			fail("room_not_found")
			return
		}
		if r.slots[1] != nil {
			fail("room_full")
			return
		}
		s.attach(p, r, 1, now)
	case "resume":
		if p.room != nil || len(m.Token) != 48 || len(m.Room) != 6 {
			fail("invalid_resume")
			return
		}
		r := s.rooms[m.Room]
		if r == nil {
			fail("room_not_found")
			return
		}
		found := -1
		for i, v := range r.slots {
			if v != nil && v.token == m.Token && v.peer == nil && now.Sub(v.disconnected) < s.config.ReconnectGrace {
				found = i
			}
		}
		if found < 0 {
			fail("invalid_resume")
			return
		}
		p.room, p.player = r, found
		r.slots[found].peer = p
		r.slots[found].disconnected = time.Time{}
		r.slots[found].input = play.Input{}
		s.send(p, Update{Type: "welcome", Room: r.code, Token: r.slots[found].token})
		if s.bothConnected(r) && r.state == "disconnected" {
			r.state = r.resumeState
			if r.state == "" {
				r.state = "fighting"
			}
		}
		s.broadcast(r)
	case "leave":
		s.leave(p, false, now)
	case "ready", "rematch":
		r := p.room
		if r == nil {
			fail("not_in_room")
			return
		}
		if m.Type == "ready" && r.state != "waiting" || m.Type == "rematch" && r.state != "over" {
			fail("invalid_room_state")
			return
		}
		r.slots[p.player].ready = true
		if s.bothReady(r) {
			var err error
			r.combat, err = play.NewConfiguredCombat(false, s.config.Rules)
			if err != nil {
				fail("invalid_server_rules")
				return
			}
			r.countdown = 180
			r.match++
			r.state = "countdown"
			for _, v := range r.slots {
				v.input = play.Input{}
				v.ready = false
			}
		}
		s.broadcast(r)
	case "input":
		r := p.room
		if r == nil || r.state != "fighting" {
			return
		}
		v := r.slots[p.player]
		if m.Seq <= v.seq || m.Seq-v.seq > 4096 {
			return
		}
		v.seq = m.Seq
		in := m.Input
		in.MergeEdges(v.input)
		v.input = in
		v.inputAt = now
	default:
		fail("unknown_message")
	}
}
func (s *Server) attach(p *peer, r *room, i int, now time.Time) {
	p.room, p.player = r, i
	r.slots[i] = &slot{token: randomID(24), peer: p, inputAt: now}
	s.send(p, Update{Type: "welcome", Room: r.code, Token: r.slots[i].token})
	s.broadcast(r)
}
func (s *Server) bothConnected(r *room) bool {
	return r.slots[0] != nil && r.slots[1] != nil && r.slots[0].peer != nil && r.slots[1].peer != nil
}
func (s *Server) bothReady(r *room) bool {
	return s.bothConnected(r) && r.slots[0].ready && r.slots[1].ready
}
func (s *Server) broadcast(r *room) {
	u := Update{Type: "state", Room: r.code, Match: r.match, State: r.state, Countdown: r.countdown}
	for i, v := range r.slots {
		if v != nil {
			u.Ready[i] = v.ready
			u.Connected[i] = v.peer != nil
		}
	}
	if r.combat != nil {
		snap := r.combat.Snapshot()
		for i, v := range r.slots {
			if v != nil {
				snap.Ack[i] = v.seq
			}
		}
		u.Snapshot = &snap
	}
	for _, v := range r.slots {
		if v != nil && v.peer != nil {
			personal := u
			personal.Token = v.token
			s.send(v.peer, personal)
		}
	}
}
func (s *Server) leave(p *peer, disconnected bool, now time.Time) {
	r := p.room
	if r == nil {
		return
	}
	v := r.slots[p.player]
	if disconnected {
		v.peer = nil
		v.input = play.Input{}
		v.disconnected = now
		if r.state == "fighting" || r.state == "countdown" {
			r.resumeState = r.state
			r.state = "disconnected"
		}
	} else {
		for _, v := range r.slots {
			if v != nil && v.peer != nil {
				s.send(v.peer, Update{Type: "left", Room: r.code, State: "closed"})
				v.peer.room = nil
				v.peer.player = -1
			}
		}
		delete(s.rooms, r.code)
	}
	p.room = nil
	if !disconnected {
		return
	}
	s.broadcast(r)
}
func (s *Server) disconnect(p *peer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.leave(p, true, time.Now())
	delete(s.peers, p)
	p.stop()
}

// Tick advances every active room by one fixed tick. Exposed for deterministic
// network integration tests; production Serve calls it at 60 Hz.
func (s *Server) Tick(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for code, r := range s.rooms {
		r.ticks++
		previousState := r.state
		expired := false
		for _, v := range r.slots {
			if v != nil && v.peer == nil && !v.disconnected.IsZero() && now.Sub(v.disconnected) > s.config.ReconnectGrace {
				expired = true
			}
		}
		if r.state == "waiting" && now.Sub(r.touched) > 10*time.Minute {
			expired = true
		}
		if expired {
			for _, v := range r.slots {
				if v != nil && v.peer != nil {
					s.send(v.peer, Update{Type: "left", State: "closed", Error: "reconnect_timeout"})
					v.peer.room = nil
					v.peer.player = -1
				}
			}
			delete(s.rooms, code)
			continue
		}
		if r.state == "countdown" {
			r.countdown--
			if r.countdown <= 0 {
				r.state = "fighting"
				r.combat.Arena.Phase = play.Fighting
			}
		}
		if r.state == "fighting" && s.bothConnected(r) {
			var in [2]play.Input
			for i, v := range r.slots {
				if now.Sub(v.inputAt) <= 250*time.Millisecond {
					in[i] = v.input
				}
				v.input.ClearEdges()
			}
			r.combat.Step(in)
			if r.combat.Arena.Phase == play.Over {
				r.state = "over"
			}
		}
		if r.ticks%3 == 0 || previousState != r.state {
			s.broadcast(r)
		}
	}
}
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	listener := s.listener
	for p := range s.peers {
		s.send(p, Update{Type: "left", State: "closed", Error: "server_shutdown"})
		p.stop()
	}
	s.mu.Unlock()
	if listener != nil {
		return listener.Close()
	}
	return nil
}
func (s *Server) Wait()          { s.wg.Wait() }
func (s *Server) RoomCount() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.rooms) }
func (s *Server) String() string { return fmt.Sprintf("NaiWa fight protocol %d", ProtocolVersion) }
