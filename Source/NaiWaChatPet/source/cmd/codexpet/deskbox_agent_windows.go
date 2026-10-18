package main

import (
	ai "codexpet/reconstruction/aiorganization"
	"codexpet/reconstruction/companion"
	"path/filepath"
)

func startDeskBoxAgent(s *companion.Server) (func(), error) {
	profile, err := companion.ProfileDirectory()
	if err != nil {
		return nil, err
	}
	agent, err := ai.Start(filepath.Join(profile, "deskbox-agent"), deskBoxHandler{companion.DeskBoxAgent{Server: s}})
	if err != nil {
		return nil, err
	}
	return func() { agent.Close() }, nil
}
