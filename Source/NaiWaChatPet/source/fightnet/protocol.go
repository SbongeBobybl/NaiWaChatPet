// Package fightnet implements room-isolated, server-authoritative combat.
// Only input, room identity and combat state cross the connection.
package fightnet

import (
	"codexpet/reconstruction/play"
)

const ProtocolVersion = 2
const MaxMessageBytes = 4096

type Message struct {
	Version int        `json:"version"`
	Type    string     `json:"type"`
	Room    string     `json:"room,omitempty"`
	Token   string     `json:"token,omitempty"`
	Seq     uint64     `json:"seq,omitempty"`
	Input   play.Input `json:"input,omitempty"`
}
type Update struct {
	Version   int                  `json:"version"`
	Type      string               `json:"type"`
	Room      string               `json:"room,omitempty"`
	Token     string               `json:"token,omitempty"`
	Player    int                  `json:"player"`
	Error     string               `json:"error,omitempty"`
	State     string               `json:"state,omitempty"`
	Match     uint64               `json:"match,omitempty"`
	Ready     [2]bool              `json:"ready"`
	Connected [2]bool              `json:"connected"`
	Countdown int                  `json:"countdown,omitempty"`
	Snapshot  *play.CombatSnapshot `json:"snapshot,omitempty"`
}
