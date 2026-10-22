//go:build windows

package main

import (
	"codexpet/reconstruction/animation"
	"codexpet/reconstruction/companion"
	"codexpet/reconstruction/desktopchat"
	"codexpet/reconstruction/emotion"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *desktopHost) initEmotions() {
	dir := animationDirectory
	if dir == "" {
		exe, _ := os.Executable()
		dir = filepath.Join(filepath.Dir(exe), "animations")
	}
	library, warnings := emotion.LoadLibrary(dir, a.assets.Canvas)
	a.emotionLibrary = library
	a.emotions = emotion.New(a.now)
	a.emotions.Register(a.emotionLibrary.Clips())
	for _, e := range warnings {
		_, _ = logDiagnostic(fmt.Errorf("emotion resource rejected: %w", e))
	}
	if companionService != nil {
		present := map[string]bool{}
		for _, c := range a.emotionLibrary.Clips() {
			present[c.Emotion] = true
		}
		var ready, missing []string
		for _, e := range emotion.Catalog {
			switch {
			case !e.Auto:
			case e.ID == "happy" || present[e.ID]:
				ready = append(ready, e.Label)
			default:
				missing = append(missing, e.Label)
			}
		}
		lines := []string{fmt.Sprintf("自动表情 %d 种已就绪：%s", len(ready), strings.Join(ready, " · "))}
		if len(missing) > 0 {
			lines = append(lines, "缺少动画、以待机替代："+strings.Join(missing, " · "))
		}
		lines = append(lines, "开心：强烈时完整捧腹大笑，轻微时微笑", "普通对话：自然待机")
		companionService.SetEmotionStatus(strings.Join(lines, "\n"))
	}
}
func (a *desktopHost) syncEmotions(now time.Duration) {
	if a.emotions == nil {
		return
	}
	if a.combatActive() {
		// Consume ambient requests without playing or replaying stale gestures
		// after combat. Player chat remains available through its normal UI.
		if companionService != nil {
			a.lastEmotionRequest = companionService.EmotionDebug().Model.RequestID
			_, a.lastLocalSequence, _ = companionService.LocalActionDetails()
		}
		a.emotionView = a.emotions.Advance(now, true)
		return
	}
	snap := a.assets.Engine.Snapshot()
	// Expressions wait while the pet is walking, jumping or striking.
	moving := a.play != nil && a.play.ctl.Busy()
	blocked := a.drag || a.pointerDown || moving || snap.DigestPending || snap.State != animation.Idle
	if companionService != nil {
		if !companionService.EmotionEnabled() {
			a.emotions.ReturnToIdle()
		} else {
			d := companionService.EmotionDebug()
			id := d.Model.RequestID
			if id != "" && id != a.lastEmotionRequest {
				if d.Model.Effective.Emotion != "neutral" && (snap.State == animation.Fight || snap.State == animation.ToFight) {
					_, _ = a.assets.Engine.Fire(now, animation.HoverOff)
				}
				o := a.emotions.Apply(id, d.Model.Effective, "deepseek-classifier")
				companionService.ReportEmotionSchedule(d.Sequence, o)
				a.lastEmotionRequest = id
			}
			action, seq, manual := companionService.LocalActionDetails()
			if seq != a.lastLocalSequence {
				if snap.State == animation.Fight || snap.State == animation.ToFight {
					_, _ = a.assets.Engine.Fire(now, animation.HoverOff)
				}
				a.lastLocalSequence = seq
				source := "local-companionship"
				if manual {
					source = "manual-action"
				}
				o := a.emotions.Apply(fmt.Sprintf("local-%d", seq), emotion.Analysis{Emotion: emotion.Canonical(action), Intensity: 1, Confidence: 1, DurationMS: 15000}, source)
				companionService.ReportEmotionSchedule(companionService.EmotionDebug().Sequence, o)
			}
		}
	}
	a.emotionView = a.emotions.Advance(now, blocked)
	if companionService != nil {
		companionService.ReportEmotionState(a.emotions.Scheduling())
	}
	if a.emotions.ConsumeLaugh(blocked) {
		a.queue.laugh = true
	}
	if snap.State == animation.ToLaugh || snap.State == animation.Laugh || snap.State == animation.FromLaugh {
		a.emotionView.Emotion = "laugh"
	}
}

func (a *desktopHost) reportPlayback(snap animation.Snapshot, clip string, frame int, success bool, state string) {
	if companionService == nil {
		return
	}
	label := "neutral"
	if clip != "" {
		label = a.emotionView.Emotion
	} else if snap.State == animation.ToLaugh || snap.State == animation.Laugh || snap.State == animation.FromLaugh {
		label = "happy"
	}
	if clip == "" {
		clip = string(snap.State)
		frame = snap.Frame
	}
	companionService.ReportEmotionPlayback(companion.EmotionPlayback{Emotion: label, Clip: clip, Frame: frame, Success: success, State: state, UpdatedAt: time.Now()})
}
func (a *desktopHost) mouthAnchor() desktopchat.Point {
	m := a.facingMouth()
	if a.play != nil && a.play.flip {
		m.X = a.assets.Canvas.Dx() - m.X
	}
	return m
}

// facingMouth is the mouth on the left-facing art.
func (a *desktopHost) facingMouth() desktopchat.Point {
	if a.emotionView.Clip != "" {
		if m, ok := a.emotionLibrary.Manifest(a.emotionView.Clip); ok {
			if len(m.MouthKeyframes) == 0 {
				return desktopchat.Point{X: int(m.MouthAnchor.X), Y: int(m.MouthAnchor.Y)}
			}
			var keys []desktopchat.Anchor
			for _, v := range m.MouthKeyframes {
				keys = append(keys, desktopchat.Anchor{Frame: v.Frame, X: v.X, Y: v.Y})
			}
			return desktopchat.Interpolate(keys, a.emotionView.Frame)
		}
	}
	s := a.assets.Engine.Snapshot()
	return desktopchat.Mouth(string(s.State), s.Frame, a.assets.Counts[string(s.State)])
}
