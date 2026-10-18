package emotion

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"testing"
	"time"
)

// Independent of model predictions: fixed decisions go into the real controller
// and imported library. This measures routing/trigger correctness, not semantics.
func TestIndependentAnimationTriggerSet(t *testing.T) {
	l, w := LoadLibrary("../../animations", image.Rect(0, 0, 420, 560))
	if len(w) > 0 {
		t.Fatal(w)
	}
	type row struct {
		ID, Label, Mode, Expected, Observed string
		Correct                             bool
	}
	var rows []row
	correct := 0
	for _, label := range Labels {
		for _, mode := range []string{"normal", "low_confidence", "weak_intensity", "duplicate", "blocked"} {
			var now time.Duration
			c := New(func() time.Duration { return now })
			c.Register(l.Clips())
			a := Analysis{Emotion: label, Intensity: .8, Confidence: .9, DurationMS: 9000}
			expected := label
			if mode == "low_confidence" {
				a.Confidence = .4
				expected = "neutral"
			}
			if mode == "weak_intensity" {
				a.Intensity = .1
				expected = "neutral"
			}
			if label == "thinking" {
				expected = "neutral"
			} // No imported thinking clip; safe idle fallback.
			id := label + "-" + mode
			if mode == "blocked" {
				c.Advance(0, true)
			}
			c.Apply(id, a, "fixture")
			if mode == "duplicate" && c.Apply(id, a, "fixture").Accepted {
				t.Fatal("duplicate accepted")
			}
			if mode == "blocked" {
				if s := c.Advance(0, true); s.Clip != "" || c.ConsumeLaugh(true) {
					t.Fatal("blocked action interrupted")
				}
				now = 500 * time.Millisecond
			}
			observed := "neutral"
			for i := 0; i < 250; i++ {
				s := c.Advance(now, false)
				if c.ConsumeLaugh(false) {
					observed = "happy"
					break
				}
				if s.Clip != "" {
					if m, ok := l.Manifest(s.Clip); ok && m.Emotion == label && s.Clip == c.mapping[label] {
						if _, e := l.Frame(s.Clip, s.Frame); e != nil {
							t.Fatal("actual mapped frame unavailable", e)
						}
						observed = label
						break
					}
				}
				now += 20 * time.Millisecond
			}
			ok := observed == expected
			if ok {
				correct++
			}
			rows = append(rows, row{id, label, mode, expected, observed, ok})
			if !ok {
				t.Errorf("%s expected=%s actual=%s", id, expected, observed)
			}
		}
	}
	report := map[string]any{"measure": "controller trigger/routing correctness with actual imported frame decoding; Win32 presentation measured separately", "total": len(rows), "correct": correct, "accuracy": float64(correct) / float64(len(rows)), "rows": rows}
	if path := os.Getenv("NAIWA_ANIMATION_REPORT"); path != "" {
		b, _ := json.MarshalIndent(report, "", "  ")
		if e := os.WriteFile(path, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	t.Log(fmt.Sprintf("Independent animation trigger set: %d/%d", correct, len(rows)))
}
