package interaction

import (
	"testing"
	"time"
)

func TestSingleDoubleDragAndTriple(t *testing.T) {
	g := Gesture{Interval: 400 * time.Millisecond, ThresholdX: 5, ThresholdY: 5}
	g.Down(0, Point{0, 0})
	g.Up(10 * time.Millisecond)
	if g.Poll(300*time.Millisecond) || !g.Poll(410*time.Millisecond) || g.Poll(time.Second) {
		t.Fatal("single click timing")
	}
	g.Down(time.Second, Point{})
	g.Up(time.Second + 20*time.Millisecond)
	g.Double(time.Second + 200*time.Millisecond)
	if g.Poll(2 * time.Second) {
		t.Fatal("double fired single")
	}
	g.Down(2*time.Second, Point{})
	if g.Move(Point{2, 2}) {
		t.Fatal("jitter dragged")
	}
	if !g.Move(Point{6, 2}) {
		t.Fatal("drag threshold")
	}
	g.Up(2100 * time.Millisecond)
	if g.Poll(3 * time.Second) {
		t.Fatal("drag fired chat")
	}
	g.Down(4*time.Second, Point{})
	g.Up(4010 * time.Millisecond)
	g.Double(4100 * time.Millisecond)
	g.Down(4200*time.Millisecond, Point{})
	g.Up(4210 * time.Millisecond)
	if g.Poll(5 * time.Second) {
		t.Fatal("triple-click leaked single")
	}
}
func TestCaptureReleaseDoesNotEraseConfirmedSingle(t *testing.T) {
	g := Gesture{Interval: time.Millisecond}
	g.Down(0, Point{})
	g.Up(0)
	g.CancelPress()
	if !g.Poll(time.Millisecond) {
		t.Fatal("capture changed lost click")
	}
}
