package companion

import (
	"errors"
	"strings"
	"time"
)

func (s *Server) validateV13Locked(c *Settings) error {
	if c.Personality == nil {
		p := DefaultPersonality()
		if s.settings.Personality != nil {
			p = *s.settings.Personality
		}
		c.Personality = &p
	}
	for _, v := range []int{c.Personality.Gentleness, c.Personality.Liveliness, c.Personality.Humor, c.Personality.Initiative} {
		if v < 0 || v > 100 {
			return errors.New("性格参数范围为 0–100")
		}
	}
	if len(c.ExcludedApps) > 1500 {
		return errors.New("排除应用列表过长")
	}
	if s.settings.AllowPaidProactive && !c.AllowPaidProactive && s.activeScreen {
		s.stopScreenLocked("paid permission revoked")
	}
	if c.ObservationMode == "" {
		if c.ScreenEnabled {
			c.ObservationMode = "periodic"
		} else {
			c.ObservationMode = "off"
		}
	}
	switch c.ObservationMode {
	case "off":
		c.ScreenEnabled = false
	case "each", "smart", "periodic":
		c.ScreenEnabled = true
	default:
		return errors.New("未知屏幕感知模式")
	}
	changed := c.ObservationMode != s.settings.ObservationMode || c.ScreenScope != s.settings.ScreenScope || c.ExcludedApps != s.settings.ExcludedApps
	if c.ScreenEnabled && changed && !c.ScreenConsent {
		return errors.New("开启或更改屏幕感知模式/范围需要明确授权")
	}
	if c.ScreenEnabled && changed {
		s.stopScreenLocked("reconfigured")
	}
	return nil
}
func (s *Server) restoreV13Preferences(p Preferences) {
	if p.ProactiveSeconds >= 30 && p.ProactiveSeconds <= 10800 {
		s.settings.ProactiveSeconds = p.ProactiveSeconds
		s.settings.Proactive = p.Proactive
	}
	if p.Personality != nil {
		s.settings.Personality = p.Personality
	}
	s.settings.ExcludedApps = p.ExcludedApps
	s.settings.ActionsOnly = p.ActionsOnly
	s.settings.ObservationMode = "off"
	s.settings.AllowPaidProactive = false
	s.settings.ScreenEnabled = false
}
func (s *Server) schedulerStatusLocked(now time.Time) SchedulerStatus {
	r := SchedulerStatus{Running: s.settings.Proactive || s.settings.IdleActions, Reason: s.scheduler.Reason, Queued: len(s.localQueue)}
	if !s.scheduler.Last().IsZero() {
		r.Last = s.scheduler.Last().Format(time.RFC3339)
		r.Next = s.scheduler.Next(s.schedulerConfigLocked()).Format(time.RFC3339)
	}
	if !r.Running {
		r.Reason = "主动陪伴已关闭"
	}
	return r
}
func shortObservation(text string) string {
	r := []rune(strings.TrimSpace(text))
	if len(r) > 220 {
		r = r[:220]
	}
	return string(r)
}
func (s *Server) DesktopReplyVisible(visible bool) {
	s.mu.Lock()
	s.desktopReplyVisible = visible
	s.mu.Unlock()
}
