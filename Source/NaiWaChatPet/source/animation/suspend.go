package animation

import "time"

// SetSuspended preserves the exact ambient action and digestion timeline while
// another mode owns the character. File-transfer notifications remain queued
// by the host until this mode releases it.
func (e *Engine) SetSuspended(now time.Duration, suspended bool) error {
	if err := e.Advance(now); err != nil {
		return err
	}
	e.suspended = suspended
	return nil
}

// IdleSnapshot selects only existing idle frames for the combat presentation;
// it does not change the suspended ambient state or its pending events.
func (e *Engine) IdleSnapshot() Snapshot {
	c := e.cfg.Clips[Idle]
	return Snapshot{State: Idle, Frame: int(e.now / c.Step % time.Duration(c.Frames))}
}
