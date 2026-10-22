package fightnet

import (
	"codexpet/reconstruction/play"
	"testing"
	"time"
)

func TestFinalResultsAndRetunedMatchReachBothClients(t *testing.T) {
	s, address := testServer(t)
	a, b, code, _ := createPair(t, s, address)
	u := b.State()
	if u.Snapshot == nil || u.Snapshot.Clock <= 119 || u.Snapshot.Clock > 120 || u.Snapshot.Fighters[0].MaxHP != 80 || u.Snapshot.Fighters[1].MaxHP != 80 {
		t.Fatal("old match numbers on wire")
	}
	match := u.Match
	for winner := -1; winner <= 1; winner++ {
		if winner >= 0 {
			a.Send(Message{Type: "rematch"})
			b.Send(Message{Type: "rematch"})
			u = waitUpdate(t, b, func(u Update) bool { return u.State == "countdown" })
			match = u.Match
		}
		s.mu.Lock()
		r := s.rooms[code]
		r.state = "fighting"
		r.combat.Arena.Phase = play.Fighting
		r.combat.Arena.Clock = play.CombatDT
		if winner >= 0 {
			r.combat.Fighters[1-winner].HP = 40
			r.combat.Arena.PetHP = r.combat.Fighters[0].HP
			r.combat.Arena.FoeHP = r.combat.Fighters[1].HP
		}
		s.mu.Unlock()
		for n := 0; n < 165; n++ {
			s.Tick(time.Now())
		}
		for _, client := range []*Client{a, b} {
			u = waitUpdate(t, client, func(u Update) bool { return u.State == "over" && u.Match == match })
			if u.Snapshot == nil || u.Snapshot.Winner != winner || u.Snapshot.Phase != play.Over {
				t.Fatal("authoritative final result lost", winner)
			}
			if len(u.Snapshot.Events) != 0 {
				t.Fatal("test must receive a durable result after transient events expired")
			}
		}
	}
}
