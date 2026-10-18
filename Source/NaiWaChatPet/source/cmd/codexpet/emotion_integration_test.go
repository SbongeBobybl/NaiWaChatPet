package main

import (
	"codexpet/reconstruction/animation"
	"codexpet/reconstruction/emotion"
	"testing"
	"time"
)

func TestStructuredHappyRunsEveryOriginalLaughFrameAndReturnsIdle(t *testing.T) {
	assets, err := loadAssets(bundledAssets, "runtime.json")
	if err != nil {
		t.Fatal(err)
	}
	var now time.Duration
	c := emotion.New(func() time.Duration { return now })
	c.Apply("model-1", emotion.Analysis{Emotion: "happy", Intensity: .8, Confidence: .9, DurationMS: 8000}, "model")
	c.Advance(0, false)
	if !c.ConsumeLaugh(false) {
		t.Fatal("model happy did not trigger")
	}
	queue := actionQueue{laugh: true}
	if ok, e := queue.service(assets.Engine, 0); e != nil || !ok {
		t.Fatal("queue rejected", e)
	}
	seen := map[animation.State]map[int]bool{}
	for now = 0; now < 30*time.Second; now += time.Millisecond {
		if e := assets.Engine.Advance(now); e != nil {
			t.Fatal(e)
		}
		s := assets.Engine.Snapshot()
		if s.State == animation.Idle {
			break
		}
		if seen[s.State] == nil {
			seen[s.State] = map[int]bool{}
		}
		seen[s.State][s.Frame] = true
	}
	for _, state := range []animation.State{animation.ToLaugh, animation.Laugh, animation.FromLaugh} {
		if len(seen[state]) != assets.Counts[string(state)] {
			t.Fatalf("%s: %d/%d frames", state, len(seen[state]), assets.Counts[string(state)])
		}
	}
	if assets.Engine.Snapshot().State != animation.Idle || queue.laugh || c.ConsumeLaugh(false) {
		t.Fatal("did not finish cleanly")
	}
	t.Log("Original full toLaugh -> Laugh -> fromLaugh completed every frame and returned to idle")
}
