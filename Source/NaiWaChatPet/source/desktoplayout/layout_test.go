package desktoplayout

import (
	"fmt"
	"reflect"
	"testing"
)

func sample() Snapshot {
	return Snapshot{Spacing: Point{80, 100}, DPI: 144, Monitors: []Monitor{{ID: "main", Area: Rect{0, 0, 800, 600}, Primary: true}, {ID: "left", Area: Rect{-800, 0, 0, 600}}}, Icons: []Icon{{Key: "system", Position: Point{10, 12}, Fixed: true}, {Key: "a", Position: Point{140, 180}}, {Key: "b", Position: Point{310, 180}}, {Key: "secondary", Position: Point{-200, 200}}}}
}
func TestZonesAndNaturalConstraints(t *testing.T) {
	s := sample()
	s.Icons[1].Category = "apps"
	s.Icons[2].Category = "projects"
	if e := Constrain(&s, "软件放右上，项目放左下，回收站固定"); e != nil {
		t.Fatal(e)
	}
	p, e := PlanGrid(s, "zones", nil, false)
	if e != nil {
		t.Fatal(e)
	}
	if e = Validate(p); e != nil {
		t.Fatal(e)
	}
	for _, m := range p.Moves {
		if m.Key == "a" && m.Target.X < 400 {
			t.Fatal("wrong app zone")
		}
		if m.Key == "b" && m.Target.Y < 300 {
			t.Fatal("wrong project zone")
		}
	}
	if e = Constrain(&s, "删除所有文件"); e == nil {
		t.Fatal("unknown action accepted")
	}
}
func TestGridDeterministicBoundsPinsAndSecondary(t *testing.T) {
	for _, mode := range []string{"columns", "rows", "zones", "minimal"} {
		s := sample()
		p, e := PlanGrid(s, mode, map[string]bool{"a": true}, false)
		if e != nil {
			t.Fatal(e)
		}
		q, _ := PlanGrid(s, mode, map[string]bool{"a": true}, false)
		if !reflect.DeepEqual(p, q) {
			t.Fatal("unstable")
		}
		for _, m := range p.Moves {
			if m.Key != "b" || !inside(m.Target, s.Monitors[0].Area) || near(m.Target, s.Icons[0].Position, s.Spacing) || near(m.Target, s.Icons[1].Position, s.Spacing) {
				t.Fatal(m)
			}
		}
	}
}
func TestNoOverlapsAndNegativeMonitors(t *testing.T) {
	s := sample()
	for i := 0; i < 20; i++ {
		s.Icons = append(s.Icons, Icon{Key: fmt.Sprint(i), Position: Point{200, 200}})
	}
	p, e := PlanGrid(s, "rows", nil, true)
	if e != nil {
		t.Fatal(e)
	}
	seen := map[Point]bool{}
	for _, m := range p.Moves {
		if seen[m.Target] {
			t.Fatal("collision")
		}
		seen[m.Target] = true
		if !inside(m.Target, s.Monitors[0].Area) && !inside(m.Target, s.Monitors[1].Area) {
			t.Fatal("out of bounds")
		}
	}
}
func TestOverflowAndAutoArrangeFailClosed(t *testing.T) {
	s := sample()
	s.AutoArrange = true
	if _, e := PlanGrid(s, "columns", nil, false); e == nil {
		t.Fatal("auto arrange accepted")
	}
	s.AutoArrange = false
	s.Monitors[0].Area = Rect{0, 0, 100, 100}
	s.Icons[1].Position = Point{20, 20}
	if _, e := PlanGrid(s, "columns", nil, false); e == nil {
		t.Fatal("overflow accepted")
	}
}
func TestRestoreSkipsManualMovesAndTopologyChanges(t *testing.T) {
	s := sample()
	p, _ := PlanGrid(s, "columns", nil, false)
	current := s
	current.Icons = append([]Icon(nil), s.Icons...)
	for i := range current.Icons {
		for _, m := range p.Moves {
			if m.Key == current.Icons[i].Key {
				current.Icons[i].Position = m.Target
			}
		}
	}
	r, e := Restore(p, current)
	if e != nil || len(r.Moves) != len(p.Moves) {
		t.Fatalf("%+v %v", r, e)
	}
	current.DPI = 192
	if _, e = Restore(p, current); e == nil {
		t.Fatal("DPI change ignored")
	}
}
