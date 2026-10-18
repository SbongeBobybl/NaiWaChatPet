package emotion

import (
	"testing"
	"time"
)

func TestEmotionProtectsHoverThroughoutQueuedEntryMainExit(t *testing.T) {
	now := time.Duration(0)
	c := New(func() time.Duration { return now })
	c.Register([]Clip{{Name: "in", Emotion: "_in", Frames: 2, Step: 100 * time.Millisecond}, {Name: "sad", Emotion: "sad", Frames: 4, Step: 100 * time.Millisecond, TransitionIn: "in", TransitionOut: "out"}, {Name: "out", Emotion: "_out", Frames: 2, Step: 100 * time.Millisecond}})
	c.Advance(now, true)
	c.Apply("message", Analysis{Emotion: "sad", Intensity: .8, Confidence: .9, DurationMS: 3000}, "model")
	if !c.ProtectsHover() {
		t.Fatal("queued emotion does not protect hover")
	}
	for _, at := range []time.Duration{0, 200 * time.Millisecond, 600 * time.Millisecond} {
		now = at
		s := c.Advance(now, false)
		if !c.ProtectsHover() || s.Clip == "" {
			t.Fatalf("emotion interrupted at %v: %+v", at, s)
		}
	}
	now = 800 * time.Millisecond
	c.Advance(now, false)
	if c.ProtectsHover() {
		t.Fatal("hover never released after complete animation")
	}
}

func TestAutomaticEmotionVocabularyRejectsLegacyLabels(t *testing.T) {
	for _, label := range []string{"thinking", "laugh", "evil"} {
		if _, e := ParseAnalysis(`{"emotion":"` + label + `","intensity":0.8,"confidence":0.99,"duration_ms":5000}`); e == nil {
			t.Fatalf("automatic classifier accepts %s", label)
		}
	}
}
