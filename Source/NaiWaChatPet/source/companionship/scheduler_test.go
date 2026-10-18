package companionship

import (
	"testing"
	"time"
)

func TestDefaultOffQuietCooldownAndLimit(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.Local)
	last := now.Add(-time.Hour)
	s := Scheduler{}
	if s.Tick(now, last, Config{Minutes: 10, MaxPerHour: 2}).Text != "" {
		t.Fatal("default proactive on")
	}
	c := Config{Enabled: true, Greeting: true, Minutes: 10, MaxPerHour: 2, QuietStart: 23, QuietEnd: 8}
	if s.Tick(now, last, c).Text == "" {
		t.Fatal("missing local reminder")
	}
	if s.Tick(now.Add(time.Minute), last, c).Text != "" {
		t.Fatal("cooldown ignored")
	}
	if s.Tick(now.Add(11*time.Minute), last, c).Text == "" {
		t.Fatal("second reminder missing")
	}
	if s.Tick(now.Add(22*time.Minute), last, c).Text != "" {
		t.Fatal("hourly limit")
	}
	if s.Tick(now.Add(12*time.Hour), last, c).Text != "" {
		t.Fatal("quiet hours")
	}
}
