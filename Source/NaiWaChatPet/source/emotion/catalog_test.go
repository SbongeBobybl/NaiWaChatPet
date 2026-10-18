package emotion

import (
	"testing"
	"time"
)

func TestCatalogIsConsistent(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range Catalog {
		if seen[e.ID] || !safeName.MatchString(e.ID) {
			t.Fatalf("expression %q is duplicated or not a safe folder name", e.ID)
		}
		seen[e.ID] = true
		if e.Label == "" || e.Priority < 1 || e.Priority > 50 {
			t.Fatalf("%s: needs a label and a priority within 1..50", e.ID)
		}
		if e.Auto && (e.Rule == "" || e.Tone == "") {
			t.Fatalf("%s: the model may choose it, so it needs a rule and a tone", e.ID)
		}
		if e.Posed != (e.Gesture != "") {
			t.Fatalf("%s: a gesture belongs to a posed expression and every posed expression needs one", e.ID)
		}
	}
	if len(Labels) < 2 || Labels[0] != "neutral" {
		t.Fatalf("classifier vocabulary = %v", Labels)
	}
	for _, label := range Labels[1:] {
		if e, ok := Find(label); !ok || !e.Auto {
			t.Fatalf("classifier offers %s, which the catalog does not mark automatic", label)
		}
	}
}

func TestOnlyAutomaticExpressionsPassTheClassifierProtocol(t *testing.T) {
	for _, e := range Catalog {
		_, err := ParseAnalysis(`{"emotion":"` + e.ID + `","intensity":0.8,"confidence":0.9,"duration_ms":5000}`)
		if e.Auto != (err == nil) {
			t.Fatalf("%s: automatic=%t but protocol error=%v", e.ID, e.Auto, err)
		}
		if !(Analysis{Emotion: e.ID, Intensity: 1, Confidence: 1, DurationMS: 5000}).Valid() {
			t.Fatalf("%s cannot be played on request", e.ID)
		}
	}
}

func TestMildHappinessSmilesAndStrongHappinessLaughs(t *testing.T) {
	var now time.Duration
	c := New(func() time.Duration { return now })
	c.Register([]Clip{{Name: "smile", Emotion: "smile", Frames: 1, Step: time.Second, Loop: true, Priority: 20}})
	if o := c.Apply("mild", Analysis{Emotion: "happy", Intensity: .4, Confidence: .9, DurationMS: 4000}, "model"); !o.Accepted || o.Current != "smile" {
		t.Fatalf("mild happiness scheduled %+v, want a smile", o)
	}
	if c.ConsumeLaugh(false) {
		t.Fatal("a smile triggered the full laugh")
	}
	c.InterruptAnimation("test")
	if o := c.Apply("strong", Analysis{Emotion: "happy", Intensity: .9, Confidence: .9, DurationMS: 4000}, "model"); o.Current != "happy" {
		t.Fatalf("strong happiness scheduled %+v, want the laugh", o)
	}
	if !c.ConsumeLaugh(false) {
		t.Fatal("strong happiness did not request the laugh")
	}

	// Without a smile clip the old behaviour stands: mild happiness stays idle.
	plain := New(func() time.Duration { return now })
	if o := plain.Apply("mild", Analysis{Emotion: "happy", Intensity: .4, Confidence: .9, DurationMS: 4000}, "model"); o.Reason != "return_to_idle" {
		t.Fatalf("without a smile clip mild happiness scheduled %+v", o)
	}
}

func TestAnimationStartedAfterTheTickTimeStillShowsItsFirstFrame(t *testing.T) {
	// The host advances the controller with the time its tick began, but an
	// expression started during that tick, for instance after a confirmation
	// dialog, is stamped with the live clock, which is later.
	clock := 10 * time.Second
	c := New(func() time.Duration { return clock })
	c.Register([]Clip{{Name: "organize_in", Emotion: "organize_in", Frames: 30, Step: 50 * time.Millisecond}, {Name: "organize", Emotion: "organize", Frames: 4, Step: 50 * time.Millisecond, Loop: true, TransitionIn: "organize_in"}})
	tick := clock
	c.Advance(tick, false)
	clock += 4 * time.Second
	c.Apply("late", Analysis{Emotion: "organize", Intensity: 1, Confidence: 1, DurationMS: 5000}, "manual-action")
	if s := c.Advance(tick, false); s.Clip != "organize_in" || s.Frame != 0 {
		t.Fatalf("snapshot = %+v, want the first frame of organize_in", s)
	}
}
