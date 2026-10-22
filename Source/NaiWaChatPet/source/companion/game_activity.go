package companion

import "time"

// DesktopGameActive is transient UI ownership, never a persisted preference.
// It pauses only unsolicited company; normal user-initiated chat still works.
func (s *Server) DesktopGameActive(active bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.desktopGameActive == active {
		return
	}
	s.desktopGameActive = active
	s.scheduler.Reset(time.Now())
	if active {
		s.localQueue = nil // Expired reminders must not burst on leaving a match.
		if s.activeProactive && s.activeCancel != nil {
			s.activeCancel()
		}
		s.scheduler.Reason = "格斗期间暂停主动陪伴，退出游戏后恢复"
	}
}
