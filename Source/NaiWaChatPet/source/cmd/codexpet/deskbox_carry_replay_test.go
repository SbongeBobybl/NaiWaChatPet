package main

import (
	ai "codexpet/reconstruction/aiorganization"
	"context"
	"testing"
	"time"
)

// Regression for "the carry animation only plays the first time".
//
// DeskBox drives one trip per category (full animation) or one trip per run
// (simple animation). After a trip has handed its files over it sits in
// tripAwaitReceipt until DeskBox commits; if it is still registered when the
// next carry arrives, stepExternalCarry used to answer "pet busy" and every
// later organize run silently skipped the animation.
//
// A trip that already delivered, or whose request context is gone, must be
// finished and replaced instead.
func TestSecondCarryReplacesACompletedExternalTrip(t *testing.T) {
	a, _ := carryFixture(t)
	a.trip.session = nil

	// The previous DeskBox request finished, so its context is cancelled.
	previousCtx, cancelPrevious := context.WithCancel(context.Background())
	cancelPrevious()

	stale := &tidyTrip{
		external: &externalCarry{
			id:        "first-carry",
			ctx:       previousCtx,
			ready:     make(chan error, 1),
			completed: make(chan error, 1),
			arrived:   true,
			success:   true,
		},
		stage:   tripAwaitReceipt,
		answers: make(chan iconAnswer, 1),
		since:   a.now(),
		groups:  []tripGroup{{kind: "文件", folder: externalTarget}},
	}
	a.trip = stale

	next := &externalCarry{
		id:    "second-carry",
		ctx:   context.Background(),
		ready: make(chan error, 1),
		move:  ai.CarryMove{SourcePath: `C:\fixture\a.txt`, DestinationX: 40, DestinationY: 40},
	}
	externalCarries <- next

	a.stepExternalCarry()

	if a.trip == stale {
		t.Fatal("a completed external trip still blocks the next carry (animation would be skipped)")
	}
	// finishTrip releases the DeskBox receipt of the finished trip; a trip that
	// was left waiting for it would leak that waiter forever.
	select {
	case <-stale.external.completed:
	default:
		t.Fatal("the finished trip never released its receipt")
	}
}

func TestSecondCarryTearsDownATimedOutWalk(t *testing.T) {
	a, _ := carryFixture(t)

	expired, cancel := context.WithCancel(context.Background())
	cancel()

	stuck := &tidyTrip{
		external: &externalCarry{
			id:        "walked-away",
			ctx:       expired,
			ready:     make(chan error, 1),
			completed: make(chan error, 1),
		},
		stage:   tripToFile,
		answers: make(chan iconAnswer, 1),
		since:   a.now() - 2*externalWalkWatchdog,
		groups:  []tripGroup{{kind: "文件", folder: externalTarget}},
	}
	a.trip = stuck

	externalCarries <- &externalCarry{
		id:    "next",
		ctx:   context.Background(),
		ready: make(chan error, 1),
		move:  ai.CarryMove{SourcePath: `C:\fixture\b.txt`, DestinationX: 40, DestinationY: 40},
	}
	a.stepExternalCarry()

	if a.trip == stuck {
		t.Fatal("a trip whose request context expired still blocks the next carry")
	}
	_ = time.Second
}
