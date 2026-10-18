package emotion

import (
	"fmt"
	"testing"
	"time"
)

func TestStructuredProtocolValidation(t *testing.T) {
	for _, label := range Labels {
		if _, err := ParseAnalysis(fmt.Sprintf(`{"emotion":%q,"intensity":0.6,"confidence":0.85,"duration_ms":5000}`, label)); err != nil {
			t.Fatal(label, err)
		}
	}
	bad := []string{
		`{}`, `null`, `[]`, `{"emotion":"evil","intensity":1,"confidence":1,"duration_ms":2000}`,
		`{"emotion":"happy","intensity":2,"confidence":1,"duration_ms":2000}`,
		`{"emotion":"happy","intensity":1,"confidence":-1,"duration_ms":2000}`,
		`{"emotion":"happy","intensity":1,"confidence":1,"duration_ms":2.5}`,
		`{"emotion":"happy","intensity":1,"confidence":1,"duration_ms":30001}`,
		`{"emotion":"happy","intensity":1,"confidence":1,"duration_ms":0}`,
		`{"emotion":"happy","intensity":1,"confidence":null,"duration_ms":2000}`,
		`{"emotion":"happy","intensity":1,"confidence":1,"duration_ms":2000,"x":1}`,
		`{"emotion":"happy","intensity":1,"confidence":1,"duration_ms":2000,"emotion":"sad"}`,
		"```json\n{\"emotion\":\"happy\"}\n```",
		`{"emotion":"neutral","intensity":0,"confidence":1,"duration_ms":0} {}`,
	}
	for _, text := range bad {
		if _, e := ParseAnalysis(text); e == nil {
			t.Fatalf("accepted invalid protocol %s", text)
		}
	}
}

func smoothController(now *time.Duration) *Controller {
	c := New(func() time.Duration { return *now })
	c.Register([]Clip{{Name: "entry", Emotion: "_entry", Frames: 2, Step: 100 * time.Millisecond}, {Name: "exit", Emotion: "_exit", Frames: 2, Step: 100 * time.Millisecond}, {Name: "sad", Emotion: "sad", Frames: 30, Step: 100 * time.Millisecond, TransitionIn: "entry", TransitionOut: "exit", Priority: 20}, {Name: "angry", Emotion: "angry", Frames: 20, Step: 100 * time.Millisecond, Priority: 30}})
	return c
}
func TestControllerInertiaDecayNeutralAndDuplicate(t *testing.T) {
	var now time.Duration
	c := smoothController(&now)
	a := Analysis{Emotion: "sad", Intensity: .8, Confidence: .92, DurationMS: 5000}
	if !c.Apply("one", a, "model").Accepted {
		t.Fatal("rejected")
	}
	c.Advance(now, false)
	now = 300 * time.Millisecond
	s := c.Advance(now, false)
	if s.Clip != "sad" {
		t.Fatal(s)
	}
	now = 500 * time.Millisecond
	if c.Apply("two", a, "model").Reason != "merged_without_restart" {
		t.Fatal("merge")
	}
	s = c.Advance(now, false)
	if s.Clip != "sad" || s.Frame != 2 || s.Intensity >= a.Intensity {
		t.Fatal("restart/decay", s)
	}
	if c.Apply("two", a, "model").Accepted {
		t.Fatal("duplicate")
	}
	c.Apply("three", Analysis{Emotion: "angry", Intensity: 1, Confidence: .9, DurationMS: 5000}, "model")
	if c.Advance(now, false).Clip != "sad" {
		t.Fatal("hard switch")
	}
	now = 3400 * time.Millisecond
	if c.Advance(now, false).Clip != "exit" {
		t.Fatal("main not completed")
	}
	now = 3700 * time.Millisecond
	c.Advance(now, false)
	now = 3800 * time.Millisecond
	if c.Advance(now, false).Emotion != "angry" {
		t.Fatal("pending lost")
	}
	c.Apply("four", Neutral(), "model")
	c.ReturnToIdle()
	c.ReturnToIdle()
	now = 6 * time.Second
	c.Advance(now, false)
	now = 7 * time.Second
	if c.Advance(now, false).Emotion != "neutral" {
		t.Fatal("idle recovery")
	}
}

func TestStructuredHappyThresholdAndCooldown(t *testing.T) {
	var now time.Duration
	c := New(func() time.Duration { return now })
	a := Analysis{Emotion: "happy", Intensity: .4, Confidence: .99, DurationMS: 8000}
	c.Apply("weak", a, "model")
	c.Advance(now, false)
	if c.ConsumeLaugh(false) {
		t.Fatal("confidence used as intensity")
	}
	a.Intensity = .8
	c.Apply("strong", a, "model")
	c.Advance(now, false)
	if !c.ConsumeLaugh(false) || c.ConsumeLaugh(false) {
		t.Fatal("full laugh trigger must happen once")
	}
	now = time.Second
	c.Apply("same", a, "model")
	if c.ConsumeLaugh(false) {
		t.Fatal("merged message retrigger")
	}
	now = 10 * time.Second
	c.Advance(now, false)
	if c.Apply("cooldown", a, "model").Reason != "cooldown" {
		t.Fatal("cooldown")
	}
}
