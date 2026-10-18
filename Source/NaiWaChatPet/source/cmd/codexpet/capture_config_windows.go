//go:build windows

package main

import "codexpet/reconstruction/companion"

func configureCompanionCapture(s *companion.Server) {
	s.ConfigureCapture(captureScreenScopeJPEG, listCaptureScopes)
	s.ConfigureScreenContext(nativeScreenContext)
}
