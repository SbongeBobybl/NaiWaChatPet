package emotion

// LocomotionState is the snapshot this track reads. The locomotion owner
// fills it; emotion code must not invent displacement from it.
type LocomotionState struct {
	ActionName     string `json:"action_name"`
	Grounded       bool   `json:"grounded"`
	Facing         int    `json:"facing"`
	BusyLocomotion bool   `json:"busy_locomotion"`
	CanPlayEmotion bool   `json:"can_play_emotion"`
}

// LocomotionSource is implemented by the locomotion teammate.
// GetLocomotionState is the pull API. OnLocomotionChange is the push API.
// Both must observe the same snapshot.
type LocomotionSource interface {
	GetLocomotionState() LocomotionState
	OnLocomotionChange(func(LocomotionState)) (unsubscribe func())
}

// Busy reports jump/attack occupancy. Emotion_in waits until this is false.
func (s LocomotionState) Busy() bool {
	if s.BusyLocomotion {
		return true
	}
	switch s.ActionName {
	case "jump", "attack", "Jump", "Attack":
		return true
	default:
		return false
	}
}

// OverlayAllowed is true for idle, walk, and run once locomotion says
// emotions may play. Jump and attack are never overlay targets.
func (s LocomotionState) OverlayAllowed() bool {
	if s.Busy() || !s.CanPlayEmotion {
		return false
	}
	return true
}
