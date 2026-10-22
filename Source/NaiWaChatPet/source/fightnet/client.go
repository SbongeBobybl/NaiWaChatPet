package fightnet

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"codexpet/reconstruction/play"
)

type Client struct {
	mu                sync.Mutex
	conn              net.Conn
	write             sync.Mutex
	updates           chan Update
	inputQueue        chan Message
	done              chan struct{}
	once              sync.Once
	last              Update
	seq, lastTick     uint64
	match             uint64
	lastEvent         uint64
	previous, current *play.CombatSnapshot
	received          time.Time
	Err               error
}

func Dial(ctx context.Context, address string) (*Client, error) {
	conn, err := dialConnection(ctx, address)
	if err != nil {
		return nil, err
	}
	c := &Client{conn: conn, updates: make(chan Update, 8), done: make(chan struct{}), inputQueue: make(chan Message, 8)}
	go c.read()
	go c.writeInputs()
	go c.heartbeat()
	return c, nil
}

func dialConnection(ctx context.Context, address string) (net.Conn, error) {
	address, err := NormalizeAddress(address)
	if err != nil {
		return nil, err
	}
	var conn net.Conn
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	if strings.HasPrefix(address, "tls://") {
		target := strings.TrimPrefix(address, "tls://")
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: &tls.Config{MinVersion: tls.VersionTLS12}}).DialContext(ctx, "tcp", target)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", strings.TrimPrefix(address, "tcp://"))
	}
	return conn, err
}
func (c *Client) Send(m Message) error {
	m.Version = ProtocolVersion
	c.write.Lock()
	defer c.write.Unlock()
	c.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	return json.NewEncoder(c.conn).Encode(m)
}
func (c *Client) SendInput(in play.Input) error {
	c.mu.Lock()
	c.seq++
	seq := c.seq
	c.mu.Unlock()
	m := Message{Type: "input", Seq: seq, Input: in}
	select {
	case <-c.done:
		return net.ErrClosed
	default:
	}
	select {
	case c.inputQueue <- m:
	default:
		select {
		case old := <-c.inputQueue:
			m.Input.MergeEdges(old.Input)
		default:
		}
		select {
		case c.inputQueue <- m:
		default:
		}
	}
	return nil
}

func (c *Client) writeInputs() {
	for {
		select {
		case <-c.done:
			return
		case msg := <-c.inputQueue:
			// Send the newest direction after a write stall, preserving one-shot
			// actions without replaying a burst of obsolete movement packets.
			for pending := len(c.inputQueue); pending > 0; pending-- {
				select {
				case next := <-c.inputQueue:
					next.Input.MergeEdges(msg.Input)
					msg = next
				default:
					goto writeLatest
				}
			}
		writeLatest:
			if err := c.Send(msg); err != nil {
				c.Close()
				return
			}
		}
	}
}
func (c *Client) read() {
	defer c.Close()
	scanner := bufio.NewScanner(c.conn)
	scanner.Buffer(make([]byte, 4096), 128<<10)
	for {
		c.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		if !scanner.Scan() {
			c.mu.Lock()
			if scanner.Err() != nil {
				c.Err = scanner.Err()
			} else {
				c.Err = errors.New("connection closed")
			}
			c.mu.Unlock()
			return
		}
		var u Update
		if json.Unmarshal(scanner.Bytes(), &u) != nil || u.Version != ProtocolVersion {
			continue
		}
		if u.Type == "pong" {
			continue
		}
		c.mu.Lock()
		if u.Type == "welcome" {
			c.last.Room, c.last.Token, c.last.Player = u.Room, u.Token, u.Player
		}
		if u.Snapshot != nil {
			if u.Match != 0 && u.Match < c.match {
				c.mu.Unlock()
				continue
			}
			if u.Match > c.match || u.Match == 0 && u.State == "countdown" && u.Snapshot.Tick == 0 && c.last.State != "countdown" {
				c.match = u.Match
				c.lastTick = 0
				c.current = nil
				c.previous = nil
				c.lastEvent = 0
			}
			if u.Snapshot.Tick < c.lastTick {
				c.mu.Unlock()
				continue
			}
			if u.Snapshot.Tick > c.lastTick || c.current == nil {
				c.previous = c.current
				c.current = u.Snapshot
				c.received = time.Now()
			}
			c.lastTick = u.Snapshot.Tick
			if u.Player >= 0 && u.Player < 2 {
				c.seq = max(c.seq, u.Snapshot.Ack[u.Player])
			}
		}
		if u.Token == "" {
			u.Token = c.last.Token
		}
		if u.Room == "" {
			u.Room = c.last.Room
		}
		c.last = u
		c.mu.Unlock()
		select {
		case c.updates <- u:
		default:
			select {
			case <-c.updates:
			default:
			}
			select {
			case c.updates <- u:
			default:
			}
		}
	}
}
func (c *Client) heartbeat() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			if c.Send(Message{Type: "ping"}) != nil {
				c.Close()
				return
			}
		}
	}
}
func (c *Client) Updates() <-chan Update { return c.updates }
func (c *Client) Done() <-chan struct{}  { return c.done }
func (c *Client) Close() error {
	var err error
	c.once.Do(func() { close(c.done); err = c.conn.Close() })
	return err
}
func (c *Client) State() Update { c.mu.Lock(); defer c.mu.Unlock(); return c.last }
func (c *Client) Error() error  { c.mu.Lock(); defer c.mu.Unlock(); return c.Err }

// DisplaySnapshot interpolates remote positions over one snapshot interval.
// A bounded local positional prediction is cosmetic; HP, MP, skills, timers
// and all hit outcomes always come from the server snapshot.
func (c *Client) DisplaySnapshot(now time.Time, local play.Input) *play.CombatSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current == nil {
		return nil
	}
	s := *c.current
	s.Projectiles = append([]play.Projectile(nil), c.current.Projectiles...)
	elapsed := now.Sub(c.received).Seconds()
	for i := range s.Fighters {
		f := &s.Fighters[i]
		advance := min(3, max(0, int(elapsed*play.CombatHz)))
		if f.Action.Skill != play.NoSkill {
			duration := 65
			if s.Rules != nil && len(s.Rules.Skills) >= int(f.Action.Skill) {
				duration = s.Rules.Skills[int(f.Action.Skill)-1].Duration
			}
			f.Action.Age = min(max(0, duration-1), f.Action.Age+advance)
		}
		if f.View.Stance && f.View.Strike > 0 {
			duration := .34
			if f.View.Power > 1 {
				duration = .5
			}
			f.View.Strike = min(1, f.View.Strike+min(.05, max(0, elapsed))/duration)
		}
	}
	alpha := min(1, max(0, elapsed/.05))
	if c.previous != nil && c.previous.Round == s.Round {
		for i := range s.Fighters {
			if i == c.last.Player {
				continue
			}
			p := c.previous.Fighters[i]
			f := &s.Fighters[i]
			if mathDistance(p.X, p.Y, f.X, f.Y) < 400*400 {
				f.X = p.X + (f.X-p.X)*alpha
				f.Y = p.Y + (f.Y-p.Y)*alpha
			}
		}
	}
	if c.last.State == "fighting" && c.last.Player >= 0 && c.last.Player < 2 {
		f := &s.Fighters[c.last.Player]
		if f.Action.Skill == play.NoSkill && s.Tick >= f.StunUntil && s.Tick >= f.HitUntil {
			f.X, f.Y = play.PredictCombatPosition(*f, local, elapsed)
		}
	}
	return &s
}
func mathDistance(x, y, a, b float64) float64 { dx, dy := x-a, y-b; return dx*dx + dy*dy }
