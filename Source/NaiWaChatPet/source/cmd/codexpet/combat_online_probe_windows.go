//go:build windows

package main

import (
	"context"
	"fmt"
	"math"
	"net"
	"time"

	"codexpet/reconstruction/fightnet"
	"codexpet/reconstruction/play"
)

type onlineCombatProbe struct {
	close      func()
	guest      *fightnet.Client
	rematch    bool
	matches    int
	samples    []map[string]any
	nextSample time.Time
}

// This is enabled only by the explicit isolated performance probe flags.
// Both sides send ordinary client inputs to an unmodified authoritative server.
func (a *desktopHost) startOnlineCombatProbe() (*onlineCombatProbe, error) {
	u := a.prepareOnline()
	if u == nil {
		return nil, fmt.Errorf("online probe: game unavailable")
	}
	ctx, cancel := context.WithCancel(context.Background())
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		cancel()
		return nil, err
	}
	s := fightnet.NewServer(fightnet.ServerConfig{})
	done := make(chan struct{})
	go func() { defer close(done); _ = s.Serve(ctx, l) }()
	cleanup := func() { cancel(); _ = s.Close(); <-done; s.Wait() }
	owner, err := fightnet.Dial(ctx, l.Addr().String())
	if err != nil {
		cleanup()
		return nil, err
	}
	guest, err := fightnet.Dial(ctx, l.Addr().String())
	if err != nil {
		owner.Close()
		cleanup()
		return nil, err
	}
	closeAll := func() { owner.Close(); guest.Close(); cleanup() }
	wait := func(c *fightnet.Client, kind string) (fightnet.Update, error) {
		timer := time.NewTimer(3 * time.Second)
		defer timer.Stop()
		for {
			select {
			case update := <-c.Updates():
				if update.Type == kind {
					return update, nil
				}
			case <-timer.C:
				return fightnet.Update{}, fmt.Errorf("online probe: waiting for %s: %v", kind, c.Error())
			}
		}
	}
	if err = owner.Send(fightnet.Message{Type: "create"}); err != nil {
		closeAll()
		return nil, err
	}
	welcome, err := wait(owner, "welcome")
	if err != nil {
		closeAll()
		return nil, err
	}
	if err = guest.Send(fightnet.Message{Type: "join", Room: welcome.Room}); err != nil {
		closeAll()
		return nil, err
	}
	if _, err = wait(guest, "welcome"); err != nil {
		closeAll()
		return nil, err
	}
	u.client, u.address, u.room, u.token = owner, l.Addr().String(), welcome.Room, welcome.Token
	u.networkState = "waiting"
	if err = owner.Send(fightnet.Message{Type: "ready"}); err != nil {
		closeAll()
		return nil, err
	}
	if err = guest.Send(fightnet.Message{Type: "ready"}); err != nil {
		closeAll()
		return nil, err
	}
	go func() {
		ticker := time.NewTicker(time.Second / 60)
		defer ticker.Stop()
		var tick uint64
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				tick++
				state := guest.State()
				if state.State == "fighting" && state.Snapshot != nil {
					_ = guest.SendInput(onlineProbeInput(*state.Snapshot, 1, tick))
				}
			}
		}
	}()
	return &onlineCombatProbe{guest: guest, close: closeAll, nextSample: time.Now().Add(15 * time.Second)}, nil
}

func onlineProbeInput(s play.CombatSnapshot, player int, tick uint64) play.Input {
	f, other := s.Fighters[player], s.Fighters[1-player]
	dx, dy := other.X-f.X, other.Y-f.Y
	in := play.Input{Left: dx < -210, Right: dx > 210, Up: dy < -20, Down: dy > 20}
	in.AttackPressed = tick%18 == 0 && math.Abs(dx) < 300
	in.FanPressed = tick%155 == 0
	in.RollPressed = tick%213 == 0
	in.PowerPressed = tick%301 == 0
	in.DodgePressed = tick%97 == 0
	return in
}

func (a *desktopHost) sampleOnlineCombatProbe(p *onlineCombatProbe) {
	u := a.play.fight.upgraded
	if u.networkState == "over" && !p.rematch && u.client != nil {
		p.rematch = true
		p.matches++
		owner, guest := u.client, p.guest
		go func() {
			_ = owner.Send(fightnet.Message{Type: "rematch"})
			_ = guest.Send(fightnet.Message{Type: "rematch"})
		}()
	}
	if u.networkState == "fighting" {
		p.rematch = false
	}
	if time.Now().Before(p.nextSample) {
		return
	}
	p.nextSample = time.Now().Add(15 * time.Second)
	gdi, user := guiResources()
	errText := ""
	if u.client == nil {
		errText = "local client disconnected"
	} else if err := u.client.Error(); err != nil {
		errText = err.Error()
	}
	p.samples = append(p.samples, map[string]any{"seconds": time.Since(combatProbeData.Started).Seconds(), "tick": u.snapshot.Tick, "state": u.networkState, "gdi": gdi, "user": user, "sprites": ownedSpriteCount(a), "effects": len(a.play.effects), "skillCacheEntries": len(u.frameCache), "error": errText, "fighters": u.snapshot.Fighters})
}
