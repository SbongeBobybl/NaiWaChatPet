package emotion

import (
	"testing"
	"time"
)

func TestAtomicPriorityCooldownExpiryAndRecovery(t *testing.T) {
	var now time.Duration
	c := New(func() time.Duration { return now })
	c.Register([]Clip{{Name: "think", Emotion: "thinking", Frames: 4, Step: time.Second, Loop: true, Priority: 20}, {Name: "smile", Emotion: "happy", Frames: 2, Step: time.Second, Loop: false, Priority: 30}})
	c.Advance(now, true)
	c.SetEmotion("thinking", 1, 10*time.Second, "request")
	if s := c.Advance(time.Second, true); s.Clip != "" {
		t.Fatal("ordinary emotion interrupted atomic action")
	}
	now = 2 * time.Second
	if s := c.Advance(now, false); s.Clip != "think" || s.Frame != 0 {
		t.Fatal(s)
	}
	if c.SetEmotion("thinking", 1, time.Second, "duplicate") {
		t.Fatal("cooldown ignored")
	}
	now = 3 * time.Second
	c.QueueAnimation("smile", 30)
	if s := c.Advance(now, false); s.Clip != "smile" {
		t.Fatal(s)
	}
	now = 6 * time.Second
	if s := c.Advance(now, false); s.Emotion != "neutral" {
		t.Fatal("one-shot did not recover", s)
	}
	c.SetEmotion("sad", 1, time.Hour, "local")
	now += 31 * time.Second
	if c.Advance(now, false).Emotion != "neutral" {
		t.Fatal("duration cap")
	}
}
func TestTransitionsAndBlockedResumeStartAtZero(t *testing.T) {
	var now time.Duration
	c := New(func() time.Duration { return now })
	c.Register([]Clip{{Name: "in", Emotion: "bridge", Frames: 2, Step: time.Second}, {Name: "out", Emotion: "bridge2", Frames: 2, Step: time.Second}, {Name: "main", Emotion: "happy", TransitionIn: "in", TransitionOut: "out", Frames: 4, Step: time.Second, Loop: true}})
	c.SetEmotion("happy", 1, 10*time.Second, "local")
	if c.Advance(now, false).Clip != "in" {
		t.Fatal("missing entry")
	}
	now = 3 * time.Second
	if c.Advance(now, false).Clip != "main" {
		t.Fatal("entry stuck")
	}
	c.Advance(4*time.Second, true)
	now = 5 * time.Second
	s := c.Advance(now, false)
	if s.Clip != "in" || s.Frame != 0 {
		t.Fatal("resume jumped", s)
	}
	c.ReturnToIdle()
	if c.Advance(now, false).Clip != "out" {
		t.Fatal("missing exit")
	}
	now = 8 * time.Second
	if c.Advance(now, false).Emotion != "neutral" {
		t.Fatal("exit stuck")
	}
}
func TestSemanticMappingIsConservative(t *testing.T) {
	if Analyze(Cue{Stage: "waiting", User: "写故事，主角很难过"}).Emotion != "neutral" {
		t.Fatal("keyword-only inference")
	}
	if Analyze(Cue{Stage: "waiting", User: "我最近压力很大，不知道怎么办"}).Emotion != "comforting" {
		t.Fatal("explicit difficulty ignored")
	}
	if Analyze(Cue{Stage: "waiting", User: "请详细解释这段代码"}).Emotion != "thinking" {
		t.Fatal("complex request ignored")
	}
	if Analyze(Cue{Stage: "streaming", Reply: "太好了"}).Emotion != "neutral" {
		t.Fatal("stream token changes emotion")
	}
}
