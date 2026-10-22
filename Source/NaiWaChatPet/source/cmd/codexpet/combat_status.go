package main

import "math"

func combatStatusSprite(size int, t float64, burn, stun bool, stacks int, protection ...bool) []byte {
	p := newPainter(size, size)
	c := float64(size) / 2
	if len(protection) > 0 && protection[0] {
		p.shade(.27, .82, 1, func(x, y float64) float64 {
			d := math.Hypot((x-c)/(c*.45), (y-c)/(c*.90))
			return math.Max(0, 1-math.Abs(d-1)/.04) * .7
		})
	}
	if burn {
		p.shade(1, .36, .025, func(x, y float64) float64 {
			dx, dy := (x-c)/c, (y-c)/c
			edge := math.Abs(dx) - (.38 + .1*math.Sin(dy*15+t*13))
			return math.Max(0, 1-math.Abs(edge)/.07) * math.Max(0, 1-math.Abs(dy)) * (.35 + .18*math.Sin(t*17+dy*7))
		})
	}
	if stacks > 0 {
		p.shade(.27, 1, .75, func(x, y float64) float64 {
			dx, dy := (x-c)/c, (y-c)/c
			d := math.Hypot(dx, dy*.4)
			return math.Max(0, 1-math.Abs(d-.43)/.035) * .25
		})
	}
	if stun {
		for n := 0; n < 3; n++ {
			theta := t*5 + float64(n)*2*math.Pi/3
			x, y := c+math.Cos(theta)*c*.36, c*.16+math.Sin(theta)*c*.055
			for dy := -4; dy <= 4; dy++ {
				for dx := -4; dx <= 4; dx++ {
					if math.Abs(float64(dx))+math.Abs(float64(dy)) < 5 {
						p.over(int(x)+dx, int(y)+dy, 1, .88, .18, 1)
					}
				}
			}
		}
	}
	return p.bgra()
}
