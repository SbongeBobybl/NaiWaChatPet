//go:build windows

package main

import (
	"codexpet/reconstruction/organizer"
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	"codexpet/reconstruction/rig"
	"codexpet/reconstruction/tidy"
)

// A tidy the pet carries out by hand. It lifts its box in the filmed scene,
// then for each kind of file walks to every file's icon and puts the file in
// the box, carries the box to that kind's folder and empties it there, and
// at the end puts the box down again in the filmed scene.
const (
	tripLift = iota
	tripLook
	tripToFile
	tripPick
	tripToFolder
	tripDrop
	tripStow
)

const (
	pickTime   = 420 * time.Millisecond
	flightTime = 480 * time.Millisecond
)

type tripGroup struct {
	kind, folder string
	moves        []tidy.Move
}

// flight is a file sailing between the desktop and the box.
type flight struct {
	sprite                 *sprite
	kind                   string
	fromX, fromY, toX, toY float64
	born                   time.Duration
	tilt                   float64
	// land puts a sparkle where a delivered file lands.
	land bool
}

type iconAnswer struct {
	icons []deskIcon
	err   error
}

type tidyTrip struct {
	external                   *externalCarry
	ctx                        context.Context
	done                       chan tidyOutcome
	progress                   func(organizer.Status)
	total, delivered, failed   int
	pending                    []tidy.Move
	err                        error
	destinationX, destinationY float64

	session  *tidy.Session
	groups   []tripGroup
	group    int
	picked   int
	target   tidy.Move
	moved    bool
	stage    int
	since    time.Duration
	icons    map[string]deskIcon
	asking   bool
	answers  chan iconAnswer
	blind    bool
	flights  []*flight
	stopping bool
	left     int
}

// startTrip begins a tidy carried out by hand.
func (a *desktopHost) startTrip(plan tidy.Plan, journals string) error {
	session, err := tidy.Begin(plan, journals)
	if err != nil {
		return err
	}
	// The folders go up first so their icons are there to walk to.
	session.Prepare(plan.Moves)
	t := &tidyTrip{session: session, answers: make(chan iconAnswer, 1), since: a.now()}
	for _, c := range tidy.Counts(plan.Moves) {
		g := tripGroup{kind: c.Kind}
		for _, m := range plan.Moves {
			if m.Kind == c.Kind {
				g.moves = append(g.moves, m)
				g.folder = filepath.Dir(m.To)
			}
		}
		t.groups = append(t.groups, g)
	}
	a.setControl(false)
	if a.chat != nil {
		a.chat.HideInput()
	}
	if a.emotions != nil {
		a.emotions.InterruptAnimation("tidy")
	}
	a.trip = t
	p := a.play
	p.showing, p.summoned = false, false
	a.emote("organize", 30*time.Second)
	t.ask()
	return nil
}

// ask fetches fresh icon positions in the background; Explorer reflows the
// icons as files leave the desktop.
func (t *tidyTrip) ask() {
	if t.asking {
		return
	}
	t.asking = true
	go func() {
		icons, err := readDesktopIcons()
		t.answers <- iconAnswer{icons, err}
	}()
}

// stopTrip asks the pet to finish the trip it is on and put the box down.
func (a *desktopHost) stopTrip() {
	if t := a.trip; t != nil {
		t.stopping = true
	}
}

// stepTrip runs the tidy by hand for one tick.
func (a *desktopHost) stepTrip(now time.Duration) {
	t, p := a.trip, a.play
	if t == nil || p == nil {
		return
	}
	select {
	case r := <-t.answers:
		t.asking = false
		if r.err == nil && len(r.icons) > 0 {
			t.icons = iconIndex(r.icons)
			t.injectExternalTarget()
		} else if t.icons == nil {
			_, _ = logDiagnostic(fmt.Errorf("tidy: desktop icons unavailable, tidying without walking: %v", r.err))
			t.blind = true
		}
	default:
	}
	a.stepFlights(now)
	elapsed := now - t.since
	switch t.stage {
	case tripLift:
		// The filmed lift plays first; the icons arrive meanwhile.
		liftDuration := 2300 * time.Millisecond
		if t.external != nil {
			liftDuration = 350 * time.Millisecond
		}
		if elapsed < liftDuration || t.icons == nil && !t.blind && elapsed < 4*time.Second {
			return
		}
		if t.icons == nil {
			t.blind = true
		}
		if t.blind {
			t.err = errors.New("无法读取桌面图标位置，已停止搬运，文件保持原位")
			t.stopping = true
			a.nextLeg(now)
			return
		}
		// Leave the film and carry the box from here.
		if a.emotions != nil {
			a.emotions.InterruptAnimation("carry")
		}
		p.carrying = []string{}
		a.nextLeg(now)
	case tripLook:
		// Wait for gathered files to reach the box, and briefly for icons that
		// may have moved since the last file left.
		if elapsed < 0 || t.asking && elapsed < 600*time.Millisecond {
			return
		}
		a.nextLeg(now)
	case tripToFile, tripToFolder:
		// A drag or an unreachable target must not count as successful arrival.
		if !p.ctl.Travelling() && elapsed > 300*time.Millisecond {
			x, y := p.ctl.Position()
			if math.Hypot(x-t.destinationX, y-t.destinationY) > 35 {
				t.err = errors.New("搬运路线被打断或目标不可达，已停止")
				t.pending = nil
				p.carrying = []string{}
				t.stopping = true
				a.nextLeg(now)
			} else {
				a.tripArrived(now)
			}
		}
	case tripPick:
		p.carrySquash = .11 * math.Sin(math.Pi*math.Min(1, float64(elapsed)/float64(pickTime)))
		if !t.moved && elapsed >= pickTime*2/5 {
			t.moved = true
			a.pickUp(t.target, now, 0)
		}
		if elapsed >= pickTime {
			p.carrySquash = 0
			t.picked++
			t.stage, t.since = tripLook, now
			t.ask()
		}
	case tripDrop:
		hold := pickTime + time.Duration(len(p.carrying))*90*time.Millisecond + flightTime
		p.carrySquash = .1 * math.Sin(math.Pi*math.Min(1, float64(elapsed)/float64(pickTime)))
		if !t.moved && elapsed >= pickTime*2/5 {
			t.moved = true
			a.deliver(now)
		}
		if elapsed >= hold {
			p.carrySquash = 0
			if len(t.groups[t.group].moves) == 0 {
				t.group++
			}
			t.picked = 0
			t.stage, t.since = tripLook, now
			t.ask()
		}
	case tripStow:
		// The filmed put-down plays out before the pet reports.
		stowDuration := 2100 * time.Millisecond
		if t.external != nil {
			stowDuration = 350 * time.Millisecond
		}
		if elapsed >= stowDuration && len(t.flights) == 0 {
			a.finishTrip()
		}
	}
}

// nextLeg sends the pet to its next file or folder, or home.
func (a *desktopHost) nextLeg(now time.Duration) {
	t, p := a.trip, a.play
	for t.group < len(t.groups) && len(t.groups[t.group].moves) == 0 && len(p.carrying) == 0 {
		t.group++
	}
	if t.stopping && len(p.carrying) == 0 || t.group >= len(t.groups) {
		for _, g := range t.groups[min(t.group, len(t.groups)):] {
			t.left += len(g.moves)
		}
		p.carrying = nil
		t.stage, t.since = tripStow, now
		if a.emotions != nil {
			a.emotions.QueueAnimation("organize_out", 45)
		}
		return
	}
	g := &t.groups[t.group]
	t.moved = false
	if len(g.moves) > 0 && (t.external != nil || len(t.pending) == 0) && !t.stopping {
		// The nearest file next, so the pet does not zigzag.
		best, bestDistance := 0, math.Inf(1)
		bx, by := a.boxOnScreen()
		for i, m := range g.moves {
			if icon, ok := t.icons[strings.ToLower(m.From)]; ok {
				if d := math.Hypot(float64(icon.x)-bx, float64(icon.y)-by); d < bestDistance {
					best, bestDistance = i, d
				}
			}
		}
		t.target = g.moves[best]
		g.moves = append(g.moves[:best], g.moves[best+1:]...)
		if icon, ok := t.icons[strings.ToLower(t.target.From)]; ok {
			x, y := a.besideIcon(icon)
			t.destinationX, t.destinationY = x, y
			p.ctl.Goto(x, y, true)
			t.stage, t.since = tripToFile, now
			t.describe("走向文件：" + filepath.Base(t.target.From))
			return
		}
		t.err = fmt.Errorf("找不到文件图标：%s，已停止搬运", filepath.Base(t.target.From))
		t.stopping = true
		a.nextLeg(now)
		return
	}
	// Box full for this kind: off to its folder.
	if icon, ok := t.icons[strings.ToLower(g.folder)]; ok {
		x, y := a.besideIcon(icon)
		t.destinationX, t.destinationY = x, y
		p.ctl.Goto(x, y, true)
		t.stage, t.since = tripToFolder, now
		t.describe("抱箱送往：" + filepath.Base(g.folder))
		return
	}
	t.err = fmt.Errorf("找不到目标文件夹图标：%s，文件保持原位", filepath.Base(g.folder))
	t.pending = nil
	p.carrying = []string{}
	t.stopping = true
	a.nextLeg(now)
}

// tripArrived is told when the pet reaches a file or a folder.
func (a *desktopHost) tripArrived(now time.Duration) {
	t := a.trip
	switch t.stage {
	case tripToFile:
		t.stage, t.since, t.moved = tripPick, now, false
	case tripToFolder:
		if t.external != nil {
			if t.stopping || t.err != nil {
				t.pending = nil
				a.play.carrying = []string{}
				t.stopping = true
				a.nextLeg(now)
				return
			}
			t.stage, t.since = tripAwaitReceipt, now
			t.external.arrived = true
			t.external.ready <- nil
			return
		}
		t.stage, t.since, t.moved = tripDrop, now, false
	}
}

// besideIcon is where the pet's feet go so that its box is just under an
// icon, on whichever side it is coming from, kept on that icon's screen.
func (a *desktopHost) besideIcon(icon deskIcon) (float64, float64) {
	f := a.factor()
	reach := (feetAnchor - rig.CarryRest.X) * f
	x := float64(icon.x) + reach
	if a.feetX() < float64(icon.x) {
		x = float64(icon.x) - reach
	}
	y := float64(icon.y) + (rig.GroundY-rig.CarryRest.Y+60)*f
	work, ok := workAreaAt(icon.x, icon.y, true)
	if !ok {
		return x, y
	}
	return math.Max(float64(work.Left)+125*f, math.Min(float64(work.Right)-125*f, x)),
		math.Max(float64(work.Top)+505*f, math.Min(float64(work.Bottom)-3, y))
}

// boxOnScreen is the mouth of the pet's box in screen pixels.
func (a *desktopHost) boxOnScreen() (float64, float64) {
	p := a.play
	at := rig.Carry(p.stand, rig.StandPose(p.view.Gait).Add(rig.Settle(p.stand, p.carrySquash)))
	x := at.X
	if p.flip {
		x = rig.CanvasWidth - x
	}
	f := a.factor()
	return float64(a.x) + x*f, float64(a.y) + (at.Y-80)*f
}

// pickUp stages the visual pickup. The journalled move happens only at delivery.
func (a *desktopHost) pickUp(m tidy.Move, now, delay time.Duration) {
	t, p := a.trip, a.play
	done := m
	t.pending = append(t.pending, m)
	t.describe("装箱：" + filepath.Base(m.From))
	bx, by := a.boxOnScreen()
	fx, fy := bx, by-200*a.factor()
	if icon, known := t.icons[strings.ToLower(done.From)]; known {
		fx, fy = float64(icon.x), float64(icon.y)
	}
	t.flights = append(t.flights, &flight{kind: done.Kind, fromX: fx, fromY: fy, toX: bx, toY: by, born: now + delay, tilt: -.3})
	p.carrying = append(p.carrying, done.Kind)
}

// deliver empties the box into the folder the pet is standing at.
func (a *desktopHost) deliver(now time.Duration) {
	t, p := a.trip, a.play
	bx, by := a.boxOnScreen()
	tx, ty := bx, by-160*a.factor()
	if t.group < len(t.groups) {
		if icon, ok := t.icons[strings.ToLower(t.groups[t.group].folder)]; ok {
			tx, ty = float64(icon.x), float64(icon.y)
		}
	}
	for i, m := range t.pending {
		done, ok := m, false
		if t.external != nil {
			ok = t.external.success
		} else {
			done, ok = t.session.Move(m)
		}
		if !ok {
			t.failed++
			continue
		}
		t.delivered++
		kind := done.Kind
		t.flights = append(t.flights, &flight{kind: kind, fromX: bx, fromY: by, toX: tx, toY: ty, born: now + time.Duration(i)*90*time.Millisecond, tilt: .3, land: i == len(p.carrying)-1})
	}
	t.pending = nil
	p.carrying = []string{}
	if t.progress != nil {
		t.progress(organizer.Status{Phase: "moving", Moved: t.delivered, Total: t.total, Skipped: t.failed, Message: "奶蛙已把文件放入目标文件夹"})
	}
}

// stepFlights moves the files in the air along little arcs.
func (a *desktopHost) stepFlights(now time.Duration) {
	t, p := a.trip, a.play
	if p.class == nil {
		t.flights = nil
		return
	}
	size := max(28, int(80*a.factor()))
	kept := t.flights[:0]
	for _, f := range t.flights {
		if now < f.born {
			kept = append(kept, f)
			continue
		}
		u := float64(now-f.born) / float64(flightTime)
		if u >= 1 {
			f.sprite.close()
			if f.land {
				p.spawn(effect{x: f.toX, y: f.toY, born: now, size: size * 2})
			}
			continue
		}
		if f.sprite == nil {
			f.sprite, _ = p.class.open()
		}
		ease := u * u * (3 - 2*u)
		x := f.fromX + (f.toX-f.fromX)*ease
		y := f.fromY + (f.toY-f.fromY)*ease - 4*90*a.factor()*u*(1-u)
		shrink := max(16, int(float64(size)*(1-.35*u)))
		_ = f.sprite.draw(int(x)-shrink/2, int(y)-shrink/2, shrink, shrink, docSprite(f.kind, shrink, f.tilt*(1-2*u)))
		kept = append(kept, f)
	}
	t.flights = kept
}

// finishTrip closes the journal and has the pet say what it did.
func (a *desktopHost) finishTrip() {
	t := a.trip
	a.trip = nil
	a.play.carrying = nil
	for _, f := range t.flights {
		f.sprite.close()
	}
	if t.external != nil {
		if !t.external.arrived {
			select {
			case t.external.ready <- errors.Join(errors.New("carry did not arrive"), t.err):
			default:
			}
		}
		if t.external.completed != nil {
			// A negative commit is a valid receipt, not an IPC transport failure.
			if !t.external.success {
				t.external.completed <- nil
			} else {
				t.external.completed <- t.err
			}
		}
		if !t.external.success {
			a.say("这次搬运已停止，原文件没有由奶蛙移动。")
		}
		return
	}
	report, err := t.session.Finish()
	err = errors.Join(err, t.err)
	if t.done != nil {
		t.done <- tidyOutcome{report: report, err: err}
	}
	text := tidySummary(report, false)
	if t.left > 0 {
		text += fmt.Sprintf("\n这次先停在这里，还有 %d 个文件没整理。", t.left)
	}
	if err != nil {
		_, _ = logDiagnostic(fmt.Errorf("tidy: %w", err))
		text += "\n" + err.Error()
	}
	a.say(text)
	if len(report.Moved) > 0 {
		a.emote("proud", 4*time.Second)
	}
}

func (t *tidyTrip) describe(message string) {
	if t.progress != nil {
		t.progress(organizer.Status{Phase: "moving", Moved: t.delivered, Total: t.total, Skipped: t.failed, Message: message})
	}
}
