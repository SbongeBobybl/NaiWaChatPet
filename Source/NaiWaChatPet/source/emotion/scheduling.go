package emotion

import "time"

type Outcome struct {
	RequestID, Reason, Current, Pending string
	Accepted                            bool
	Priority                            int
}

func (c *Controller) Apply(id string, a Analysis, source string) Outcome {
	c.mu.Lock()
	defer c.mu.Unlock()
	o := Outcome{RequestID: id}
	result := func(reason string, accepted bool) Outcome {
		o.Reason = reason
		o.Accepted = accepted
		o.Current = "neutral"
		if c.current != nil {
			o.Current = c.current.emotion
			o.Priority = c.current.priority
		}
		if c.pending != nil {
			o.Pending = c.pending.emotion
		}
		return o
	}
	if !a.Valid() {
		a = Neutral()
	}
	if a.Confidence < MinimumConfidence {
		a = Neutral()
	}
	if c.seen == nil {
		c.seen = map[string]bool{}
	}
	if id != "" && c.seen[id] {
		return result("duplicate_request", false)
	}
	if id != "" {
		c.seen[id] = true
		c.seenOrder = append(c.seenOrder, id)
		if len(c.seenOrder) > 64 {
			delete(c.seen, c.seenOrder[0])
			c.seenOrder = c.seenOrder[1:]
		}
	}
	now := c.clock()
	// A mild happiness is a smile; only a strong one earns the full laugh.
	if a.Emotion == "happy" && a.Intensity >= .18 && a.Intensity < .6 && c.mapping["smile"] != "" {
		a.Emotion = "smile"
	}
	if a.Emotion == "neutral" || a.Intensity < .18 || a.Emotion == "happy" && a.Intensity < .6 {
		c.pending = nil
		c.idleRequested = true
		return result("return_to_idle", true)
	}
	if c.current != nil && c.current.emotion == a.Emotion && c.phase != "out" {
		c.current.intensity = .65*c.current.intensity + .35*a.Intensity
		c.current.confidence = a.Confidence
		c.current.expires = min(c.current.activated+30*time.Second, max(c.current.expires, now+time.Duration(a.DurationMS)*time.Millisecond))
		c.pending = nil
		c.idleRequested = false
		return result("merged_without_restart", true)
	}
	cooldown := 6 * time.Second
	if a.Emotion == "happy" {
		cooldown = 20 * time.Second
	}
	if at, ok := c.last[a.Emotion]; ok && now-at < cooldown && source != "manual-action" {
		return result("cooldown", false)
	}
	priority := 20
	if a.Emotion == "happy" {
		priority = 25
	}
	if a.Emotion == "angry" {
		priority = 30
	}
	clip := c.mapping[a.Emotion]
	if clip != "" {
		priority = c.clips[clip].Priority
	}
	duration := time.Duration(a.DurationMS) * time.Millisecond
	if clip != "" {
		v := c.clips[clip]
		if !v.Loop {
			cycle := time.Duration(v.Frames) * v.Step
			for _, bridge := range []string{v.TransitionIn, v.TransitionOut} {
				b := c.clips[bridge]
				cycle += time.Duration(b.Frames) * b.Step
			}
			duration = max(duration, cycle+time.Second)
		}
	}
	r := &request{smooth: true, id: id, confidence: a.Confidence, emotion: a.Emotion, source: source, clip: clip, intensity: a.Intensity, priority: priority, expires: now + min(30*time.Second, max(1200*time.Millisecond, duration))}
	if c.blocked || c.current != nil {
		c.pending = r
		return result("queued_with_inertia", true)
	}
	c.activate(r, now)
	if clip == "" && a.Emotion != "happy" {
		return result("no_clip_idle_fallback", true)
	}
	return result("activated", true)
}

func (c *Controller) Scheduling() Outcome {
	c.mu.Lock()
	defer c.mu.Unlock()
	o := Outcome{Current: "neutral"}
	if c.current != nil {
		o.Current = c.current.emotion
		o.Priority = c.current.priority
	}
	if c.pending != nil {
		o.Pending = c.pending.emotion
	}
	return o
}
