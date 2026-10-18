package interaction

import "time"

type Point struct{ X, Y int }

// Gesture waits for the OS double-click interval before confirming a click.
// Dragging and repeated clicks are tracked independently of animation state.
type Gesture struct {
	Interval               time.Duration
	ThresholdX, ThresholdY int
	down, drag, suppress   bool
	start                  Point
	pending                bool
	due, blockedUntil      time.Duration
}

func (g *Gesture) Down(now time.Duration, p Point) {
	g.down = true
	g.drag = false
	g.start = p
	g.suppress = now < g.blockedUntil
}
func (g *Gesture) Move(p Point) bool {
	if !g.down {
		return false
	}
	x, y := p.X-g.start.X, p.Y-g.start.Y
	if x < 0 {
		x = -x
	}
	if y < 0 {
		y = -y
	}
	tx, ty := g.ThresholdX, g.ThresholdY
	if tx < 1 {
		tx = 4
	}
	if ty < 1 {
		ty = 4
	}
	if x >= tx || y >= ty {
		g.drag = true
		g.pending = false
	}
	return g.drag
}
func (g *Gesture) Up(now time.Duration) {
	if g.down && !g.drag && !g.suppress {
		g.pending = true
		interval := g.Interval
		if interval <= 0 {
			interval = 500 * time.Millisecond
		}
		g.due = now + interval
	}
	g.down = false
	g.drag = false
}
func (g *Gesture) Double(now time.Duration) {
	g.pending = false
	g.down = false
	g.drag = false
	g.blockedUntil = now + g.Interval
	g.suppress = true
}
func (g *Gesture) CancelPress() { g.down = false; g.drag = false }
func (g *Gesture) Poll(now time.Duration) bool {
	if g.pending && !g.down && now >= g.due {
		g.pending = false
		return true
	}
	return false
}
