package desktopchat

import "testing"

func TestDPIAndMultiMonitorBounds(t *testing.T) {
	for _, dpi := range []uint32{96, 120, 144, 192, 288} {
		for _, work := range []Rect{{0, 0, 1920, 1040}, {-1920, -200, 1920, 1080}, {400, -1600, 1440, 1560}} {
			for _, p := range []Point{{work.X, work.Y}, {work.Right() - 160, work.Y}, {work.X, work.Bottom() - 230}, {work.Right() - 160, work.Bottom() - 230}, {work.X + 600, work.Y + 500}} {
				pet := Rect{p.X, p.Y, 153, 204}
				input := Input(pet, work, pet.Y+196, dpi)
				bubble := Bubble(pet, work, Point{pet.X + 48, pet.Y + 42}, dpi, 30, false)
				if !work.Contains(input) || !work.Contains(bubble.Rect) {
					t.Fatalf("DPI %d work=%+v input=%+v bubble=%+v", dpi, work, input, bubble)
				}
				if bubble.Rect.X < pet.Right() && bubble.Rect.Right() > pet.X && bubble.Rect.Y < pet.Bottom() && bubble.Rect.Bottom() > pet.Y {
					t.Fatalf("bubble covers face/body %+v %+v", pet, bubble)
				}
			}
		}
	}
}
func TestSeparateAnchorsAndLongReplyLifetime(t *testing.T) {
	a := Interpolate([]Anchor{{0, 10, 20}, {10, 30, 60}}, 5)
	if a != (Point{20, 40}) {
		t.Fatal(a)
	}
	if DismissAfter(string(make([]rune, 200))) != 0 {
		t.Fatal("long reply auto dismissed")
	}
}
