package emotion

import (
	"testing"
	"time"
)

func TestManualAngryReplayBypassesAutomaticCooldown(t *testing.T) {
	var now time.Duration
	c := New(func() time.Duration { return now })
	c.Register([]Clip{{Name: "angry", Emotion: "angry", Frames: 2, Step: 100 * time.Millisecond, Priority: 25}})
	a := Analysis{Emotion: "angry", Intensity: .8, Confidence: .95, DurationMS: 500}
	c.Apply("one", a, "deepseek-classifier")
	for now = 0; now < 250*time.Millisecond; now += time.Millisecond {
		c.Advance(now, false)
	}
	if c.Apply("auto-repeat", a, "deepseek-classifier").Accepted {
		t.Fatal("automatic cooldown removed")
	}
	if !c.Apply("manual-repeat", a, "manual-action").Accepted {
		t.Fatal("manual replay blocked by automatic cooldown")
	}
}
