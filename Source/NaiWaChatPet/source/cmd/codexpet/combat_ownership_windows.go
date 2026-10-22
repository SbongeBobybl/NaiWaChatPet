//go:build windows

package main

import (
	"codexpet/reconstruction/animation"
	"time"
)

func (a *desktopHost) combatActive() bool {
	return a.play != nil && a.play.fight != nil && a.play.fight.upgraded != nil
}

func (a *desktopHost) suspendAmbientForCombat(now time.Duration) {
	if err := a.assets.Engine.SetSuspended(now, true); err != nil {
		_, _ = logDiagnostic(err)
	}
	a.queue.laugh = false
	a.hover.inside, a.hoverSent = false, false
	if a.emotions != nil {
		a.emotions.InterruptAnimation("combat owns character")
		a.emotionView = a.emotions.Advance(now, true)
	}
	if companionService != nil {
		companionService.DesktopGameActive(true)
	}
	if a.chat != nil {
		a.chat.HideInput()
		a.chat.hideReply()
	}
}

func (a *desktopHost) resumeAmbientAfterCombat(now time.Duration) {
	if err := a.assets.Engine.SetSuspended(now, false); err != nil {
		_, _ = logDiagnostic(err)
	}
	_, _ = a.assets.Engine.Fire(now, animation.HoverOff)
	if companionService != nil {
		companionService.DesktopGameActive(false)
	}
	a.haveKey = false
}
