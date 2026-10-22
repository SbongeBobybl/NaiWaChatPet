package fightnet

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"codexpet/reconstruction/play"
)

func TestRematchSnapshotRecoversWhenCountdownWasMissed(t *testing.T) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	done := make(chan struct{})
	stop := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		encoder := json.NewEncoder(conn)
		for _, u := range []Update{
			{Version: ProtocolVersion, Type: "state", Match: 1, State: "fighting", Snapshot: &play.CombatSnapshot{Tick: 9000}},
			{Version: ProtocolVersion, Type: "state", Match: 2, State: "fighting", Snapshot: &play.CombatSnapshot{Tick: 30}},
			{Version: ProtocolVersion, Type: "state", Match: 1, State: "fighting", Snapshot: &play.CombatSnapshot{Tick: 9003}},
		} {
			if encoder.Encode(u) != nil {
				return
			}
		}
		<-stop
	}()
	defer func() { close(stop); l.Close(); <-done }()
	c, err := Dial(context.Background(), l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	waitUpdate(t, c, func(u Update) bool { return u.Match == 2 })
	time.Sleep(10 * time.Millisecond)
	state := c.State()
	if state.Match != 2 || state.Snapshot.Tick != 30 {
		t.Fatalf("new match stuck behind old tick: %+v", state)
	}
}

func TestInputWriterCoalescesStalledFrames(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	c := &Client{conn: a, inputQueue: make(chan Message, 8), done: make(chan struct{})}
	defer c.Close()
	for seq := uint64(1); seq <= 8; seq++ {
		c.inputQueue <- Message{Type: "input", Seq: seq, Input: play.Input{Left: seq < 8, Right: seq == 8, FanPressed: seq == 1, AttackPressed: seq == 4}}
	}
	go c.writeInputs()
	b.SetReadDeadline(time.Now().Add(time.Second))
	var msg Message
	if err := json.NewDecoder(bufio.NewReader(b)).Decode(&msg); err != nil {
		t.Fatal(err)
	}
	if msg.Seq != 8 || msg.Input.Left || !msg.Input.Right || !msg.Input.FanPressed || !msg.Input.AttackPressed {
		t.Fatalf("old movements replayed or press edges lost: %+v", msg)
	}
}

func TestRoomSnapshotRateStaysBoundedAfterMatch(t *testing.T) {
	s := NewServer(ServerConfig{ManualTicks: true})
	p := &peer{player: 0, out: make(chan Update, 100), done: make(chan struct{})}
	c := play.NewCombat(false)
	r := &room{code: "ABC123", state: "over", combat: c, slots: [2]*slot{{peer: p}, nil}}
	s.rooms[r.code] = r
	for tick := 0; tick < 60; tick++ {
		s.Tick(time.Now())
	}
	if got := len(p.out); got != 20 {
		t.Fatalf("finished match broadcasts %d times/sec; want 20", got)
	}
}
