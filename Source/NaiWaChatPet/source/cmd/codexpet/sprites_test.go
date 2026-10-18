package main

import "testing"

// layered windows reject colour brighter than its own alpha.
func premultiplied(t *testing.T, name string, pix []byte) {
	t.Helper()
	opaque := 0
	for i := 0; i < len(pix); i += 4 {
		if pix[i] > pix[i+3] || pix[i+1] > pix[i+3] || pix[i+2] > pix[i+3] {
			t.Fatalf("%s: pixel %d has colour above its alpha: %v", name, i/4, pix[i:i+4])
		}
		if pix[i+3] > 0 {
			opaque++
		}
	}
	if opaque == 0 {
		t.Fatalf("%s: drew nothing", name)
	}
}

func TestSpritesArePremultipliedAndVisible(t *testing.T) {
	premultiplied(t, "shadow", shadowSprite(120, 28, 1, true))
	premultiplied(t, "shadow without rim", shadowSprite(40, 10, .5, false))
	for _, heading := range []int{-1, 1} {
		for wing := 0.0; wing < 1; wing += .125 {
			premultiplied(t, "fly", flySprite(31, wing, heading))
		}
	}
	for _, hard := range []bool{false, true} {
		premultiplied(t, "burst", burstSprite(80, .3, hard))
	}
	pix, w, h := textSprite(scoreText(12, 3), 3, true, 1)
	if len(pix) != w*h*4 {
		t.Fatalf("text sprite is %d bytes for %dx%d", len(pix), w, h)
	}
	premultiplied(t, "score", pix)
}

func TestFlySpriteFacesItsHeading(t *testing.T) {
	// The red eye is on the head, which leads; the swept-back wings do not.
	eye := func(pix []byte) float64 {
		var sum, n float64
		for i := 0; i < len(pix); i += 4 {
			if r, g := int(pix[i+2]), int(pix[i+1]); r > 120 && r > 2*g {
				sum += float64(i / 4 % 40)
				n++
			}
		}
		if n == 0 {
			t.Fatal("the fly has no eye")
		}
		return sum / n
	}
	if left, right := eye(flySprite(40, 0, -1)), eye(flySprite(40, 0, 1)); left >= 20 || right <= 20 {
		t.Fatalf("eye of a left-facing fly at x=%.1f, of a right-facing fly at x=%.1f", left, right)
	}
}

func TestFadedTextDisappears(t *testing.T) {
	pix, _, _ := textSprite("+1", 2, false, 0)
	for i := 3; i < len(pix); i += 4 {
		if pix[i] != 0 {
			t.Fatal("a fully faded number still shows")
		}
	}
}

func TestScoreTextShowsAStreakOnlyWhenThereIsOne(t *testing.T) {
	for _, c := range []struct {
		score, streak int
		want          string
	}{{0, 0, "0"}, {7, 1, "7"}, {12, 2, "12 x2"}} {
		if got := scoreText(c.score, c.streak); got != c.want {
			t.Fatalf("scoreText(%d, %d) = %q, want %q", c.score, c.streak, got, c.want)
		}
	}
}

func TestMirrorFlipsRowsAndIsItsOwnInverse(t *testing.T) {
	pix := []uint8{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 0, 0}
	mirror(pix, 14, 3, 1)
	if want := []uint8{9, 10, 11, 12, 5, 6, 7, 8, 1, 2, 3, 4}; string(pix[:12]) != string(want) {
		t.Fatalf("mirrored row = %v, want %v", pix[:12], want)
	}
	if pix[12] != 0 || pix[13] != 0 {
		t.Fatal("mirror wrote into the row padding")
	}
	mirror(pix, 14, 3, 1)
	if pix[0] != 1 || pix[8] != 9 {
		t.Fatal("mirroring twice did not restore the row")
	}
}
