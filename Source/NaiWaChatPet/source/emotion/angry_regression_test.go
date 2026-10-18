package emotion

import (
	"image"
	"testing"
	"time"
)

func TestStructuredAngryPlaysEntireImportedSequenceWithoutHover(t *testing.T) {
	l, warnings := LoadLibrary("../../animations", image.Rect(0, 0, 420, 560))
	if len(warnings) > 0 {
		t.Fatal(warnings)
	}
	a, e := ParseAnalysis(`{"emotion":"angry","intensity":0.8,"confidence":0.95,"duration_ms":7000}`)
	if e != nil {
		t.Fatal(e)
	}
	var now time.Duration
	c := New(func() time.Duration { return now })
	c.Register(l.Clips())
	if o := c.Apply("angry-message", a, "deepseek-classifier"); !o.Accepted {
		t.Fatal(o)
	}
	seen := map[string]map[int]bool{}
	for now = 0; now < 10*time.Second; now += time.Millisecond {
		s := c.Advance(now, false)
		if s.Clip == "" {
			break
		}
		if !c.ProtectsHover() {
			t.Fatal("hover can preempt running angry clip")
		}
		if seen[s.Clip] == nil {
			seen[s.Clip] = map[int]bool{}
		}
		if !seen[s.Clip][s.Frame] {
			if _, err := l.Frame(s.Clip, s.Frame); err != nil {
				t.Fatal(err)
			}
		}
		seen[s.Clip][s.Frame] = true
	}
	count := 0
	for _, name := range []string{"angry_in", "angry", "angry_out"} {
		m, ok := l.Manifest(name)
		if !ok || len(seen[name]) != m.FrameCount {
			t.Fatalf("%s: played %d/%d frames", name, len(seen[name]), m.FrameCount)
		}
		count += m.FrameCount
	}
	if c.ProtectsHover() || c.Scheduling().Current != "neutral" {
		t.Fatal("angry failed to return to idle")
	}
	t.Logf("Angry JSON -> controller -> angry_in/angry/angry_out -> idle: %d frames complete", count)
}
