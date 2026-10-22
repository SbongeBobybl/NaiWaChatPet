//go:build windows

package main

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

var combatPlaySound = syscall.NewLazyDLL("winmm.dll").NewProc("PlaySoundW")
var combatSounds = map[string][]byte{}

func (a *desktopHost) combatSound(kind string) {
	if a.play == nil || a.play.settings.Game.Volume <= 0 {
		return
	}
	volume := a.play.settings.Game.Volume
	key := fmt.Sprintf("%s/%.2f", kind, volume)
	b := combatSounds[key]
	if b == nil {
		if len(combatSounds) > 32 {
			_, _, _ = combatPlaySound.Call(0, 0, 0)
			clear(combatSounds)
		}
		b = combatWAV(kind, volume)
		combatSounds[key] = b
	}
	_, _, _ = combatPlaySound.Call(uintptr(unsafe.Pointer(&b[0])), 0, 0x0007)
	runtime.KeepAlive(b)
}
