//go:build windows

package main

import (
	"image"
	"math"
	"sync/atomic"
	"time"

	"codexpet/reconstruction/rig"
)

// Go's Windows runtime uses high-resolution waitable timers. A coalescing
// message keeps all Win32 drawing on the UI thread and prevents queue growth.
type combatClock struct {
	stop, done chan struct{}
	presented  chan struct{}
	queued     atomic.Bool
	rendering  bool // UI thread only; Win32 calls can dispatch nested messages.
	owner      *desktopHost
}

func (a *desktopHost) startCombatClock(u *combatPresentation) {
	u.originalScale, u.originalDPI = a.scale, a.dpi
	a.fitCombatWindow(u)
	_, _ = call(killTimer, a.hwnd, 1)
	a.timerReady = false
	clock := &combatClock{stop: make(chan struct{}), done: make(chan struct{}), presented: make(chan struct{}, 1), owner: a}
	u.clock = clock
	go func() {
		defer close(clock.done)
		ticker := time.NewTicker(time.Second / 60)
		defer ticker.Stop()
		retry := time.NewTimer(time.Hour)
		retry.Stop()
		defer retry.Stop()
		var retryC <-chan time.Time
		late := false
		post := func() {
			clock.queued.Store(true)
			if ok, _ := call(postMessage, a.hwnd, wmTimer, 1, 0); ok == 0 {
				clock.queued.Store(false)
			}
		}
		for {
			select {
			case <-clock.stop:
				return
			case <-ticker.C:
				if clock.queued.Load() {
					late = true
				} else {
					post()
				}
			case <-clock.presented:
				if late {
					// Finish the whole frame, then leave a real input-service gap.
					// Do not wait another full 16.7ms after an overrun, or post
					// continuous high-priority frames ahead of hardware input.
					late = false
					retry.Reset(2 * time.Millisecond)
					retryC = retry.C
				} else {
					clock.queued.Store(false)
				}
			case <-retryC:
				retryC = nil
				late = false
				post()
			}
		}
	}()

}
func (c *combatClock) close() {
	if c == nil {
		return
	}
	close(c.stop)
	<-c.done
	a := c.owner
	if !a.closing {
		ok, _ := call(setTimer, a.hwnd, 1, 16, 0)
		a.timerReady = ok != 0
	}
}
func (a *desktopHost) renderCombatRig(p *rig.Puppet, texture *rig.Texture, pose rig.Pose) (*image.NRGBA, error) {
	if a.play != nil && a.play.fight != nil {
		return p.RenderSized(texture, pose, a.w, a.h)
	}
	return p.Render(texture, pose)
}

func (a *desktopHost) fitCombatWindow(u *combatPresentation) {
	work, ok := workAreaAt(int(a.feetX()), int(a.groundLine())-2, true)
	if !ok {
		return
	}
	limit := math.Min(float64(work.Right-work.Left-40)/1800, float64(work.Bottom-work.Top-180)/840)
	limit = math.Max(.08, limit)
	if a.factor() <= limit {
		return
	}
	width := max(24, int(math.Floor(420*limit)))
	height := max(32, int(math.Round(float64(width)*560/420)))
	if err := a.surface.Resize(width, height); err != nil {
		_, _ = logDiagnostic(err)
		return
	}
	a.w, a.h = width, height
	u.windowReduced = true
	a.haveKey = false
}
