// Package animation is a reconstruction of the requested behavior, not recovered V10 code.
package animation

import (
	"errors"
	"fmt"
	"time"
)

type State string

const (
	Idle      State = "idle"
	ToFight   State = "toFight"
	Fight     State = "fight"
	FromFight State = "fromFight"
	ToLaugh   State = "toLaugh"
	Laugh     State = "video"
	FromLaugh State = "fromLaugh"
	ToEat     State = "toEat"
	Eat       State = "eat"
	FromEat   State = "fromEat"
	ToBack    State = "toBack"
	Poop      State = "poop"
	FromBack  State = "fromBack"
	AfterPoop State = "afterPoop"
)

var States = []State{Idle, ToFight, Fight, FromFight, ToLaugh, Laugh, FromLaugh, ToEat, Eat, FromEat, ToBack, Poop, FromBack, AfterPoop}

type Clip struct {
	Frames int
	Step   time.Duration
}

func (c Clip) duration() time.Duration { return time.Duration(c.Frames) * c.Step }

// Config is explicit: no unverified V10 frame duration is silently assumed.
// BeginDropFrame is a marker inside the Poop clip, after turning has finished.
type Config struct {
	Clips          map[State]Clip
	DigestDelay    time.Duration
	BeginDropFrame int
}

type Input int

const (
	HoverOn Input = iota
	HoverOff
	DoubleClick
	Contact
	FilesStored
)

type Event struct {
	Kind  string
	At    time.Duration
	State State
	Frame int
}

type Snapshot struct {
	State         State
	Frame         int
	StateStarted  time.Duration
	DropVisible   bool
	DropAge       time.Duration
	DigestPending bool
}

// Engine owns a single monotonic timeline. Window motion and DPI are not clocks.
type Engine struct {
	cfg                      Config
	now, entered             time.Duration
	state                    State
	hover                    bool
	digestPending            bool
	digestDue                time.Duration
	dropVisible, markerFired bool
	dropBorn                 time.Duration
	queuedAction             int
	events                   []Event
	suspended                bool
}

func New(cfg Config) (*Engine, error) {
	if cfg.DigestDelay <= 0 || cfg.DigestDelay > 24*time.Hour {
		return nil, errors.New("DigestDelay must be between zero and 24 hours")
	}
	clips := make(map[State]Clip, len(cfg.Clips))
	for _, state := range States {
		c, ok := cfg.Clips[state]
		if !ok || c.Frames <= 0 || c.Step <= 0 || int64(c.Step) > int64((1<<63-1)/int64(c.Frames)) {
			return nil, fmt.Errorf("invalid clip %s", state)
		}
		if c.duration() > 24*time.Hour {
			return nil, fmt.Errorf("clip %s exceeds 24 hours", state)
		}
		clips[state] = c
	}
	if cfg.BeginDropFrame < 1 || cfg.BeginDropFrame >= clips[Poop].Frames {
		return nil, errors.New("BeginDrop marker must follow a pause inside Poop")
	}
	cfg.Clips = clips // caller mutation cannot change live timing
	return &Engine{cfg: cfg, state: Idle}, nil
}

// Advance preserves crossed event nodes even if the timer misses multiple frames.
// No frame index is inherited by a new state.
func (e *Engine) Advance(now time.Duration) error {
	if now < e.now {
		return errors.New("animation clock moved backwards")
	}
	if now > time.Duration(1<<63-1)-24*time.Hour {
		return errors.New("animation clock near duration limit")
	}
	if e.suspended {
		delta := now - e.now
		e.entered += delta
		if e.digestPending {
			e.digestDue += delta
		}
		if e.dropVisible {
			e.dropBorn += delta
		}
		e.now = now
		return nil
	}
	for {
		if e.state == Idle || e.state == Fight {
			if e.digestPending && e.digestDue <= now {
				at := e.digestDue
				if at < e.entered {
					at = e.entered
				}
				e.digestPending = false
				e.enter(ToBack, at)
				continue
			}
			break
		}
		end := e.entered + e.cfg.Clips[e.state].duration()
		if e.state == Poop && !e.markerFired {
			marker := e.entered + time.Duration(e.cfg.BeginDropFrame)*e.cfg.Clips[Poop].Step
			if marker <= now {
				e.markerFired = true
				e.dropVisible = true
				e.dropBorn = marker
				e.events = append(e.events, Event{"BeginDrop", marker, Poop, e.cfg.BeginDropFrame})
			}
		}
		if end > now {
			break
		}
		next := Idle
		switch e.state {
		case ToFight:
			if e.hover && e.queuedAction == 0 {
				next = Fight
			} else {
				next = FromFight
			}
		case FromFight, FromLaugh:
			if e.queuedAction == 1 {
				next = ToLaugh
				e.queuedAction = 0
			} else if e.queuedAction == 2 {
				next = ToEat
				e.queuedAction = 0
			} else if e.hover {
				next = ToFight
			}
		case ToLaugh:
			next = Laugh
		case Laugh:
			next = FromLaugh
		case ToEat:
			next = Eat
		case Eat:
			next = FromEat
		case FromEat:
			e.digestPending = true
			e.digestDue = end + e.cfg.DigestDelay
			if e.hover {
				next = ToFight
			}
		case ToBack:
			next = Poop
		case Poop:
			next = FromBack
		case FromBack:
			next = AfterPoop
		case AfterPoop:
			e.dropVisible = false
			e.markerFired = false
			e.events = append(e.events, Event{"ClearDrop", end, AfterPoop, e.cfg.Clips[AfterPoop].Frames - 1})
			if e.hover {
				next = ToFight
			}
		}
		e.enter(next, end)
	}
	e.now = now
	return nil
}

func (e *Engine) enter(state State, at time.Duration) {
	e.state = state
	e.entered = at
	if state == ToBack {
		e.dropVisible = false
		e.markerFired = false
	}
	e.events = append(e.events, Event{"EnterState", at, state, 0})
}

func (e *Engine) atomic() bool {
	switch e.state {
	case ToEat, Eat, FromEat, ToBack, Poop, FromBack, AfterPoop:
		return true
	}
	return false
}

// Fire reports whether a new action was accepted. Repeated clicks/contact never
// restart laughter. Eating and dropping cannot be overwritten by other actions.
func (e *Engine) Fire(now time.Duration, input Input) (bool, error) {
	if err := e.Advance(now); err != nil {
		return false, err
	}
	if e.suspended {
		return false, nil
	}
	switch input {
	case HoverOn:
		e.hover = true
		if e.state == Idle {
			e.enter(ToFight, now)
			return true, nil
		}
	case HoverOff:
		e.hover = false
		if e.state == Fight {
			e.enter(FromFight, now)
			return true, nil
		}
	case DoubleClick, Contact:
		if e.atomic() || e.queuedAction != 0 || e.state == ToLaugh || e.state == Laugh || e.state == FromLaugh {
			return false, nil
		}
		if e.state == ToFight || e.state == Fight || e.state == FromFight {
			e.queuedAction = 1
			if e.state == Fight {
				e.enter(FromFight, now)
			}
			return true, nil
		}
		e.enter(ToLaugh, now)
		return true, nil
	case FilesStored:
		if e.atomic() || e.digestPending || e.queuedAction != 0 {
			return false, nil
		}
		if e.state == ToFight || e.state == Fight || e.state == FromFight || e.state == ToLaugh || e.state == Laugh || e.state == FromLaugh {
			e.queuedAction = 2
			if e.state == Fight {
				e.enter(FromFight, now)
			}
			return true, nil
		}
		e.enter(ToEat, now)
		return true, nil
	default:
		return false, errors.New("unknown animation input")
	}
	return false, nil
}

func (e *Engine) Snapshot() Snapshot {
	c := e.cfg.Clips[e.state]
	frame := int((e.now - e.entered) / c.Step)
	if e.state == Idle || e.state == Fight {
		frame %= c.Frames
	} else if frame >= c.Frames {
		frame = c.Frames - 1
	}
	s := Snapshot{State: e.state, Frame: frame, StateStarted: e.entered, DropVisible: e.dropVisible, DigestPending: e.digestPending}
	if e.dropVisible {
		s.DropAge = e.now - e.dropBorn
	}
	return s
}

func (e *Engine) DrainEvents() []Event {
	out := append([]Event(nil), e.events...)
	e.events = nil
	return out
}
