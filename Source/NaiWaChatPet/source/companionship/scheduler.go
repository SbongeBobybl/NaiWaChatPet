package companionship

import "time"

type Config struct {
	Seconds                                           int
	ActionsOnly                                       bool
	Enabled, IdleActions, Greeting, Breaks, Encourage bool
	Minutes, MaxPerHour, QuietStart, QuietEnd         int
}
type Scheduler struct {
	Reason          string
	last            time.Time
	hour            time.Time
	count, sequence int
	lastIdle        time.Time
}
type Event struct{ Text, Action string }

func Quiet(hour, start, end int) bool {
	if start == end {
		return false
	}
	if start < end {
		return hour >= start && hour < end
	}
	return hour >= start || hour < end
}
func (s *Scheduler) Tick(now, lastInteraction time.Time, c Config) Event {
	s.Reason = "等待间隔"
	if Quiet(now.Hour(), c.QuietStart, c.QuietEnd) {
		s.Reason = "夜间免打扰"
		return Event{}
	}
	interval := Interval(c)
	if c.Seconds >= 30 && now.Sub(lastInteraction) < interval {
		s.Reason = "等待用户忙完"
		return Event{}
	}
	hour := now.Truncate(time.Hour)
	if hour != s.hour {
		s.hour = hour
		s.count = 0
	}
	if c.IdleActions && now.Sub(lastInteraction) >= interval && now.Sub(s.lastIdle) >= interval {
		s.lastIdle = now
		if !c.Enabled {
			return Event{Action: "confused"}
		}
	}
	if !c.Enabled {
		s.Reason = "主动陪伴已关闭"
		return Event{}
	}
	if s.count >= max(1, min(120, c.MaxPerHour)) {
		s.Reason = "已达每小时次数上限"
		return Event{}
	}
	if !s.last.IsZero() && now.Sub(s.last) < interval {
		return Event{}
	}
	var texts []string
	if c.Greeting {
		texts = append(texts, "我在这儿。想说点什么，点我就好。")
	}
	if c.Breaks {
		texts = append(texts, "看一会儿远处吧，手腕和肩膀也可以放松一下。")
	}
	if c.Encourage {
		texts = append(texts, "把下一步做小一点，慢慢来就能继续前进。")
	}
	if len(texts) == 0 && !c.ActionsOnly {
		s.Reason = "未选择提醒内容"
		return Event{}
	}
	text := ""
	if !c.ActionsOnly {
		text = texts[s.sequence%len(texts)]
	}
	s.sequence++
	s.last = now
	s.count++
	s.Reason = "已触发桌面提醒"
	return Event{Text: text, Action: "confused"}
}

func (s *Scheduler) Reset(now time.Time) { s.last = now; s.Reason = "等待间隔" }
func (s *Scheduler) Last() time.Time     { return s.last }
func (s *Scheduler) Next(c Config) time.Time {
	return s.last.Add(Interval(c))
}
func Interval(c Config) time.Duration {
	if c.Seconds > 0 {
		return time.Duration(max(30, min(10800, c.Seconds))) * time.Second
	}
	return time.Duration(max(1, min(180, c.Minutes))) * time.Minute
}
