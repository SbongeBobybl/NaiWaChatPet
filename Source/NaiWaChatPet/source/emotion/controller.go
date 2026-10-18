package emotion

import (
	"math"
	"strings"
	"sync"
	"time"
)

var Initial = []string{"neutral", "happy", "laugh", "curious", "thinking", "sad", "comforting", "sleepy", "shy", "surprised", "angry", "confused"}

type Clip struct {
	Name, Emotion, TransitionIn, TransitionOut string
	Frames                                     int
	Step                                       time.Duration
	Loop                                       bool
	Priority                                   int
}
type Snapshot struct {
	Emotion, Source, Clip string
	Intensity             float64
	Frame                 int
	Fallback, Blocked     bool
}
type request struct {
	smooth                bool
	id                    string
	confidence            float64
	activated             time.Duration
	emotion, source, clip string
	intensity             float64
	priority              int
	expires               time.Duration
}
type Controller struct {
	seen             map[string]bool
	seenOrder        []string
	idleRequested    bool
	laughConsumed    bool
	mu               sync.Mutex
	clock            func() time.Duration
	clips            map[string]Clip
	mapping          map[string]string
	current, pending *request
	last             map[string]time.Duration
	phase, playing   string
	started, now     time.Duration
	blocked          bool
}

func New(clock func() time.Duration) *Controller {
	return &Controller{clock: clock, clips: map[string]Clip{}, mapping: map[string]string{}, last: map[string]time.Duration{}}
}
func (c *Controller) Register(clips []Clip) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, v := range clips {
		if v.Frames > 0 && v.Step > 0 {
			v.Priority = max(1, min(50, v.Priority))
			c.clips[v.Name] = v
			c.mapping[v.Emotion] = v.Name
		}
	}
}
func (c *Controller) SetEmotion(emotion string, intensity float64, duration time.Duration, source string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock()
	known := c.mapping[emotion] != ""
	for _, name := range Initial {
		known = known || emotion == name
	}
	if !known {
		return false
	}
	if math.IsNaN(intensity) || math.IsInf(intensity, 0) {
		return false
	}
	intensity = math.Max(0, math.Min(1, intensity))
	duration = max(time.Second, min(30*time.Second, duration))
	if emotion == "neutral" {
		c.reset()
		return true
	}
	cooldown := 6 * time.Second
	if emotion == "happy" || emotion == "laugh" {
		cooldown = 20 * time.Second
	}
	if at, ok := c.last[emotion]; ok && now-at < cooldown {
		return false
	}
	r := &request{emotion: emotion, source: source, intensity: intensity, priority: 20, expires: now + duration, clip: c.mapping[emotion]}
	if r.clip != "" {
		r.priority = c.clips[r.clip].Priority
	}
	if c.blocked || c.current != nil && c.current.priority > r.priority {
		c.pending = r
		return true
	}
	c.activate(r, now)
	return true
}
func (c *Controller) QueueAnimation(name string, priority int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.clips[name]
	if !ok {
		return false
	}
	now := c.clock()
	r := &request{emotion: v.Emotion, source: "queued", clip: name, intensity: 1, priority: max(1, min(50, priority)), expires: now + 30*time.Second}
	if c.blocked || c.current != nil && c.current.priority > r.priority {
		c.pending = r
	} else {
		c.activate(r, now)
	}
	return true
}
func (c *Controller) activate(r *request, now time.Duration) {
	c.idleRequested = false
	r.activated = now
	c.laughConsumed = false
	c.current = r
	c.pending = nil
	c.started = now
	c.phase = "main"
	c.playing = r.clip
	c.last[r.emotion] = now
	if v, ok := c.clips[r.clip]; ok && v.TransitionIn != "" {
		c.playing = v.TransitionIn
		c.phase = "in"
	}
}
func (c *Controller) finish() { c.current = nil; c.phase = ""; c.playing = "" }
func (c *Controller) reset()  { c.current = nil; c.pending = nil; c.phase = ""; c.playing = "" }
func (c *Controller) ReturnToIdle() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending = nil
	if c.phase == "out" {
		return
	}
	if c.current != nil && c.current.smooth {
		c.idleRequested = true
		return
	}
	if c.current != nil {
		v := c.clips[c.current.clip]
		if v.TransitionOut != "" {
			c.playing = v.TransitionOut
			c.phase = "out"
			c.started = c.clock()
			c.pending = nil
			return
		}
	}
	c.reset()
}
func (c *Controller) InterruptAnimation(reason string) { c.mu.Lock(); defer c.mu.Unlock(); c.reset() }
func (c *Controller) Advance(now time.Duration, blocked bool) Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	if now < c.now {
		now = c.now
	}
	c.now = now
	wasBlocked := c.blocked
	c.blocked = blocked
	if c.pending != nil && now >= c.pending.expires {
		c.pending = nil
	}
	if !blocked && c.pending != nil && (c.current == nil || !c.pending.smooth && c.pending.priority >= c.current.priority) {
		c.activate(c.pending, now)
	}
	if wasBlocked && !blocked && c.current != nil {
		c.started = now
		c.playing = c.current.clip
		c.phase = "main"
		if v := c.clips[c.current.clip]; v.TransitionIn != "" {
			c.playing = v.TransitionIn
			c.phase = "in"
		}
	}
	if c.current == nil {
		return Snapshot{Emotion: "neutral", Fallback: true, Blocked: blocked}
	}
	main := c.clips[c.current.clip]
	leave := now >= c.current.expires || c.idleRequested || c.pending != nil && c.pending.priority >= c.current.priority
	canLeave := !c.current.smooth || now-c.current.activated >= 1200*time.Millisecond && (c.current.clip == "" || main.Loop && c.phase != "in")
	if leave && canLeave && c.phase != "out" {
		v := c.clips[c.current.clip]
		if v.TransitionOut != "" {
			c.playing = v.TransitionOut
			c.phase = "out"
			c.started = now
		} else {
			c.finish()
			return Snapshot{Emotion: "neutral", Fallback: true, Blocked: blocked}
		}
	}
	clip, ok := c.clips[c.playing]
	frame := 0
	if ok && !blocked {
		// An expression started after the time this tick was taken, such as
		// one begun once a dialog closed, is still on its first frame.
		frame = int(max(0, now-c.started) / clip.Step)
		if frame >= clip.Frames {
			if c.phase == "in" {
				c.playing = c.current.clip
				c.phase = "main"
				c.started = now
				clip = c.clips[c.playing]
				frame = 0
			} else if c.phase == "main" && !clip.Loop && clip.TransitionOut != "" {
				c.playing = clip.TransitionOut
				c.phase = "out"
				c.started = now
				clip = c.clips[c.playing]
				frame = 0
			} else if c.phase == "out" || !clip.Loop {
				c.finish()
				return Snapshot{Emotion: "neutral", Fallback: true}
			} else {
				frame %= clip.Frames
			}
		}
	}
	s := Snapshot{Emotion: c.current.emotion, Source: c.current.source, Intensity: c.current.intensity, Clip: c.playing, Frame: frame, Fallback: !ok, Blocked: blocked}
	if c.current.smooth {
		span := c.current.expires - c.current.activated
		if span > 0 {
			progress := math.Min(1, math.Max(0, float64(now-c.current.activated)/float64(span)))
			s.Intensity *= 1 - .7*progress
		}
	}
	if blocked || !ok {
		s.Clip = ""
	}
	return s
}

type Cue struct{ Stage, User, Reply, Previous, Context string }
type Decision struct {
	Emotion    string
	Confidence float64
	Duration   time.Duration
}

func LegacyAnalyze(e Cue) Decision {
	neutral := Decision{"neutral", 0, 0}
	if e.Stage == "streaming" {
		return neutral
	}
	if e.Stage == "error" || e.Stage == "cancelled" {
		return neutral
	}
	user := strings.TrimSpace(e.User)
	// First-person sharing plus an explicit difficulty cue; creative examples do not qualify.
	personal := strings.Contains(user, "我") && !strings.Contains(user, "写故事") && !strings.Contains(user, "举例")
	difficulty := strings.Contains(user, "很难过") || strings.Contains(user, "压力很大") || strings.Contains(user, "不知道怎么办") || strings.Contains(user, "我失败了")
	if personal && difficulty {
		return Decision{"comforting", .82, 15 * time.Second}
	}
	if e.Stage == "waiting" {
		if len([]rune(user)) > 80 || strings.Contains(user, "代码") || strings.Contains(user, "证明") || strings.Contains(user, "详细") {
			return Decision{"thinking", .92, 30 * time.Second}
		}
		if (strings.Contains(user, "如何") || strings.Contains(user, "为什么")) && (strings.Contains(user, "？") || strings.Contains(user, "?")) {
			return Decision{"curious", .72, 12 * time.Second}
		}
	}
	if e.Stage == "completed" && len([]rune(e.Reply)) < 150 && (strings.Contains(e.Reply, "太好了") || strings.Contains(e.Reply, "很开心")) {
		return Decision{"happy", .78, 10 * time.Second}
	}
	return neutral
}
