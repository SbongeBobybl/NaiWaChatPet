package emotion

import "fmt"

// Cause distinguishes automatic model choices from manual and player-driven plays.
// RequestEmotion is the automatic path. Manual forced plays and explicit
// player emotion triggers use CauseManual or CausePlayer; those beat auto
// emotion, and still wait out jump/attack.
type Cause int

const (
	CauseAuto Cause = iota
	CauseManual
	CausePlayer
)

// EmotionState is the overlay snapshot exported to locomotion and UI.
type EmotionState struct {
	EmotionID               string   `json:"emotion_id"`
	EmotionPhase            Phase    `json:"emotion_phase"`
	EmotionPriority         Priority `json:"emotion_priority"`
	EmotionPlaying          bool     `json:"emotion_playing"`
	EmotionBlocksHoverPunch bool     `json:"emotion_blocks_hover_punch"`
}

// Decision is the result of RequestEmotion.
type Decision struct {
	Accepted bool
	Queued   bool
	Reason   string
}

// Durations control in/loop/out. Tests and the legacy player can shorten them.
type Durations struct {
	InMS   int
	LoopMS int
	OutMS  int
}

// DefaultDurations match a short reaction plus the existing in/out clips.
func DefaultDurations() Durations {
	return Durations{InMS: 450, LoopMS: 4000, OutMS: 500}
}

type queued struct {
	id       string
	priority Priority
	cause    Cause
}

// Scheduler is the emotion entry point. It does not move the body.
//
// Startup order the caller must keep:
//  1. animation resources are loaded
//  2. locomotion is ready and grounded
//  3. Start subscribes
//  4. only then may the LLM or companion call RequestEmotion
type Scheduler struct {
	dur       Durations
	loc       LocomotionSource
	unsub     func()
	ready     bool
	loco      LocomotionState
	state     EmotionState
	phaseMS   int
	cause     Cause
	manualQ   *queued
	autoQ     *queued
	listeners []func(EmotionState)
}

// NewScheduler builds an idle overlay. Call Start before accepting LLM input.
func NewScheduler(dur Durations) *Scheduler {
	if dur.InMS <= 0 || dur.LoopMS <= 0 || dur.OutMS <= 0 {
		dur = DefaultDurations()
	}
	return &Scheduler{
		dur: dur,
		state: EmotionState{
			EmotionID:       NeutralID,
			EmotionPhase:    PhaseIdle,
			EmotionPriority: PriorityLow,
		},
	}
}

// Start subscribes to locomotion. Emotions stay rejected until the first
// grounded, non-busy snapshot with CanPlayEmotion.
func (s *Scheduler) Start(loc LocomotionSource) error {
	if loc == nil {
		return fmt.Errorf("emotion: locomotion source is nil")
	}
	if s.unsub != nil {
		s.unsub()
		s.unsub = nil
	}
	s.loc = loc
	s.unsub = loc.OnLocomotionChange(s.onLocomotion)
	s.onLocomotion(loc.GetLocomotionState())
	return nil
}

// Stop detaches the locomotion subscription.
func (s *Scheduler) Stop() {
	if s.unsub != nil {
		s.unsub()
		s.unsub = nil
	}
	s.loc = nil
}

// GetEmotionState returns the overlay snapshot. Locomotion should read this
// instead of sharing an animator clip slot.
func (s *Scheduler) GetEmotionState() EmotionState {
	return s.state
}

// OnEmotionChange registers a listener. The callback must not re-enter the
// scheduler. The returned function removes that listener.
func (s *Scheduler) OnEmotionChange(fn func(EmotionState)) (unsubscribe func()) {
	if fn == nil {
		return func() {}
	}
	s.listeners = append(s.listeners, fn)
	idx := len(s.listeners) - 1
	return func() {
		if idx >= 0 && idx < len(s.listeners) && fmt.Sprintf("%p", s.listeners[idx]) == fmt.Sprintf("%p", fn) {
			s.listeners[idx] = nil
		}
	}
}

// RequestEmotion asks to play an automatic emotion. It queues while
// jump/attack is busy and rejects until Start has seen a ready locomotion
// snapshot. Unknown ids are rejected. neutral cancels the overlay.
func (s *Scheduler) RequestEmotion(emotionID string, priority Priority) Decision {
	return s.RequestEmotionFrom(emotionID, priority, CauseAuto)
}

// RequestEmotionFrom is the same entry with an explicit cause.
// CauseManual and CausePlayer beat automatic emotions of any rank, and
// still queue behind jump/attack.
func (s *Scheduler) RequestEmotionFrom(emotionID string, priority Priority, cause Cause) Decision {
	if !s.ready {
		return Decision{Reason: "not_ready"}
	}
	if emotionID == NeutralID {
		s.cancel(cause)
		return Decision{Accepted: true, Reason: "return_to_idle"}
	}
	if _, ok := ByID(emotionID); !ok {
		return Decision{Reason: "unknown_emotion"}
	}
	if !validPriority(priority) {
		return Decision{Reason: "bad_priority"}
	}
	req := queued{id: emotionID, priority: priority, cause: cause}
	if s.loco.Busy() || !s.loco.OverlayAllowed() {
		s.enqueue(req)
		return Decision{Queued: true, Reason: "busy_locomotion"}
	}
	if s.state.EmotionPlaying && s.state.EmotionID == emotionID && s.causeRank() >= causeRank(cause) {
		return Decision{Accepted: true, Reason: "already_playing"}
	}
	if s.state.EmotionPlaying && !s.canInterrupt(req) {
		return Decision{Reason: "lower_priority"}
	}
	s.begin(req)
	return Decision{Accepted: true, Reason: "play"}
}

// InterruptsLocomotionIdle reports that a high-priority overlay is playing
// while locomotion is not in jump/attack. The locomotion owner may drop
// walk/run idle variants. Jump and attack are never interrupted.
func (s *Scheduler) InterruptsLocomotionIdle() bool {
	return s.state.EmotionPlaying && s.state.EmotionPriority == PriorityHigh && !s.loco.Busy()
}

// Tick advances in → loop → out → idle. dtMS is milliseconds.
// The legacy frame player keeps its own clock; this clock is the contract
// the locomotion teammate can drive or mirror.
func (s *Scheduler) Tick(dtMS int) {
	if dtMS <= 0 || !s.state.EmotionPlaying {
		return
	}
	s.phaseMS += dtMS
	switch s.state.EmotionPhase {
	case PhaseIn:
		if s.phaseMS >= s.dur.InMS {
			s.phaseMS = 0
			s.setPhase(PhaseLoop)
		}
	case PhaseLoop:
		if s.phaseMS >= s.dur.LoopMS {
			s.phaseMS = 0
			s.setPhase(PhaseOut)
		}
	case PhaseOut:
		if s.phaseMS >= s.dur.OutMS {
			s.phaseMS = 0
			s.finish()
		}
	}
}

func (s *Scheduler) onLocomotion(st LocomotionState) {
	wasBusy := s.loco.Busy()
	s.loco = st
	if st.Grounded && st.CanPlayEmotion && !st.Busy() {
		s.ready = true
	}
	if st.Busy() {
		if s.state.EmotionPlaying {
			// Jump/attack clears the overlay and replays emotion_in later.
			s.enqueue(queued{id: s.state.EmotionID, priority: s.state.EmotionPriority, cause: s.cause})
			s.forceIdle()
		}
		return
	}
	if wasBusy || !s.state.EmotionPlaying {
		s.drain()
	}
}

func (s *Scheduler) enqueue(q queued) {
	if q.cause == CauseAuto {
		if s.autoQ == nil || rank(q.priority) >= rank(s.autoQ.priority) {
			s.autoQ = &q
		}
		return
	}
	// Manual and player input replace any automatic waiter and any weaker manual waiter.
	s.autoQ = nil
	if s.manualQ == nil || causeRank(q.cause) >= causeRank(s.manualQ.cause) || rank(q.priority) >= rank(s.manualQ.priority) {
		s.manualQ = &q
	}
}

func (s *Scheduler) drain() {
	if s.state.EmotionPlaying || s.loco.Busy() || !s.loco.OverlayAllowed() {
		return
	}
	var next *queued
	if s.manualQ != nil {
		next = s.manualQ
		s.manualQ = nil
	} else if s.autoQ != nil {
		next = s.autoQ
		s.autoQ = nil
	}
	if next == nil {
		return
	}
	s.begin(*next)
}

func (s *Scheduler) begin(q queued) {
	s.cause = q.cause
	s.phaseMS = 0
	s.state = EmotionState{
		EmotionID:               q.id,
		EmotionPhase:            PhaseIn,
		EmotionPriority:         q.priority,
		EmotionPlaying:          true,
		EmotionBlocksHoverPunch: true,
	}
	s.emit()
}

func (s *Scheduler) setPhase(p Phase) {
	s.state.EmotionPhase = p
	s.state.EmotionPlaying = p != PhaseIdle
	s.state.EmotionBlocksHoverPunch = p != PhaseIdle
	s.emit()
}

func (s *Scheduler) finish() {
	s.forceIdle()
	s.drain()
}

func (s *Scheduler) forceIdle() {
	if s.state.EmotionPhase == PhaseIdle && !s.state.EmotionPlaying && s.state.EmotionID == NeutralID {
		return
	}
	s.phaseMS = 0
	s.cause = CauseAuto
	s.state = EmotionState{
		EmotionID:       NeutralID,
		EmotionPhase:    PhaseIdle,
		EmotionPriority: PriorityLow,
	}
	s.emit()
}

func (s *Scheduler) cancel(cause Cause) {
	s.autoQ = nil
	if cause != CauseAuto {
		s.manualQ = nil
	}
	if s.loco.Busy() {
		s.forceIdle()
		return
	}
	if s.state.EmotionPlaying && s.state.EmotionPhase != PhaseOut {
		s.phaseMS = 0
		s.setPhase(PhaseOut)
		return
	}
	s.forceIdle()
}

func (s *Scheduler) canInterrupt(req queued) bool {
	if !s.state.EmotionPlaying {
		return true
	}
	// Player input and manual forced plays beat auto emotion.
	if req.cause != CauseAuto && s.cause == CauseAuto {
		return true
	}
	if req.cause == CauseAuto && s.cause != CauseAuto {
		return false
	}
	return rank(req.priority) > rank(s.state.EmotionPriority)
}

func (s *Scheduler) causeRank() int {
	return causeRank(s.cause)
}

func causeRank(c Cause) int {
	switch c {
	case CausePlayer:
		return 3
	case CauseManual:
		return 2
	default:
		return 1
	}
}

func (s *Scheduler) emit() {
	snap := s.state
	for _, fn := range s.listeners {
		if fn != nil {
			fn(snap)
		}
	}
}
