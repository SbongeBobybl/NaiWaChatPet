package emotion

// ProtectsHover keeps low-priority mouse reactions out of the animation engine
// for both queued and running emotional clips, including their transitions.
func (c *Controller) ProtectsHover() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.current != nil && c.current.emotion != "neutral" || c.pending != nil && c.pending.emotion != "neutral"
}
