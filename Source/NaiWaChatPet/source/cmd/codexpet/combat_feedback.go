package main

import (
	"fmt"
	"math"
	"strings"

	"codexpet/reconstruction/play"
)

// Five layers: contact flash, lobed hot core, expanding shock front,
// ballistic sparks/debris and cooling tongues/afterglow.
func combatEffectSprite(kind string, size int, t, intensity float64) []byte {
	p := newPainter(size, size)
	c := float64(size) / 2
	t = math.Max(0, math.Min(1, t))
	fade := (1 - t) * (1 - t)
	radius := c * (.15 + .53*(1-math.Exp(-t*9)))
	if kind == "explosion" {
		p.shade(1, .35, .025, func(x, y float64) float64 {
			dx, dy := x-c, y-c
			angle := math.Atan2(dy, dx)
			lobes := 1 + .16*math.Sin(angle*7+t*9) + .08*math.Sin(angle*13-t*17)
			d := math.Hypot(dx, dy*1.07) / (radius * lobes)
			return math.Min(1, math.Max(0, 1-d)*4) * math.Min(1, t*25) * math.Max(0, 1-t*1.1)
		})
		p.shade(1, .87, .3, func(x, y float64) float64 {
			d := math.Hypot(x-c, (y-c)*1.15) / (radius * .67)
			return math.Min(1, math.Max(0, 1-d)*3) * math.Max(0, 1-t*1.7)
		})
		if t < .14 {
			p.shade(1, 1, .94, func(x, y float64) float64 {
				dx, dy := x-c, y-c
				theta := math.Atan2(dy, dx)
				star := .5 + .5*math.Pow(math.Abs(math.Cos(theta*4)), 8)
				return math.Max(0, 1-math.Hypot(dx, dy)/(c*.6*star)) * (1 - t/.14)
			})
		}
		p.shade(1, .74, .25, func(x, y float64) float64 {
			r := c * (.12 + .85*(1-math.Exp(-t*5)))
			d := math.Abs(math.Hypot(x-c, (y-c)*1.1) - r)
			return math.Max(0, 1-d/(1.2+c*.025*(1-t))) * fade
		})
		for n := 0; n < int(18*intensity); n++ {
			angle := float64(n) * 2.399963
			speed := .5 + .5*math.Sin(float64(n)*4.7)
			x := c + math.Cos(angle)*c*t*(.7+speed)
			y := c + math.Sin(angle)*c*t*(.7+speed) + c*.5*t*t
			rr := 1.2 + c*.025*(1-t)
			for py := int(y - rr); py <= int(y+rr); py++ {
				for px := int(x - rr); px <= int(x+rr); px++ {
					alpha := math.Max(0, 1-math.Hypot(float64(px)-x, float64(py)-y)/rr) * fade
					p.over(px, py, 1, .48+.4*speed, .08, alpha)
				}
			}
		}
		if t > .45 {
			p.shade(.95, .25, .035, func(x, y float64) float64 {
				dx, dy := (x-c)/c, (y-c)/c
				w := .18 + .08*math.Sin(dy*18+t*22)
				return math.Max(0, 1-math.Abs(dx)/w) * math.Max(0, 1-math.Abs(dy+.25*t)) * (1 - t) * .6
			})
		}
	} else if kind == "power" {
		p.shade(.3, 1, .74, func(x, y float64) float64 {
			dx, dy := x-c, (y-c)*1.25
			r := c * (.18 + .78*t)
			return math.Max(0, 1-math.Abs(math.Hypot(dx, dy)-r)/(2+c*.04)) * fade
		})
		for n := 0; n < 8; n++ {
			angle := float64(n) * math.Pi / 4
			x, y := c+math.Cos(angle)*c*.5, c+math.Sin(angle)*c*.4-c*t*.5
			p.box(x-1, y-4, x+1, y+4, [3]float64{.6, 1, .8}, fade)
		}
	} else if kind == "dodge" {
		p.shade(.55, .82, 1, func(x, y float64) float64 {
			dx, dy := (x-c)/(c*.48), (y-c)/(c*.83)
			return math.Max(0, 1-dx*dx-dy*dy) * fade * .24 * intensity
		})
	} else {
		p.shade(.6, .87, 1, func(x, y float64) float64 {
			r := math.Hypot(x-c, (y-c)*1.4)
			return math.Max(0, 1-math.Abs(r-c*(.2+.7*t))/2) * fade
		})
		for i := 0; i < 6; i++ {
			y := c + float64(i-3)*c*.13
			p.box(c*.05, y, c*(.5+t), y+1.5, [3]float64{.75, .95, 1}, fade*.7*intensity)
		}
	}
	return p.bgra()
}
func combatHUDSprite(w int, s play.CombatSnapshot, keys map[string]uint16, local int, status string) ([]byte, int) {
	local = max(0, min(1, local))
	l, r := s.Fighters[local], s.Fighters[1-local]
	lh, rh := l.MaxHP, r.MaxHP
	if lh <= 0 {
		lh = 100
	}
	if rh <= 0 {
		rh = 100
	}
	base, bh := hudSprite(w, l.HP/lh, math.Max(l.HP, s.HPShown[local])/lh, r.HP/rh, math.Max(r.HP, s.HPShown[1-local])/rh, int(math.Ceil(s.Clock)), max(1, s.Round), "RIVAL", [3]float64{.88, .3, .18})
	dot := max(2, w/240)
	p := newPainter(w, bh+dot*34)
	p.blit(base, w, bh, 0, 0)
	mid := float64(w) / 2
	left := float64(dot * 2)
	gap := float64(dot * 12)
	barWidth := mid - gap - left
	for side, f := range []play.FighterState{l, r} {
		from := left
		if side == 1 {
			from = mid + gap
		}
		y := float64(dot * 16)
		p.box(from-1, y-1, from+barWidth+1, y+float64(dot*3)+1, [3]float64{.015, .05, .09}, 1)
		p.box(from, y, from+barWidth, y+float64(dot*3), [3]float64{.02, .12, .18}, 1)
		fill := barWidth * math.Max(0, math.Min(1, f.MP/100))
		x0, x1 := from, from+fill
		if side == 0 {
			x0, x1 = from+barWidth-fill, from+barWidth
		}
		p.box(x0, y, x1, y+float64(dot), [3]float64{.54, .94, 1}, 1)
		p.box(x0, y+float64(dot), x1, y+float64(dot*3), [3]float64{.06, .57, .83}, 1)
		text := fmt.Sprintf("HP %.1f MP %.1f", f.HP, f.MP)
		pix, tw, th := textSprite(text, max(1, dot/2), false, 1)
		p.blit(pix, tw, th, int(from), dot*21)
		actions := []string{"fan", "roll", "power", "dodge"}
		costs := []float64{10, 20, 30, 0}
		if s.Rules != nil && len(s.Rules.Skills) == 4 {
			for i := range costs {
				costs[i] = s.Rules.Skills[i].Cost
			}
		}
		for i, action := range actions {
			span := barWidth / 4
			x := from + float64(i)*span
			cool := 0.0
			if f.Cooldown[i] > s.Tick {
				cool = float64(f.Cooldown[i]-s.Tick) / play.CombatHz
			}
			ready := f.MP >= costs[i] && cool <= 0 && s.Tick >= f.StunUntil
			alpha := .4
			if ready {
				alpha = 1
			}
			p.box(x, float64(dot*28), x+span-4, float64(dot*37), [3]float64{.025, .09, .14}, .92)
			label := fmt.Sprintf("%c %.0f", keys[action], costs[i])
			if side == 1 {
				label = fmt.Sprintf("%d %.0f", i+1, costs[i])
			}
			if cool > 0 {
				label = fmt.Sprintf("%c %.1f", keys[action], cool)
			}
			pix, tw, th = textSprite(label, max(1, dot/2), false, alpha)
			p.blit(pix, tw, th, int(x)+3, dot*29)
		}
		state := fmt.Sprintf("BUFF %d", f.BuffStacks)
		if f.BurnLeft > 0 {
			state += " BURN"
		}
		if s.Tick < f.StunUntil {
			state += " STUN"
		}
		if s.Tick < f.ProtectionUntil {
			state += " GUARD"
		}
		if f.Combo >= 2 {
			state += fmt.Sprintf(" %d HIT", f.Combo)
		}
		pix, tw, th = textSprite(state, max(1, dot/2), true, 1)
		p.blit(pix, tw, th, int(from), dot*39)
	}
	if status != "" {
		text := strings.ToUpper(status)
		if len(text) > 60 {
			text = text[:60]
		}
		pix, tw, th := textSprite(text, max(1, dot/2), false, 1)
		p.blit(pix, tw, th, max(0, (w-tw)/2), dot*47)
	}
	return p.bgra(), p.h
}
