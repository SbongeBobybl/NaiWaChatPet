//go:build windows

package main

import (
	"codexpet/reconstruction/play"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"time"
	"unsafe"
)

type combatProbe struct {
	online                    *onlineCombatProbe
	SlowFrames                int
	PendingRefreshFrames      int
	UnclaimedFrames           int
	ClockFrames               int
	LobbyPage                 int
	SpriteCounts              []int
	ActiveFrames              int
	ActiveSeconds             float64
	SampledAt                 time.Time
	CPUFile                   *os.File
	Started                   time.Time
	Frames                    int
	RenderMS                  []float64
	Cycles                    int
	Ticks                     uint64
	Events                    map[string]int
	BaselineGDI, BaselineUSER uintptr
	GDIByCycle, USERByCycle   []uintptr
	lastTick                  uint64
	finished                  bool
}

var combatProbeData combatProbe
var combatGUIResources = userDLL.NewProc("GetGuiResources")

func ownedSpriteCount(a *desktopHost) int {
	find := userDLL.NewProc("FindWindowExW")
	pidProc := userDLL.NewProc("GetWindowThreadProcessId")
	count := 0
	previous := uintptr(0)
	for {
		hwnd, _, _ := find.Call(0, previous, uintptr(unsafe.Pointer(a.play.class.name)), 0)
		if hwnd == 0 {
			break
		}
		previous = hwnd
		var pid uint32
		_, _, _ = pidProc.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if int(pid) == os.Getpid() {
			count++
		}
	}
	return count
}

func playProbeBrain() *play.Brain {
	return play.NewBrain(17, play.Rival{Speed: 1, Aggression: .8, Agility: .3})
}
func guiResources() (uintptr, uintptr) {
	gdi, _, _ := combatGUIResources.Call(^uintptr(0), 0)
	user, _, _ := combatGUIResources.Call(^uintptr(0), 1)
	return gdi, user
}
func recordCombatPresent(a *desktopHost, start time.Time) {
	p := &combatProbeData
	if p.Started.IsZero() || p.finished || a.play == nil || a.play.fight == nil {
		return
	}
	p.Frames++
	if clock := a.play.fight.upgraded.clock; p.online != nil && clock != nil && clock.rendering {
		p.ClockFrames++
		// Deliberately overrun the refresh interval to exercise the starvation
		// fix. This delay exists only inside the explicit isolated GUI probe.
		if p.Frames%120 == 0 {
			time.Sleep(35 * time.Millisecond)
			p.SlowFrames++
		}
		if !clock.queued.Load() {
			p.UnclaimedFrames++
		}
		var message messageWin
		if pending, _ := call(userDLL.NewProc("PeekMessageW"), uintptr(unsafe.Pointer(&message)), a.hwnd, wmTimer, wmTimer, 0); pending != 0 {
			p.PendingRefreshFrames++
		}
	}
	if !a.play.view.Still {
		p.ActiveFrames++
	}
	p.RenderMS = append(p.RenderMS, float64(time.Since(start))/float64(time.Millisecond))
}
func (a *desktopHost) stepCombatProbe(now time.Duration) {
	p := &combatProbeData
	if p.finished {
		return
	}
	if p.Started.IsZero() {
		if a.play.fight == nil {
			return
		}
		if combatOnlineSmoke {
			var err error
			p.online, err = a.startOnlineCombatProbe()
			if err != nil {
				a.stop(err)
				return
			}
		}
		p.Started = time.Now()
		p.SampledAt = p.Started
		if file, err := os.Create(combatSmokeReport + ".cpu.pprof"); err == nil {
			p.CPUFile = file
			_ = pprof.StartCPUProfile(file)
		}
		p.Cycles = 1
		p.Events = map[string]int{}
		p.BaselineGDI, p.BaselineUSER = guiResources()
		p.lastTick = a.play.fight.upgraded.snapshot.Tick
		if combatLobbySmoke {
			a.openGamePanel()
		}
	}
	if a.play.fight != nil {
		if p.online != nil {
			a.sampleOnlineCombatProbe(p.online)
		}
		if !a.play.view.Still {
			p.ActiveSeconds += time.Since(p.SampledAt).Seconds()
		}
		p.SampledAt = time.Now()
		s := a.play.fight.upgraded.snapshot
		if s.Tick >= p.lastTick {
			p.Ticks += s.Tick - p.lastTick
		}
		p.lastTick = s.Tick
		for _, e := range s.Events {
			key := e.Kind
			if e.Tick == s.Tick {
				p.Events[key]++
			}
		}
	}
	elapsed := time.Since(p.Started).Seconds()
	if combatLobbySmoke && a.play.gamePanel != nil {
		page := int(elapsed) % 3
		if page != p.LobbyPage {
			p.LobbyPage = page
			a.play.gamePanel.switchLobbyPage(page)
		}
	}
	target := float64(p.Cycles) * combatSmokeSeconds / 3
	if p.online != nil {
		target = combatSmokeSeconds
	}
	if elapsed < target {
		return
	}
	a.endFight()
	a.setControl(false)
	gdi, user := guiResources()
	p.GDIByCycle = append(p.GDIByCycle, gdi)
	p.USERByCycle = append(p.USERByCycle, user)
	p.SpriteCounts = append(p.SpriteCounts, ownedSpriteCount(a))
	if p.Cycles < 3 && p.online == nil {
		p.Cycles++
		p.lastTick = 0
		a.startFight()
		if a.play.fight != nil {
			a.play.fight.auto = playProbeBrain()
		}
		if combatLobbySmoke {
			a.openGamePanel()
		}
		return
	}
	p.finished = true
	if p.CPUFile != nil {
		pprof.StopCPUProfile()
		p.CPUFile.Close()
	}
	sort.Float64s(p.RenderMS)
	percentile := func(v float64) float64 {
		if len(p.RenderMS) == 0 {
			return 0
		}
		return p.RenderMS[min(len(p.RenderMS)-1, int(float64(len(p.RenderMS))*v))]
	}
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	heapBeforeGC := memory.HeapAlloc
	runtime.GC()
	runtime.ReadMemStats(&memory)
	report := map[string]any{"kind": "actual Windows layered-window GUI smoke; visual/manual acceptance separate", "durationSeconds": elapsed, "presentedFrames": p.Frames, "presentationFPS": float64(p.Frames) / elapsed, "simulationHz": float64(p.Ticks) / elapsed, "renderMedianMS": percentile(.5), "renderP95MS": percentile(.95), "cycles": p.Cycles, "baselineGDI": p.BaselineGDI, "baselineUSER": p.BaselineUSER, "gdiAfterExit": p.GDIByCycle, "userAfterExit": p.USERByCycle, "heapBytes": memory.HeapAlloc, "eventsAtSampledTicks": p.Events, "crossMachineVerified": false, "publicDeploymentVerified": false}
	report["activePresentationFrames"] = p.ActiveFrames
	report["gameBuild"] = play.GameBuild
	if executable, err := os.Executable(); err == nil {
		if b, err := os.ReadFile(executable); err == nil {
			report["clientSHA256"] = fmt.Sprintf("%x", sha256.Sum256(b))
		}
	}
	report["ownedSpritesAfterExit"] = p.SpriteCounts
	report["activePresentationSeconds"] = p.ActiveSeconds
	if p.ActiveSeconds > 0 {
		report["activePresentationFPS"] = float64(p.ActiveFrames) / p.ActiveSeconds
	}
	report["heapBeforeGCBytes"] = heapBeforeGC
	report["heapBytesAfterGC"] = memory.HeapAlloc
	if combatLobbySmoke {
		report["kind"] = "actual Windows EXE; native lobby pages and three game/lobby lifecycle cycles"
	}
	if p.online != nil {
		p.online.close()
		report["kind"] = "actual Windows GUI + two TCP clients; continuous online combat and automatic rematches"
		report["onlineSamples"] = p.online.samples
		report["completedOnlineMatches"] = p.online.matches
		report["injectedSlowFrames"] = p.SlowFrames
		report["framesWithQueuedRefreshDuringRender"] = p.PendingRefreshFrames
		report["framesWithReleasedClockDuringRender"] = p.UnclaimedFrames
		report["framesPresentedFromCombatClock"] = p.ClockFrames
	}
	b, _ := json.MarshalIndent(report, "", "  ")
	_ = os.MkdirAll(filepath.Dir(combatSmokeReport), 0700)
	if err := os.WriteFile(combatSmokeReport, b, 0600); err != nil {
		_, _ = logDiagnostic(err)
	}
	a.stop(nil)
}
