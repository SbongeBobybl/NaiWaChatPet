package main

import (
	"codexpet/reconstruction/play"
	"testing"
)

func TestCombatResultsFromSnapshotWithoutEvents(t *testing.T) {
	for _, phase := range []play.Phase{play.KnockedOut, play.Over} {
		for _, winner := range []int{-1, 0, 1} {
			for local := 0; local < 2; local++ {
				s := play.CombatSnapshot{Phase: phase, Winner: winner}
				label, banner := combatResult(s, local)
				want := "胜利"
				if winner < 0 {
					want = "平局"
				} else if winner != local {
					want = "失败"
				}
				if label != want || banner == "" {
					t.Fatalf("outcome %d/%d: %s %s", winner, local, label, banner)
				}
			}
		}
	}
	if label, _ := combatResult(play.CombatSnapshot{Phase: play.Fighting, Winner: -1}, 0); label != "" {
		t.Fatal("result leaked into next match")
	}
}
