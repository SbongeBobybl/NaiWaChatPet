package play

import (
	"fmt"
	"strings"
)

type GameSettings struct {
	Keys       map[string]uint16 `json:"keys"`
	Difficulty AIDifficulty      `json:"difficulty"`
	Volume     float64           `json:"volume"`
	Effects    float64           `json:"effects"`
	Server     string            `json:"server"`
}

func DefaultGameSettings() GameSettings {
	return GameSettings{Keys: map[string]uint16{"left": 'A', "right": 'D', "up": 'W', "down": 'S', "run": 0x10, "attack": 'J', "jump": 'K', "fan": 'U', "roll": 'I', "power": 'O', "dodge": 'L'}, Difficulty: NormalAI, Volume: .65, Effects: 1, Server: "127.0.0.1:28760"}
}
func (g *GameSettings) Normalize() {
	d := DefaultGameSettings()
	if g.Keys == nil {
		g.Keys = d.Keys
	}
	for k, v := range d.Keys {
		if g.Keys[k] == 0 {
			g.Keys[k] = v
		}
	}
	if g.Difficulty != EasyAI && g.Difficulty != HardAI {
		g.Difficulty = NormalAI
	}
	g.Volume = max(0, min(1, g.Volume))
	g.Effects = max(.25, min(1, g.Effects))
	if strings.TrimSpace(g.Server) == "" {
		g.Server = d.Server
	}
}
func (g *GameSettings) Bind(action string, key uint16) error {
	if _, ok := g.Keys[action]; !ok {
		return fmt.Errorf("unknown action")
	}
	if key == 0x1b || key == 0x0d || key == 'T' || key < 0x10 || key > 0xfe {
		return fmt.Errorf("reserved key")
	}
	aliases := map[uint16]string{0x20: "jump", 0x25: "left", 0x26: "up", 0x27: "right", 0x28: "down"}
	if owner, ok := aliases[key]; ok && owner != action {
		return fmt.Errorf("compatibility key reserved for %s", owner)
	}
	for a, k := range g.Keys {
		if a != action && k == key {
			return fmt.Errorf("key already bound to %s", a)
		}
	}
	g.Keys[action] = key
	return nil
}

type CombatViewport struct{ X, Y, Scale float64 }

func (v CombatViewport) Point(x, y float64) (float64, float64) {
	return v.X + x*v.Scale, v.Y + y*v.Scale
}
func (v CombatViewport) World(x, y float64) (float64, float64) {
	return (x - v.X) / v.Scale, (y - v.Y) / v.Scale
}
