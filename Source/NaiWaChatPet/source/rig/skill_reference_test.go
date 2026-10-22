package rig

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"testing"
)

func TestAdapterMatchesSuppliedSpineCoreReference(t *testing.T) {
	data, err := os.ReadFile("skilldata/validation/spine-core-poses.json")
	if err != nil {
		t.Fatal(err)
	}
	var reference struct {
		Source     string
		Animations map[string][]struct {
			T     float64
			Bones map[string][]float64
		}
	}
	if err = json.Unmarshal(data, &reference); err != nil {
		t.Fatal(err)
	}
	lib, err := DefaultSkillAnimations()
	if err != nil {
		t.Fatal(err)
	}
	samples := 0
	maxLinear, maxPosition := 0.0, 0.0
	worst := ""
	for name, frames := range reference.Animations {
		for _, frame := range frames {
			transforms := lib.transforms(name, frame.T, false)
			for bone, expected := range frame.Bones {
				index, ok := lib.index[bone]
				if !ok {
					t.Fatalf("missing reference bone %s", bone)
				}
				m := transforms[index]
				got := []float64{m.A, m.C, m.B, m.D, m.E, m.F}
				for i, v := range got {
					if i >= len(expected) {
						t.Fatal("reference component missing")
					}
					difference := math.Abs(v - expected[i])
					if i < 4 {
						maxLinear = math.Max(maxLinear, difference)
					} else if difference > maxPosition {
						maxPosition = difference
						worst = fmt.Sprintf("%s/%v/%s/%d", name, frame.T, bone, i)
					}
				}
				samples++
			}
		}
	}
	if samples < 100 {
		t.Fatalf("insufficient reference samples: %d", samples)
	}
	t.Logf("%d bone transforms match %s", samples, reference.Source)
	t.Logf("maximum matrix difference %.8f; position difference %.8f at %s", maxLinear, maxPosition, worst)
	if maxLinear > .01 || maxPosition > .25 {
		t.Fatalf("adapter exceeds subpixel reference tolerance")
	}
}
