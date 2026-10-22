package play

func (in *Input) ClearEdges() {
	in.AttackPressed = false
	in.JumpPressed = false
	in.FanPressed = false
	in.RollPressed = false
	in.PowerPressed = false
	in.DodgePressed = false
}
func (in *Input) MergeEdges(previous Input) {
	in.AttackPressed = in.AttackPressed || previous.AttackPressed
	in.JumpPressed = in.JumpPressed || previous.JumpPressed
	in.FanPressed = in.FanPressed || previous.FanPressed
	in.RollPressed = in.RollPressed || previous.RollPressed
	in.PowerPressed = in.PowerPressed || previous.PowerPressed
	in.DodgePressed = in.DodgePressed || previous.DodgePressed
}

// CanCancel allows a skill after the active part of a normal strike, and
// during its guard/recovery. It never cancels another skill or a hit reaction.
func (c *Controller) CanCancel() bool {
	return !c.down && c.stun <= 0 && (c.act == actNone || c.act == actGuard || c.act == actRise || c.act == actStrike && c.at/c.strikeTime >= chainFrom)
}
func (c *Controller) CancelAction() {
	c.act = actNone
	c.at = 0
	c.combo = 0
	c.buffer = 0
	c.crouch = 0
	c.target = nil
	c.vx = 0
	c.vy = 0
	c.events = nil
}
