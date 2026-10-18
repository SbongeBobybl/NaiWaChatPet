package companionship

import (
	"testing"
	"time"
)

func TestThirtySecondInactivityAndSpacing(t *testing.T) {
	at:=time.Date(2026,10,11,12,0,0,0,time.Local)
	s:=Scheduler{}
	c:=Config{Seconds:30,Enabled:true,Greeting:true,MaxPerHour:120}
	s.Reset(at)
	if s.Tick(at.Add(29*time.Second),at,c).Text!=""{t.Fatal("fires before 30s")}
	if s.Tick(at.Add(30*time.Second),at,c).Text==""{t.Fatal("does not fire at 30s")}
	if s.Tick(at.Add(59*time.Second),at,c).Text!=""{t.Fatal("next event too soon")}
	if s.Tick(at.Add(60*time.Second),at,c).Text==""{t.Fatal("next event missing")}
	if s.Tick(at.Add(90*time.Second),at.Add(80*time.Second),c).Text!=""{t.Fatal("recent user activity ignored")}
}
