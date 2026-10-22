package main

import "codexpet/reconstruction/play"

// Results come from the durable authoritative snapshot, never a transient KO event.
func combatResult(s play.CombatSnapshot, local int) (label, banner string) {
	if s.Phase != play.KnockedOut && s.Phase != play.Over {
		return "", ""
	}
	if s.Winner < 0 {
		return "平局", "DRAW"
	}
	if s.Winner == local {
		return "胜利", "YOU WIN"
	}
	return "失败", "YOU LOSE"
}
