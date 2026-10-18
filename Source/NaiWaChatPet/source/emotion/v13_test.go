package emotion

import (
	"codexpet/reconstruction/compositor"
	"image"
	"testing"
	"time"
)

func TestHappyRequestsOneAtomicLaughAndCooldown(t *testing.T) {
	var now time.Duration
	c := New(func() time.Duration { return now })
	if !c.RequestEmotion("happy", .95, "conversation") {
		t.Fatal("happy rejected")
	}
	if c.ConsumeLaugh(true) {
		t.Fatal("atomic action interrupted")
	}
	c.Advance(now, false)
	if !c.ConsumeLaugh(false) || c.ConsumeLaugh(false) {
		t.Fatal("laugh not exactly once")
	}
	now = 10 * time.Second
	if c.RequestEmotion("happy", .95, "conversation") {
		t.Fatal("happy cooldown missing")
	}
	if c.RequestEmotion("happy", .5, "conversation") {
		t.Fatal("low confidence laugh")
	}
}
func TestV13RuntimeClipsAndEveryFrameAlpha(t *testing.T) {
	l, w := LoadLibrary("../../animations", image.Rect(0, 0, 420, 560))
	if len(w) > 0 {
		t.Fatal(w)
	}
	// Five filmed expressions with an entry and an exit each, one held pose
	// for every posed expression, and an entry, loop and exit for each scene.
	want := 15
	for _, e := range Catalog {
		clips := []string{e.ID}
		switch {
		case e.Posed:
		case e.Scene:
			clips = append(clips, e.ID+"_in", e.ID+"_out")
		default:
			continue
		}
		for _, name := range clips {
			want++
			if _, ok := l.Manifest(name); !ok {
				t.Fatalf("%s has no imported clip %s", e.ID, name)
			}
		}
	}
	if len(l.Clips()) != want {
		t.Fatalf("clips=%d, want %d", len(l.Clips()), want)
	}
	n := 0
	for _, clip := range l.Clips() {
		for i := 0; i < clip.Frames; i++ {
			f, e := l.Frame(clip.Name, i)
			if e != nil {
				t.Fatal(e)
			}
			pixels := compositor.BGRA(f)
			for j := 0; j < len(pixels); j += 4 {
				if pixels[j] > pixels[j+3] || pixels[j+1] > pixels[j+3] || pixels[j+2] > pixels[j+3] {
					t.Fatal("invalid premul")
				}
			}
			n++
		}
	}
	t.Logf("validated %d imported frames", n)
}
