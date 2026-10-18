//go:build windows

package main

import (
	"codexpet/reconstruction/companion"
	"codexpet/reconstruction/emotion"
	"codexpet/reconstruction/winlayer"
	"runtime"
	"testing"
	"time"
	"unsafe"
)

func TestAngryModelReachesNativePresentation(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	s, e := companion.Start(nil)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	s.EnableOfflineDemo() // Real JSON/stream client pipeline, no external API/key.
	previous := companionService
	companionService = s
	defer func() { companionService = previous }()
	if _, e = s.Submit("奶蛙，表演生气一下"); e != nil {
		t.Fatal(e)
	}
	until := time.Now().Add(3 * time.Second)
	for s.View().Stage != "completed" && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	if d := s.EmotionDebug(); d.Model.Effective.Emotion != "angry" || d.Model.State != "validated" {
		t.Fatal("angry model result not accepted", d)
	}
	assets, e := loadAssets(bundledAssets, "runtime.json")
	if e != nil {
		t.Fatal(e)
	}
	l, warnings := emotion.LoadLibrary("../../../animations", assets.Canvas)
	if len(warnings) > 0 {
		t.Fatal(warnings)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	class, title := utf("STATIC"), utf("NaiWaChatPet angry rendering regression")
	h, e := call(createWindow, wsExLayered|wsExToolWindow|0x08000000, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)), wsPopup, 0, 0, 420, 560, 0, 0, 0, 0)
	runtime.KeepAlive(class)
	runtime.KeepAlive(title)
	if h == 0 {
		t.Fatal(e)
	}
	defer call(destroyWindow, h)
	surface, e := winlayer.New(420, 560)
	if e != nil {
		t.Fatal(e)
	}
	defer surface.Close()
	var now time.Duration
	c := emotion.New(func() time.Duration { return now })
	c.Register(l.Clips())
	a := desktopHost{assets: assets, hwnd: h, surface: surface, emotions: c, emotionLibrary: l, w: 420, h: 560, x: -5000, y: -5000}
	seen := map[string]map[int]bool{}
	for now = 0; now < 10*time.Second; now += time.Millisecond {
		a.syncEmotions(now)
		if a.emotionView.Clip == "" {
			break
		}
		if !c.ProtectsHover() {
			t.Fatal("running emotion lost hover protection")
		}
		if e = a.render(false); e != nil {
			t.Fatal(e)
		}
		p := s.EmotionDebug().Playback
		if !p.Success || p.State != "presented" {
			t.Fatal("actual native frame not presented", p)
		}
		if seen[p.Clip] == nil {
			seen[p.Clip] = map[int]bool{}
		}
		seen[p.Clip][p.Frame] = true
	}
	for _, clip := range []string{"angry_in", "angry", "angry_out"} {
		m, _ := l.Manifest(clip)
		if len(seen[clip]) != m.FrameCount {
			t.Fatalf("native %s: %d/%d frames", clip, len(seen[clip]), m.FrameCount)
		}
	}
	t.Log("Validated angry model JSON -> independent natural chat -> controller -> all 93 native UpdateLayeredWindow frames")
}
