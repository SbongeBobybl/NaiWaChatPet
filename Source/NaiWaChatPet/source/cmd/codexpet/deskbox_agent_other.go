//go:build !windows

package main

import "codexpet/reconstruction/companion"

func startDeskBoxAgent(s *companion.Server) (func(), error) { return func() {}, nil }

func acquireDeskBoxPetInstance() (func(), bool, error) { return func() {}, true, nil }
