package main

import (
	ai "codexpet/reconstruction/aiorganization"
	"codexpet/reconstruction/companion"
	"codexpet/reconstruction/tidy"
	"context"
	"errors"
	"strings"
	"time"
)

type deskBoxHandler struct{ companion.DeskBoxAgent }
type externalCarry struct {
	id        string
	move      ai.CarryMove
	ctx       context.Context
	ready     chan error
	completed chan error
	arrived   bool
	success   bool
}
type carryReceipt struct {
	id      string
	success bool
	done    chan error
}

var externalCarries = make(chan *externalCarry, 1)
var externalReceipts = make(chan carryReceipt, 1)

const tripAwaitReceipt = 99
const externalTarget = "deskbox-animation-target"

// externalWalkWatchdog bounds one walking stage of a DeskBox-driven carry.
// Without it a single stuck trip (pet dragged away mid-walk, Explorer reflow
// removing the icon the pet was walking to, a lost answer channel) left
// a.trip non-nil, and every later carry in the same session was rejected as
// "pet busy" — which the user saw as "the animation only plays the first
// time". A stuck external trip is now closed and reported instead.
const externalWalkWatchdog = 20 * time.Second

func (h deskBoxHandler) Carry(ctx context.Context, r ai.Request) error {
	carry := &externalCarry{id: r.RequestID, move: *r.Carry, ctx: ctx, ready: make(chan error, 1)}
	select {
	case externalCarries <- carry:
	case <-ctx.Done():
		return ctx.Err()
	case <-carryShutdown:
		return errors.New("pet closed")
	}
	select {
	case err := <-carry.ready:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-carryShutdown:
		return errors.New("pet closed")
	}
}
func (h deskBoxHandler) CompleteCarry(ctx context.Context, r ai.Request) error {
	receipt := carryReceipt{r.CarryRequestID, r.Succeeded, make(chan error, 1)}
	select {
	case externalReceipts <- receipt:
	case <-ctx.Done():
		return ctx.Err()
	case <-carryShutdown:
		return errors.New("pet closed")
	}
	select {
	case err := <-receipt.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-carryShutdown:
		return errors.New("pet closed")
	}
}
func (a *desktopHost) stepExternalCarry() {
	select {
	case c := <-externalCarries:
		if c.ctx.Err() != nil {
			c.ready <- c.ctx.Err()
			break
		}
		if a.trip != nil {
			// The animation must replay on every organizer update. A trip that
			// already handed its files to DeskBox (awaiting/completed receipt)
			// or whose request context is gone is finished work, not an active
			// animation: tear it down instead of rejecting the next category.
			if t := a.trip; t.external != nil &&
				(t.stage == tripAwaitReceipt || t.stopping || t.external.completed != nil || t.external.ctx.Err() != nil) {
				_, _ = logDiagnostic(errors.New("carry: finishing a completed external trip before the next carry"))
				a.finishTrip()
			}
		}
		if a.trip != nil || a.tidying != nil || a.play == nil || a.play.stand == nil || a.play.class == nil || a.play.fight != nil {
			c.ready <- errors.New("pet busy")
			break
		}
		if _, ok := workAreaAt(int(c.move.DestinationX), int(c.move.DestinationY), false); !ok {
			c.ready <- errors.New("destination unavailable")
			break
		}
		paths := c.move.SourcePaths
		if len(paths) == 0 {
			paths = []string{c.move.SourcePath}
		}
		moves := make([]tidy.Move, 0, len(paths))
		for _, path := range paths {
			moves = append(moves, tidy.Move{From: path, To: externalTarget, Kind: "文件"})
		}
		t := &tidyTrip{external: c, total: len(moves), answers: make(chan iconAnswer, 1), since: a.now(), groups: []tripGroup{{kind: "文件", folder: externalTarget, moves: moves}}}
		a.setControl(false)
		if a.chat != nil {
			a.chat.HideInput()
			a.chat.hideReply()
		}
		if a.emotions != nil {
			a.emotions.InterruptAnimation("tidy")
		}
		a.trip = t
		a.play.showing, a.play.summoned = false, false
		a.emote("organize", 30*time.Second)
		t.ask()
	default:
	}
	select {
	case receipt := <-externalReceipts:
		t := a.trip
		if t == nil || t.external == nil || t.external.id != receipt.id || !t.external.arrived || t.external.completed != nil {
			receipt.done <- errors.New("stale carry receipt")
			break
		}
		t.external.completed = receipt.done
		t.external.success = receipt.success
		if receipt.success {
			t.stage, t.since, t.moved = tripDrop, a.now(), false
		} else {
			t.pending = nil
			a.play.carrying = []string{}
			t.stopping = true
			t.err = errors.New("DeskBox 未提交此文件，原文件保持原位")
			a.nextLeg(a.now())
		}
	default:
	}
	if t := a.trip; t != nil && t.external != nil {
		// A walking stage that never advances would otherwise hold a.trip for
		// the rest of the session. Awaiting the file receipt is excluded: that
		// wait is DeskBox moving files, which has its own 15 minute ceiling.
		if t.stage != tripAwaitReceipt && a.now()-t.since > externalWalkWatchdog {
			_, _ = logDiagnostic(errors.New("carry: external trip watchdog fired, resetting trip"))
			a.finishTrip()
			return
		}
		if (!t.external.arrived && t.external.ctx.Err() != nil) || (t.stage == tripAwaitReceipt && a.now()-t.since > 15*time.Minute) {
			t.pending = nil
			a.play.carrying = []string{}
			t.stopping = true
			t.err = errors.New("搬运已取消，尚未收到文件提交回执")
			a.nextLeg(a.now())
		}
	}
}
func (t *tidyTrip) injectExternalTarget() {
	if t.external != nil && t.icons != nil {
		t.icons[strings.ToLower(externalTarget)] = deskIcon{path: externalTarget, x: int(t.external.move.DestinationX), y: int(t.external.move.DestinationY)}
	}
}
