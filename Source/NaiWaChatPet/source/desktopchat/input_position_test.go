package desktopchat

import "testing"

func TestInputStaysCenteredBelowPetNearTaskbarAndScreenEdge(t *testing.T) {
	for _, dpi := range []uint32{96, 144, 192} {
		work := Rect{0, 0, 1920, 1040}
		pet := Rect{1760, 810, 153, 204}
		ground := 1006
		p := PetForInput(pet, work, ground, dpi)
		input := Input(p, work, ground+p.Y-pet.Y, dpi)
		if input.Y < p.Bottom() || !work.Contains(input) || absInt(input.X+input.W/2-p.X-p.W/2) > 1 {
			t.Fatalf("input not centered below: pet=%+v input=%+v", p, input)
		}
	}
}
func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
