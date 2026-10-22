package fightnet

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"testing"
	"time"

	"codexpet/reconstruction/play"
)

func testServer(t *testing.T) (*Server, string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := NewServer(ServerConfig{ManualTicks: true})
	done := make(chan struct{})
	go func() { defer close(done); _ = s.Serve(ctx, listener) }()
	t.Cleanup(func() { cancel(); s.Close(); <-done; s.Wait() })
	return s, listener.Addr().String()
}
func testClient(t *testing.T, address string) *Client {
	t.Helper()
	c, e := Dial(context.Background(), address)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestConnectionProbeDoesNotCreateRooms(t *testing.T) {
	s, address := testServer(t)
	result, err := Probe(context.Background(), address)
	if err != nil || result.Version != ProtocolVersion {
		t.Fatalf("probe: %+v %v", result, err)
	}
	s.mu.Lock()
	rooms := len(s.rooms)
	s.mu.Unlock()
	if rooms != 0 {
		t.Fatal("connection check created a room")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Probe(ctx, address); err == nil {
		t.Fatal("canceled probe succeeded")
	}
}
func waitUpdate(t *testing.T, c *Client, predicate func(Update) bool) Update {
	t.Helper()
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case u := <-c.Updates():
			if predicate(u) {
				return u
			}
		case <-timeout.C:
			t.Fatalf("update timed out; last=%+v err=%v", c.State(), c.Error())
			return Update{}
		}
	}
}
func createPair(t *testing.T, s *Server, address string) (*Client, *Client, string, string) {
	a, b := testClient(t, address), testClient(t, address)
	if e := a.Send(Message{Type: "create"}); e != nil {
		t.Fatal(e)
	}
	u := waitUpdate(t, a, func(u Update) bool { return u.Type == "welcome" })
	b.Send(Message{Type: "join", Room: u.Room})
	waitUpdate(t, b, func(u Update) bool { return u.Type == "welcome" })
	a.Send(Message{Type: "ready"})
	b.Send(Message{Type: "ready"})
	waitUpdate(t, a, func(u Update) bool { return u.State == "countdown" })
	for n := 0; n < 183; n++ {
		s.Tick(time.Now())
	}
	waitUpdate(t, a, func(u Update) bool { return u.State == "fighting" })
	waitUpdate(t, b, func(u Update) bool { return u.State == "fighting" })
	return a, b, u.Room, u.Token
}
func TestTwoTCPClientsRoomsAuthorityReconnectAndRematch(t *testing.T) {
	s, address := testServer(t)
	a, b, code, token := createPair(t, s, address)
	a.SendInput(play.Input{PowerPressed: true})
	b.SendInput(play.Input{FanPressed: true})
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		r := s.rooms[code]
		ready := r.slots[0].seq > 0 && r.slots[1].seq > 0
		s.mu.Unlock()
		if ready {
			break
		}
		time.Sleep(time.Millisecond)
	}
	for n := 0; n < 90; n++ {
		s.Tick(time.Now())
	}
	u := waitUpdate(t, a, func(u Update) bool { return u.Snapshot != nil && u.Snapshot.Tick >= 90 })
	v := waitUpdate(t, b, func(u Update) bool {
		return u.Snapshot != nil && u.Snapshot.Tick == u.Snapshot.Tick && u.Snapshot.Tick >= 90
	})
	if u.Snapshot.Fighters[0].BuffStacks != 1 {
		t.Fatal("power not synchronized")
	}
	if v.Snapshot.Fighters[0].BuffStacks != 1 {
		t.Fatal("other client disagrees")
	}
	// A forged HP/damage field is rejected by the wire decoder.
	raw, e := net.Dial("tcp", address)
	if e != nil {
		t.Fatal(e)
	}
	defer raw.Close()
	fmt.Fprintf(raw, "{\"version\":1,\"type\":\"damage\",\"hp\":99999}\n")
	raw.SetReadDeadline(time.Now().Add(time.Second))
	line, _ := bufio.NewReader(raw).ReadString('\n')
	if !strings.Contains(line, "invalid_message") {
		t.Fatalf("forged packet accepted %s", line)
	}
	a.Close()
	waitUpdate(t, b, func(u Update) bool { return u.State == "disconnected" })
	resumed := testClient(t, address)
	resumed.Send(Message{Type: "resume", Room: code, Token: token})
	waitUpdate(t, resumed, func(u Update) bool { return u.Type == "welcome" })
	waitUpdate(t, b, func(u Update) bool { return u.State == "fighting" })
	// Duplicate and reordered sequences must not spend mana again.
	s.mu.Lock()
	slot := s.rooms[code].slots[0]
	seq := slot.seq
	before := s.rooms[code].combat.Fighters[0].MP
	s.mu.Unlock()
	resumed.Send(Message{Type: "input", Seq: seq, Input: play.Input{PowerPressed: true}})
	for n := 0; n < 3; n++ {
		s.Tick(time.Now())
	}
	s.mu.Lock()
	after := s.rooms[code].combat.Fighters[0].MP
	s.mu.Unlock()
	if after+1e-7 < before {
		t.Fatal("duplicate input spent mana")
	}
	// End the authoritative match and exercise a real rematch exchange.
	s.mu.Lock()
	r := s.rooms[code]
	r.combat.Arena.Clock = play.CombatDT
	r.combat.Fighters[0].BuffStacks = 4
	s.mu.Unlock()
	for n := 0; n < 170; n++ {
		s.Tick(time.Now())
	}
	waitUpdate(t, b, func(u Update) bool { return u.State == "over" })
	resumed.Send(Message{Type: "rematch"})
	b.Send(Message{Type: "rematch"})
	waitUpdate(t, b, func(u Update) bool { return u.State == "countdown" })
	s.mu.Lock()
	stacks := s.rooms[code].combat.Fighters[0].BuffStacks
	s.mu.Unlock()
	if stacks != 0 {
		t.Fatal("buff crossed complete new match")
	}
	resumed.Send(Message{Type: "leave"})
	waitUpdate(t, b, func(u Update) bool { return u.Type == "left" })
	if s.RoomCount() != 0 {
		t.Fatal("room leak")
	}
}

// This TCP message proxy delays each direction, adds deterministic jitter and
// drops selected input/snapshot updates. TCP itself remains a reliable ordered
// stream; protocol sequence tests separately cover stale/reordered inputs.
func latencyProxy(t *testing.T, target string, latency time.Duration) (string, func()) {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var connections []net.Conn
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			a, e := l.Accept()
			if e != nil {
				return
			}
			b, e := net.Dial("tcp", target)
			if e != nil {
				a.Close()
				continue
			}
			connections = append(connections, a, b)
			for direction := 0; direction < 2; direction++ {
				src, dst := a, b
				if direction == 1 {
					src, dst = b, a
				}
				go func(src, dst net.Conn, seed int64) {
					rng := rand.New(rand.NewSource(seed))
					scan := bufio.NewScanner(src)
					scan.Buffer(make([]byte, 4096), 128<<10)
					for scan.Scan() {
						line := append([]byte(nil), scan.Bytes()...)
						var m map[string]any
						_ = json.Unmarshal(line, &m)
						if (m["type"] == "input" || m["type"] == "state") && rng.Intn(20) == 0 {
							continue
						}
						delay := latency/2 + time.Duration(rng.Int63n(int64(latency/5+1)))
						timer := time.NewTimer(delay)
						select {
						case <-ctx.Done():
							timer.Stop()
							return
						case <-timer.C:
						}
						if _, e := dst.Write(append(line, '\n')); e != nil {
							return
						}
					}
				}(src, dst, int64(direction+7))
			}
		}
	}()
	closeFn := func() {
		cancel()
		l.Close()
		<-done
		for _, c := range connections {
			c.Close()
		}
	}
	return l.Addr().String(), closeFn
}
func TestLatencyJitterAndDroppedUpdates(t *testing.T) {
	for _, ms := range []int{50, 100, 200} {
		t.Run(fmt.Sprint(ms), func(t *testing.T) {
			s, address := testServer(t)
			proxy, stop := latencyProxy(t, address, time.Duration(ms)*time.Millisecond)
			defer stop()
			a := testClient(t, proxy)
			b := testClient(t, address)
			a.Send(Message{Type: "create"})
			room := waitUpdate(t, a, func(u Update) bool { return u.Type == "welcome" })
			b.Send(Message{Type: "join", Room: room.Room})
			waitUpdate(t, b, func(u Update) bool { return u.Type == "welcome" })
			a.Send(Message{Type: "ready"})
			b.Send(Message{Type: "ready"})
			waitUpdate(t, b, func(u Update) bool { return u.State == "countdown" })
			for n := 0; n < 181; n++ {
				s.Tick(time.Now())
			}
			waitUpdate(t, a, func(u Update) bool { return u.State == "fighting" })
			a.SendInput(play.Input{PowerPressed: true})
			time.Sleep(time.Duration(ms) * time.Millisecond)
			for n := 0; n < 30; n++ {
				s.Tick(time.Now())
			}
			u := waitUpdate(t, a, func(u Update) bool { return u.Snapshot != nil && u.Snapshot.Tick >= 30 })
			if u.Snapshot.Fighters[0].BuffStacks != 1 {
				t.Fatalf("skill missing under latency: tick=%d ack=%v mp=%v", u.Snapshot.Tick, u.Snapshot.Ack, u.Snapshot.Fighters[0].MP)
			}
			if u.Snapshot.Fighters[0].MP < 69 || u.Snapshot.Fighters[0].MP > 72 {
				t.Fatal("authority corrupted under latency")
			}
		})
	}
}
func TestVersionTimeoutIsolationAndResourceCleanup(t *testing.T) {
	s, address := testServer(t)
	a := testClient(t, address)
	a.Send(Message{Version: 999, Type: "nonsense"})
	waitUpdate(t, a, func(u Update) bool { return u.Type == "error" })
	a.Send(Message{Type: "create"})
	u := waitUpdate(t, a, func(u Update) bool { return u.Type == "welcome" })
	b := testClient(t, address)
	b.Send(Message{Type: "resume", Room: u.Room, Token: strings.Repeat("0", 48)})
	waitUpdate(t, b, func(u Update) bool { return u.Error == "invalid_resume" })
	a.Close()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		gone := s.rooms[u.Room].slots[0].peer == nil
		s.mu.Unlock()
		if gone {
			break
		}
		time.Sleep(time.Millisecond)
	}
	s.Tick(time.Now().Add(31 * time.Second))
	if s.RoomCount() != 0 {
		t.Fatal("expired room not removed")
	}
}

func TestIndependentRoomsIgnoreSpoofedRoomOnInput(t *testing.T) {
	s, address := testServer(t)
	a, _, first, _ := createPair(t, s, address)
	_, _, second, _ := createPair(t, s, address)
	if first == second {
		t.Fatal("room collision")
	}
	a.Send(Message{Type: "input", Room: second, Seq: 1, Input: play.Input{PowerPressed: true}})
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		received := s.rooms[first].slots[0].seq == 1
		s.mu.Unlock()
		if received {
			break
		}
		time.Sleep(time.Millisecond)
	}
	for n := 0; n < 30; n++ {
		s.Tick(time.Now())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rooms[first].combat.Fighters[0].BuffStacks != 1 || s.rooms[second].combat.Fighters[0].BuffStacks != 0 {
		t.Fatal("input crossed room boundary")
	}
}
