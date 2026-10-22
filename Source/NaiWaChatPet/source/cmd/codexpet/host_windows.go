//go:build windows

package main

import (
	"codexpet/reconstruction/companion"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"syscall"
	"time"
	"unsafe"

	"codexpet/reconstruction/animation"
	"codexpet/reconstruction/compositor"
	"codexpet/reconstruction/emotion"
	"codexpet/reconstruction/interaction"
	"codexpet/reconstruction/winlayer"
)

const (
	wmDestroy         = 0x0002
	wmPaint           = 0x000f
	wmClose           = 0x0010
	wmEraseBackground = 0x0014
	wmNCHitTest       = 0x0084
	wmTimer           = 0x0113
	wmMouseMove       = 0x0200
	wmLeftDown        = 0x0201
	wmLeftUp          = 0x0202
	wmDoubleClick     = 0x0203
	wmRightUp         = 0x0205
	wmMouseWheel      = 0x020a
	wmCaptureChanged  = 0x0215
	wmDropFiles       = 0x0233
	wmDPIChanged      = 0x02e0
	wsPopup           = 0x80000000
	wsExLayered       = 0x00080000
	wsExTopmost       = 0x00000008
	wsExToolWindow    = 0x00000080
)

var (
	userDLL          = syscall.NewLazyDLL("user32.dll")
	kernelDLL        = syscall.NewLazyDLL("kernel32.dll")
	shellDLL         = syscall.NewLazyDLL("shell32.dll")
	registerClass    = userDLL.NewProc("RegisterClassExW")
	unregisterClass  = userDLL.NewProc("UnregisterClassW")
	createWindow     = userDLL.NewProc("CreateWindowExW")
	destroyWindow    = userDLL.NewProc("DestroyWindow")
	defWindowProc    = userDLL.NewProc("DefWindowProcW")
	getMessage       = userDLL.NewProc("GetMessageW")
	translateMessage = userDLL.NewProc("TranslateMessage")
	dispatchMessage  = userDLL.NewProc("DispatchMessageW")
	postQuit         = userDLL.NewProc("PostQuitMessage")
	postMessage      = userDLL.NewProc("PostMessageW")
	setTimer         = userDLL.NewProc("SetTimer")
	killTimer        = userDLL.NewProc("KillTimer")
	showWindow       = userDLL.NewProc("ShowWindow")
	setWindowPos     = userDLL.NewProc("SetWindowPos")
	getSystemMetrics = userDLL.NewProc("GetSystemMetrics")
	getCursorPos     = userDLL.NewProc("GetCursorPos")
	setCapture       = userDLL.NewProc("SetCapture")
	getCapture       = userDLL.NewProc("GetCapture")
	releaseCapture   = userDLL.NewProc("ReleaseCapture")
	createMenu       = userDLL.NewProc("CreatePopupMenu")
	destroyMenu      = userDLL.NewProc("DestroyMenu")
	appendMenu       = userDLL.NewProc("AppendMenuW")
	trackMenu        = userDLL.NewProc("TrackPopupMenu")
	foregroundWindow = userDLL.NewProc("SetForegroundWindow")
	messageBox       = userDLL.NewProc("MessageBoxW")
	loadCursor       = userDLL.NewProc("LoadCursorW")
	validateRect     = userDLL.NewProc("ValidateRect")
	dpiContext       = userDLL.NewProc("SetProcessDpiAwarenessContext")
	dpiAware         = userDLL.NewProc("SetProcessDPIAware")
	getDPI           = userDLL.NewProc("GetDpiForWindow")
	dragAccept       = shellDLL.NewProc("DragAcceptFiles")
	dragQuery        = shellDLL.NewProc("DragQueryFileW")
	dragFinish       = shellDLL.NewProc("DragFinish")
	moduleHandle     = kernelDLL.NewProc("GetModuleHandleW")
	shellExecute     = shellDLL.NewProc("ShellExecuteW")
	clearLastError   = kernelDLL.NewProc("SetLastError")
	activeHost       *desktopHost
)

type pointWin struct{ X, Y int32 }
type rectWin struct{ Left, Top, Right, Bottom int32 }
type classWin struct {
	Size, Style                        uint32
	Procedure                          uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	SmallIcon                          uintptr
}
type messageWin struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Point          pointWin
	Private        uint32
}

func call(proc *syscall.LazyProc, args ...uintptr) (uintptr, error) {
	if err := proc.Find(); err != nil {
		return 0, err
	}
	if err := clearLastError.Find(); err != nil {
		return 0, err
	}
	clearLastError.Call(0)
	r, _, err := proc.Call(args...)
	return r, err
}
func failure(op string, err error) error {
	if err == nil || err == syscall.Errno(0) {
		err = errors.New("API returned failure without a Win32 error code")
	}
	return fmt.Errorf("%s: %w", op, err)
}
func cursor() (pointWin, error) {
	var p pointWin
	r, err := call(getCursorPos, uintptr(unsafe.Pointer(&p)))
	runtime.KeepAlive(&p)
	if r == 0 {
		return p, failure("GetCursorPos", err)
	}
	return p, nil
}
func nativeMessage(hwnd uintptr, title, text string, flags uintptr) {
	if runes := []rune(text); len(runes) > 1500 {
		text = string(runes[:1500]) + "\n\u7701\u7565\u90e8\u5206\u6587\u4ef6\u8def\u5f84"
	}
	tp, _ := syscall.UTF16PtrFromString(title)
	body, _ := syscall.UTF16PtrFromString(text)
	if tp == nil || body == nil {
		fmt.Fprintln(os.Stderr, text)
		return
	}
	_, _ = call(messageBox, hwnd, uintptr(unsafe.Pointer(body)), uintptr(unsafe.Pointer(tp)), flags)
	runtime.KeepAlive(tp)
	runtime.KeepAlive(body)
}
func showFailure(err error) {
	path, logErr := logDiagnostic(err)
	message := startupHint(err)
	if logErr == nil {
		message += "\n\u8be6\u7ec6\u4fe1\u606f\u5df2\u4fdd\u5b58\u5230\uff1a\n" + path
	} else {
		message += "\n\u8be6\u7ec6\u9519\u8bef\u65e0\u6cd5\u5199\u5165\u8bca\u65ad\u65e5\u5fd7\u3002"
	}
	nativeMessage(0, "Codex Pet \u542f\u52a8\u5931\u8d25", message, 0x10)
}

type renderKey struct {
	emotionClip  string
	emotionFrame int
	state        animation.State
	frame, w, h  int
	drop         bool
	watching     bool
	age          time.Duration
	flip         bool
	motion       uint64
}
type desktopHost struct {
	play    *playState
	tidying *tidyJob
	trip    *tidyTrip
	// jolt shakes the window sideways while a blow holds time still.
	jolt                                            int
	lastReportedEmotionClip                         string
	tray                                            *nativeTray
	assets                                          *loadedAssets
	options                                         hostOptions
	hwnd                                            uintptr
	surface                                         *winlayer.Surface
	workers                                         *hostWorkers
	epoch                                           time.Time
	scale                                           float64
	dpi                                             uint32
	x, y, w, h                                      int
	drag                                            bool
	pointerDown                                     bool
	gesture                                         interaction.Gesture
	chat                                            *nativeChat
	emotions                                        *emotion.Controller
	emotionLibrary                                  *emotion.Library
	emotionView                                     emotion.Snapshot
	lastEmotionRequest, lastEmotionStage            string
	lastLocalSequence                               uint64
	startCursor                                     pointWin
	startX, startY                                  int
	hover                                           hoverHold
	hoverSent                                       bool
	contacts                                        contactLatch
	peers                                           []rectJSON
	queue                                           actionQueue
	key                                             renderKey
	haveKey                                         bool
	presented                                       bool
	lastPresentedX, lastPresentedY                  int
	bgra                                            []byte
	currentBody                                     *image.NRGBA
	timerReady, closing, menuOpen, peerProblemShown bool
	terminalError                                   error
}

func (a *desktopHost) now() time.Duration { return time.Since(a.epoch) }
func (a *desktopHost) stop(err error) {
	if err != nil {
		a.terminalError = errors.Join(a.terminalError, err)
	}
	if a.closing {
		return
	}
	a.closing = true
	if a.workers != nil {
		a.workers.cancel()
	}
	if a.hwnd != 0 {
		if r, e := call(postMessage, a.hwnd, wmClose, 0, 0); r == 0 {
			a.terminalError = errors.Join(a.terminalError, failure("PostMessage(WM_CLOSE)", e))
			_, _ = call(destroyWindow, a.hwnd)
			_, _ = call(postQuit, 1)
		}
	}
}
func runHost(assets *loadedAssets, options hostOptions) (err error) {
	traceStartup("runHost: before LockOSThread")
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if activeHost != nil {
		return errors.New("only one desktop host is allowed per process")
	}
	// A per-monitor-aware process uses physical desktop coordinates. One DPI factor
	// is applied to the complete canvas, including prop and shadow.
	if dpiContext.Find() == nil {
		if r, _ := call(dpiContext, ^uintptr(3)); r == 0 {
			_, _ = call(dpiAware)
		}
	} else {
		_, _ = call(dpiAware)
	}
	traceStartup("runHost: querying GetModuleHandleW")
	module, e := call(moduleHandle, 0)
	if module == 0 {
		return failure("GetModuleHandleW", e)
	}
	name, e := syscall.UTF16PtrFromString("CodexPetReconstruction_" + strconv.Itoa(os.Getpid()))
	if e != nil {
		return e
	}
	cursorHandle, _ := call(loadCursor, 0, 32512)
	callback := syscall.NewCallback(windowProc)
	klass := classWin{Size: uint32(unsafe.Sizeof(classWin{})), Style: 0x0008, Procedure: callback, Instance: module, Cursor: cursorHandle, ClassName: name}
	klass.Icon, _ = call(userDLL.NewProc("LoadIconW"), module, 1)
	klass.SmallIcon = klass.Icon
	traceStartup("runHost: registering window class")
	atom, e := call(registerClass, uintptr(unsafe.Pointer(&klass)))
	runtime.KeepAlive(&klass)
	runtime.KeepAlive(name)
	if atom == 0 {
		return failure("RegisterClassExW", e)
	}
	defer func() {
		r, ce := call(unregisterClass, uintptr(unsafe.Pointer(name)), module)
		runtime.KeepAlive(name)
		if r == 0 {
			err = errors.Join(err, failure("UnregisterClassW", ce))
		}
	}()
	traceStartup("runHost: window class registered")
	a := &desktopHost{assets: assets, options: options, scale: assets.Config.Scale.Default, dpi: 96, epoch: time.Now()}
	doubleTime, _ := call(chatDoubleTime)
	dragX, _ := call(getSystemMetrics, 68)
	dragY, _ := call(getSystemMetrics, 69)
	a.gesture = interaction.Gesture{Interval: time.Duration(doubleTime) * time.Millisecond, ThresholdX: max(4, int(dragX)), ThresholdY: max(4, int(dragY))}
	a.w, a.h, e = scaledCanvas(assets.Canvas, a.scale)
	if e != nil {
		return e
	}
	sx, _ := call(getSystemMetrics, 0)
	sy, _ := call(getSystemMetrics, 1)
	a.x = max(0, int(sx)-a.w-80)
	a.y = max(0, int(sy)-a.h-80)
	activeHost = a
	defer func() { activeHost = nil }()
	title, _ := syscall.UTF16PtrFromString("NaiWaChatPet")
	exStyle := uintptr(wsExLayered | wsExTopmost | wsExToolWindow)
	if startMode == "demo" {
		exStyle = wsExLayered | wsExTopmost | 0x00040000
		title = utf("NaiWaChatPet 离线验收")
	}
	traceStartup("runHost: creating layered window")
	hwnd, e := call(createWindow, exStyle, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(title)), wsPopup, uintptr(a.x), uintptr(a.y), uintptr(a.w), uintptr(a.h), 0, 0, module, 0)
	runtime.KeepAlive(name)
	runtime.KeepAlive(title)
	if hwnd == 0 {
		return failure("CreateWindowExW", e)
	}
	traceStartup("runHost: CreateWindowExW returned success")
	a.hwnd = hwnd
	a.initEmotions()
	a.initPlay(module)
	defer func() {
		a.play.close(hwnd)
		if a.tray != nil {
			a.tray.Close()
		}
		if a.chat != nil {
			a.chat.Close()
		}
		if a.workers != nil {
			a.workers.close()
		}
		if a.surface != nil {
			err = errors.Join(err, a.surface.Close())
		}
		if !a.closing {
			_, _ = call(destroyWindow, hwnd)
		}
		err = errors.Join(err, a.terminalError)
	}()
	if getDPI.Find() == nil {
		if d, _ := call(getDPI, hwnd); d >= 48 && d <= 768 {
			a.dpi = uint32(d)
			a.w, a.h, e = scaledCanvas(assets.Canvas, a.scale*float64(a.dpi)/96)
			if e != nil {
				return e
			}
		}
	}
	if a.dpi != 96 {
		factor := a.scale * float64(a.dpi) / 96
		sx, _ = call(getSystemMetrics, 0)
		sy, _ = call(getSystemMetrics, 1)
		a.x = max(0, int(sx)-a.w-80)
		a.y = max(0, int(sy)-int(math.Round(assets.Config.Drop.GroundY*factor))-80)
		if r, ce := call(setWindowPos, hwnd, 0, uintptr(a.x), uintptr(a.y), uintptr(a.w), uintptr(a.h), 0x0004|0x0010); r == 0 {
			return failure("SetWindowPos(initial DPI size)", ce)
		}
	}
	traceStartup("runHost: allocating GDI DIB surface")
	a.reserveInitialChatSpace()
	a.surface, e = winlayer.New(a.w, a.h)
	if e != nil {
		if a.surface != nil {
			e = errors.Join(e, a.surface.Close())
		}
		return e
	}
	traceStartup("runHost: surface ready, starting workers")
	a.workers, e = newWorkers(options)
	if e != nil {
		return e
	}
	replaceLatest(a.workers.poses, a.rectangle())
	traceStartup("runHost: before first layered-window Present")
	if e = a.render(true); e != nil {
		return e
	}
	traceStartup("runHost: first frame presented successfully")
	if tray, trayErr := a.initTray(); trayErr == nil {
		a.tray = tray
	} else {
		_, _ = logDiagnostic(trayErr)
	}
	if chat, chatErr := newNativeChat(a); chatErr != nil {
		_, _ = logDiagnostic(fmt.Errorf("optional native chat unavailable: %w", chatErr))
	} else {
		a.chat = chat
	}
	_, _ = call(dragAccept, hwnd, 1)
	_, _ = call(showWindow, hwnd, 4)
	if r, ce := call(setWindowPos, hwnd, ^uintptr(0), 0, 0, 0, 0, 0x0001|0x0002|0x0010|0x0040); r == 0 {
		return failure("SetWindowPos(topmost)", ce)
	}
	traceStartup("runHost: window shown, setting animation timer")
	timer, e := call(setTimer, hwnd, 1, 16, 0)
	if timer == 0 {
		return failure("SetTimer", e)
	}
	a.timerReady = true
	traceStartup("runHost: timer installed, entering message loop")
	var message messageWin
	for {
		r, ce := call(getMessage, uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		runtime.KeepAlive(&message)
		if int32(r) == -1 {
			return failure("GetMessageW", ce)
		}
		if r == 0 {
			break
		}
		if a.play != nil && a.play.gamePanel != nil && a.play.gamePanel.lobbyMessage(&message) {
			continue
		}
		if a.chat != nil && a.chat.tab(message) {
			continue
		}
		_, _ = call(translateMessage, uintptr(unsafe.Pointer(&message)))
		_, _ = call(dispatchMessage, uintptr(unsafe.Pointer(&message)))
		runtime.KeepAlive(&message)
	}
	traceStartup("runHost: message loop exited normally")
	return nil
}
func (a *desktopHost) rectangle() rectJSON { return rectJSON{a.x, a.y, a.w, a.h} }
func (a *desktopHost) tick() error {
	now := a.now()
	if companionService != nil && companionService.Sharing().Allowed {
		rememberForeground()
	}
	if err := a.assets.Engine.Advance(now); err != nil {
		return err
	}
	for {
		select {
		case report := <-a.workers.results:
			if report.Stored > 0 {
				a.queue.stored()
			}
			if report.SourceRetained > 0 {
				_, _ = logDiagnostic(fmt.Errorf("file feed copied %d file(s) with alternate data streams; verified destination, original preserved", report.SourceRetained))
			}
			if len(report.Problems) > 0 {
				for _, problem := range report.Problems {
					_, _ = logDiagnostic(fmt.Errorf("file transfer stage=%s source=%q target=%q target_verified=%t source_removed=%t source_removal_uncertain=%t: %s", problem.Stage, problem.Source, problem.Target, problem.TargetVerified, problem.SourceRemoved, problem.SourceRemovalUncertain, problem.Detail))
				}
				nativeMessage(a.hwnd, "\u6587\u4ef6\u5904\u7406\u7ed3\u679c", transferProblemsText(report.Problems), 0x30)
			}
		case peers := <-a.workers.peers:
			a.peers = peers
		case problem := <-a.workers.problems:
			fmt.Fprintln(os.Stderr, problem)
			if !a.peerProblemShown {
				a.peerProblemShown = true
				nativeMessage(a.hwnd, "同伴互动暂不可用", problem.Error(), 0x30)
			}
		default:
			goto drained
		}
	}
drained:
	// Modal error UI may have consumed time. Resume from the current monotonic time.
	now = a.now()
	if err := a.assets.Engine.Advance(now); err != nil {
		return err
	}
	if a.contacts.update(a.rectangle(), a.peers) && !a.combatActive() {
		if _, err := a.queue.requestLaugh(a.assets.Engine, now); err != nil {
			return err
		}
	}
	snap := a.assets.Engine.Snapshot()
	protected := a.emotions != nil && a.emotions.ProtectsHover()
	if companionService != nil && companionService.EmotionEnabled() {
		d := companionService.EmotionDebug()
		protected = protected || d.Model.RequestID != "" && d.Model.RequestID != a.lastEmotionRequest && d.Model.Effective.Emotion != "neutral"
	}
	// A pet that is being steered, or is walking somewhere, does not stop to
	// spar with the pointer.
	steering := a.play != nil && (a.play.control || a.play.ctl.Busy())
	eligible := !protected && !steering && !a.drag && !a.pointerDown && !snap.DigestPending && a.queue.storedBatches == 0 && !a.queue.laugh && (snap.State == animation.Idle || snap.State == animation.Fight || snap.State == animation.ToFight || snap.State == animation.FromFight)
	hit := false
	// Windows refuses the pointer position while the lock screen or a UAC
	// prompt has the desktop. No pointer means no hover; it is not a reason to
	// shut the pet down.
	if eligible {
		if p, err := cursor(); err == nil {
			mask := a.assets.Mask
			if a.assets.Config.HoverMaskMode == "body" && a.currentBody != nil {
				mask = a.currentBody
			}
			hit = maskHit(mask, int(p.X)-a.x, int(p.Y)-a.y, a.w, a.h)
		}
	}
	inside := a.hover.update(now, hit)
	if !eligible {
		a.hover.inside = false
		inside = false
	}
	if inside != a.hoverSent {
		input := animation.HoverOff
		if inside {
			input = animation.HoverOn
		}
		if _, err := a.assets.Engine.Fire(now, input); err != nil {
			return err
		}
		a.hoverSent = inside
	}
	if !a.combatActive() {
		if _, err := a.queue.service(a.assets.Engine, now); err != nil {
			return err
		}
	}
	a.assets.Engine.DrainEvents()
	a.stepPlay(now)
	a.stepTidy(now)
	a.stepOrganizer()
	a.stepTrip(now)
	a.syncEmotions(now)
	if a.play != nil && a.play.gamePanel != nil {
		a.play.gamePanel.refresh()
	}
	replaceLatest(a.workers.poses, a.rectangle())
	if a.chat != nil {
		// While steering, a click on the pet only hands it the keyboard back.
		if a.gesture.Poll(now) && !a.drag && !a.menuOpen && (a.play == nil || !a.play.control) {
			if chatErr := a.chat.ShowInput(); chatErr != nil {
				_, _ = logDiagnostic(chatErr)
			}
		}
		if chatErr := a.chat.Tick(); chatErr != nil {
			_, _ = logDiagnostic(chatErr)
			a.chat.hideReply()
		}
	}
	return a.render(false)
}
func (a *desktopHost) render(force bool) error {
	probeRenderStart := time.Now()
	if a.surface == nil {
		return nil
	}
	snap := a.assets.Engine.Snapshot()
	if a.combatActive() {
		snap = a.assets.Engine.IdleSnapshot()
	}
	watching := companionService != nil && companionService.ScreenActive()
	key := renderKey{state: snap.State, frame: snap.Frame, w: a.w, h: a.h, drop: snap.DropVisible, watching: watching}
	key.emotionClip = a.emotionView.Clip
	key.emotionFrame = a.emotionView.Frame
	key.flip, key.motion = a.motionKey()
	if snap.DropVisible {
		key.age = snap.DropAge
	}
	changed := force || !a.haveKey || key != a.key
	playedClip := ""
	if !changed {
		playedClip = a.key.emotionClip
	}
	if changed {
		body, err := a.assets.Bodies.BodyCopy(string(snap.State), snap.Frame)
		if err != nil {
			return err
		}
		if !a.combatActive() && a.emotionView.Clip != "" && !a.emotionView.Blocked {
			if custom, customErr := a.emotionLibrary.Frame(a.emotionView.Clip, a.emotionView.Frame); customErr == nil {
				body = custom
				playedClip = a.emotionView.Clip
			} else {
				a.emotions.InterruptAnimation("resource failure")
				_, _ = logDiagnostic(customErr)
			}
		}
		// Walking, jumping, striking and held poses are the same frames bent
		// by a rig; everything else is shown as it was rendered.
		if posed, rigErr := a.playFrame(snap, body, playedClip); rigErr != nil {
			return rigErr
		} else if posed != nil {
			body = posed
		}
		if a.play != nil && a.play.flash > 0 {
			whiten(body, a.play.flash/.15*.85)
		}
		a.currentBody = body
		var prop, shadow *compositor.Layer
		if snap.DropVisible {
			pose, err := animation.DropAt(a.assets.Config.Drop.config(), snap.DropAge)
			if err != nil {
				return err
			}
			if pose.Reveal > 0 {
				pw := max(1, int(math.Round(float64(a.assets.Prop.Bounds().Dx())*pose.ScaleX)))
				ph := max(1, int(math.Round(float64(a.assets.Prop.Bounds().Dy())*pose.ScaleY)))
				sprite, err := compositor.Resize(a.assets.Prop, pw, ph)
				if err != nil {
					return err
				}
				revealed := max(1, min(ph, int(math.Ceil(float64(ph)*pose.Reveal))))
				visible := sprite.SubImage(image.Rect(0, 0, pw, revealed))
				prop = &compositor.Layer{Image: visible, At: image.Pt(int(math.Round(pose.X-float64(pw)/2)), int(math.Round(pose.BottomY))-revealed)}
			}
			sw := max(2, int(math.Round(float64(a.assets.Prop.Bounds().Dx())*1.15*pose.ShadowScale)))
			sh := max(2, int(math.Round(7*pose.ShadowScale)))
			shadow = &compositor.Layer{Image: ellipseShadow(sw, sh, pose.ShadowOpacity), At: image.Pt(int(math.Round(pose.X))-sw/2, int(math.Round(a.assets.Config.Drop.GroundY))-sh/2)}
		}
		composite := compositor.Compose(body.Bounds(), compositor.Layer{Image: body, At: image.Point{}}, prop, shadow)
		if watching { // Always-visible dot on the pet while periodic screen sharing is active.
			for dy := -7; dy <= 7; dy++ {
				for dx := -7; dx <= 7; dx++ {
					if dx*dx+dy*dy <= 49 {
						px, py := composite.Bounds().Dx()-max(12, 32*composite.Bounds().Dx()/420)+dx, max(12, 45*composite.Bounds().Dy()/560)+dy
						if image.Pt(px, py).In(composite.Bounds()) {
							composite.SetRGBA(px, py, color.RGBA{R: 207, G: 74, B: 49, A: 255})
						}
					}
				}
			}
		}
		if key.flip {
			// The art faces left. Turn the finished canvas, and the body used
			// for hit tests with it, when the pet faces right.
			mirror(composite.Pix, composite.Stride, composite.Rect.Dx(), composite.Rect.Dy())
			mirror(body.Pix, body.Stride, body.Rect.Dx(), body.Rect.Dy())
		}
		resized, err := compositor.Resize(composite, a.w, a.h)
		if err != nil {
			return err
		}
		a.bgra = compositor.BGRA(resized)
		a.key = key
		a.haveKey = true
	}
	if !changed && a.presented && a.lastPresentedX == a.x+a.jolt && a.lastPresentedY == a.y {
		return nil
	}
	if err := a.surface.Present(a.hwnd, a.x+a.jolt, a.y, a.bgra); err != nil {
		a.reportPlayback(snap, playedClip, a.emotionView.Frame, false, "present_failed")
		return err
	}
	if combatSmokeReport != "" {
		recordCombatPresent(a, probeRenderStart)
	}
	a.reportPlayback(snap, playedClip, a.emotionView.Frame, true, "presented")
	actualClip := playedClip
	if actualClip == "" {
		actualClip = string(snap.State)
	}
	if actualClip != a.lastReportedEmotionClip {
		traceStartup("emotion presented: " + actualClip)
		a.lastReportedEmotionClip = actualClip
	}
	a.presented = true
	a.lastPresentedX = a.x + a.jolt
	a.lastPresentedY = a.y
	if a.chat != nil {
		if e := a.chat.position(); e != nil {
			_, _ = logDiagnostic(e)
		}
	}
	return nil
}
func ellipseShadow(width, height int, opacity float64) *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			dx := (float64(x)+.5)/float64(width)*2 - 1
			dy := (float64(y)+.5)/float64(height)*2 - 1
			distance := dx*dx + dy*dy
			if distance < 1 {
				alpha := uint8(math.Round(255 * math.Max(0, math.Min(1, opacity)) * (1 - distance)))
				im.SetNRGBA(x, y, color.NRGBA{A: alpha})
			}
		}
	}
	return im
}
func (a *desktopHost) resize(scale float64, dpi uint32) error {
	s := a.assets.Config.Scale
	scale = math.Max(s.Min, math.Min(s.Max, scale))
	if dpi < 48 || dpi > 768 {
		return errors.New("invalid window DPI")
	}
	w, h, err := scaledCanvas(a.assets.Canvas, scale*float64(dpi)/96)
	if err != nil {
		return err
	}
	if w == a.w && h == a.h {
		a.scale = scale
		a.dpi = dpi
		return nil
	}
	if err = a.surface.Resize(w, h); err != nil {
		return err
	}
	a.y = scaleAnchor(a.y, a.assets.Config.Drop.GroundY,
		a.scale*float64(a.dpi)/96, scale*float64(dpi)/96)
	a.scale = scale
	a.dpi = dpi
	a.w = w
	a.h = h
	a.haveKey = false
	replaceLatest(a.workers.poses, a.rectangle())
	return a.render(true)
}
func (a *desktopHost) releaseDrag() error {
	a.drag = false
	a.pointerDown = false
	if r, _ := call(getCapture); r == a.hwnd {
		ok, err := call(releaseCapture)
		if ok == 0 {
			return failure("ReleaseCapture", err)
		}
	}
	return nil
}
func (a *desktopHost) menu() error {
	if a.menuOpen {
		return nil
	}
	a.menuOpen = true
	defer func() { a.menuOpen = false }()
	p, err := cursor()
	if err != nil {
		return err
	}
	menu, err := call(createMenu)
	if menu == 0 {
		return failure("CreatePopupMenu", err)
	}
	defer func() {
		if r, e := call(destroyMenu, menu); r == 0 {
			a.terminalError = errors.Join(a.terminalError, failure("DestroyMenu", e))
		}
	}()
	add := func(id uintptr, label string, flags uintptr) error {
		t, e := syscall.UTF16PtrFromString(label)
		if e != nil {
			return e
		}
		r, e := call(appendMenu, menu, flags, id, uintptr(unsafe.Pointer(t)))
		runtime.KeepAlive(t)
		if r == 0 {
			return failure("AppendMenuW", e)
		}
		return nil
	}
	talk := "和奶蛙聊天"
	if a.play != nil && a.play.hotTalk {
		talk += "\tCtrl+Alt+T"
	}
	if err = add(304, talk, 0); err != nil {
		return err
	}
	a.tidyMenu(menu)
	interact, e := call(createMenu)
	if interact == 0 {
		return failure("CreatePopupMenu(interact)", e)
	}
	text := utf("互动内容")
	r, e := call(appendMenu, menu, 0x10, interact, uintptr(unsafe.Pointer(text)))
	runtime.KeepAlive(text)
	if r == 0 {
		call(destroyMenu, interact)
		return failure("AppendMenuW(interact)", e)
	}
	actions, e := call(createMenu)
	if actions == 0 {
		return failure("CreatePopupMenu(actions)", e)
	}
	actionLabels := []string{"待机", "捧腹大笑", "害羞", "惊讶", "伤心", "生气", "疑惑"}
	diagnosticEmotions := []string{"neutral", "happy", "shy", "surprised", "sad", "angry", "confused"}
	for i, label := range actionLabels {
		text := utf(label)
		r, e := call(appendMenu, actions, 0, uintptr(500+i), uintptr(unsafe.Pointer(text)))
		runtime.KeepAlive(text)
		if r == 0 {
			call(destroyMenu, actions)
			return failure("AppendMenuW(action)", e)
		}
	}
	text = utf("陪我一会儿")
	call(appendMenu, actions, 0, 307, uintptr(unsafe.Pointer(text)))
	runtime.KeepAlive(text)
	text = utf("动作")
	r, e = call(appendMenu, interact, 0x10, actions, uintptr(unsafe.Pointer(text)))
	runtime.KeepAlive(text)
	if r == 0 {
		call(destroyMenu, actions)
		return failure("AppendMenuW(actions)", e)
	}
	sizes, e := call(createMenu)
	if sizes == 0 {
		return failure("CreatePopupMenu(sizes)", e)
	}
	text = utf("恢复默认大小")
	call(appendMenu, sizes, 0, 2, uintptr(unsafe.Pointer(text)))
	runtime.KeepAlive(text)
	scales := a.assets.Config.Scale.Menu
	if len(scales) == 0 {
		scales = []float64{a.assets.Config.Scale.Min, a.assets.Config.Scale.Default, a.assets.Config.Scale.Max}
	}
	for i, scale := range scales {
		flags := uintptr(0)
		if math.Abs(scale-a.scale) < .00001 {
			flags = 8
		}
		text = utf(fmt.Sprintf("大小 %.0f%%", scale*100))
		call(appendMenu, sizes, flags, uintptr(100+i), uintptr(unsafe.Pointer(text)))
		runtime.KeepAlive(text)
	}
	text = utf("大小")
	r, e = call(appendMenu, menu, 0x10, sizes, uintptr(unsafe.Pointer(text)))
	runtime.KeepAlive(text)
	if r == 0 {
		call(destroyMenu, sizes)
		return failure("AppendMenuW(sizes)", e)
	}
	a.playMenu(menu, interact)
	settings, e := call(createMenu)
	if settings == 0 {
		return failure("CreatePopupMenu(settings)", e)
	}
	for _, item := range []struct {
		id    uintptr
		label string
	}{{306, "截图预览"}, {301, "设置 / DeepSeek 连接…"}, {302, "暂停观察与提醒"}, {303, "复制设置链接"}, {309, "恢复出厂设置…"}} {
		text := utf(item.label)
		r, e := call(appendMenu, settings, 0, item.id, uintptr(unsafe.Pointer(text)))
		runtime.KeepAlive(text)
		if r == 0 {
			call(destroyMenu, settings)
			return failure("AppendMenuW(settings)", e)
		}
	}
	text = utf("设置与测试")
	r, e = call(appendMenu, menu, 0x10, settings, uintptr(unsafe.Pointer(text)))
	runtime.KeepAlive(text)
	if r == 0 {
		call(destroyMenu, settings)
		return failure("AppendMenuW(settings)", e)
	}
	call(appendMenu, menu, 0x800, 0, 0)

	if err = add(1, "\u9000\u51fa", 0); err != nil {
		return err
	}
	_, _ = call(foregroundWindow, a.hwnd)
	command, err := call(trackMenu, menu, 0x100|0x80|0x2, uintptr(p.X), uintptr(p.Y), 0, a.hwnd, 0)
	if command == 0 {
		if err != nil && err != syscall.Errno(0) {
			return failure("TrackPopupMenu", err)
		}
		return nil
	}
	switch {
	case a.playCommand(command):
		return nil
	case command == menuTidy:
		a.startTidy()
		return nil
	case command == menuUntidy:
		a.startTidy()
		return nil
	case command == 309:
		if companionService != nil {
			title, body := utf("恢复出厂设置"), utf("将清除本版本的 API Key、对话记忆和个性设置，并关闭观察与提醒。确定继续吗？")
			result, _ := call(userDLL.NewProc("MessageBoxW"), a.hwnd, uintptr(unsafe.Pointer(body)), uintptr(unsafe.Pointer(title)), 0x34)
			runtime.KeepAlive(title)
			runtime.KeepAlive(body)
			if result == 6 {
				if e := companionService.RestoreFactory(); e != nil {
					nativeMessage(a.hwnd, "NaiWaChatPet", e.Error(), 0x30)
				}
			}
		}
		return nil
	case command == 308:
		if a.chat != nil {
			a.chat.openEmotionDebug()
		}
		return nil
	case command == 1:
		a.stop(nil)
	case command == 306:
		if a.chat != nil {
			a.chat.openPreview()
		}
		return nil
	case command == 307:
		if companionService != nil {
			companionService.TestCompanionship()
		}
		return nil
	case command >= 500 && command < 507:
		if companionService != nil {
			_ = companionService.RequestDiagnosticEmotion(diagnosticEmotions[int(command)-500])
		}
		return nil
	case command == 304:
		if a.chat != nil {
			if chatErr := a.chat.ShowInput(); chatErr != nil {
				_, _ = logDiagnostic(chatErr)
			}
		} else {
			nativeMessage(a.hwnd, "奶蛙聊天", "原生聊天组件不可用；请在设置中查看诊断。", 0x30)
		}
	case command == 301:
		if companionService == nil {
			_, _ = logDiagnostic(errors.New("chat menu: companion service unavailable"))
			nativeMessage(a.hwnd, "奶蛙陪伴", "聊天服务尚未启动，请查看诊断日志；奶蛙仍可正常使用。", 0x30)
			return nil
		}
		// Browser activation is optional and must NEVER tear down the desktop-pet
		// message loop. ShellExecuteW can fail on a user's Windows profile due
		// to URL association, DDE/COM or browser policy even while the pet is fine.
		startCompanionPage(companionService)
		return nil
	case command == 303:
		if err := copyChatURLFromMenu(); err != nil {
			_, _ = logDiagnostic(fmt.Errorf("chat: clipboard fallback failed: %w", err))
			nativeMessage(a.hwnd, "奶蛙聊天", "复制聊天链接失败，请查看 diagnostic.log。奶蛙仍可正常使用。", 0x30)
		} else {
			nativeMessage(a.hwnd, "奶蛙聊天链接", "聊天地址已复制到剪贴板。请在浏览器地址栏粘贴并回车。", 0x40)
		}
		return nil
	case command == 302:
		if companionService != nil {
			companionService.Pause()
		}
		a.haveKey = false
	case command == 2:
		return a.resize(a.assets.Config.Scale.Default, a.dpi)
	case command >= 100 && command < uintptr(100+len(scales)):
		return a.resize(scales[int(command)-100], a.dpi)
	}
	return nil
}

// startCompanionPage launches the companion independently of the native GUI.
// All failures are reported, never fed into desktopHost.stop.
func startCompanionPage(s *companion.Server) {
	traceStartup("chat: browser launch requested")
	go func() {
		defer func() {
			if p := recover(); p != nil {
				_, _ = logDiagnostic(fmt.Errorf("chat browser launcher panic: %v", p))
			}
		}()
		if err := openCompanionPage(s); err == nil {
			traceStartup("chat: ShellExecuteW accepted browser request")
			return
		} else {
			_, _ = logDiagnostic(fmt.Errorf("chat: ShellExecuteW failed, trying Windows URL handler: %w", err))
		}
		// The Windows URL protocol handler is independent of the shell's direct
		// URL association path, and needs no third-party libraries or services.
		cmd := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", s.URL())
		if err := cmd.Start(); err == nil {
			traceStartup("chat: fallback URL handler started")
			if waitErr := cmd.Wait(); waitErr != nil {
				_, _ = logDiagnostic(fmt.Errorf("chat: fallback URL handler exited abnormally: %w", waitErr))
			}
			return
		} else {
			_, _ = logDiagnostic(fmt.Errorf("chat: fallback URL handler could not start: %w", err))
		}
		// Only show a warning; the pet window remains alive and interactive.
		nativeMessage(0, "奶蛙聊天无法打开", "无法启动系统默认浏览器。请右键奶蛙选择『复制聊天链接』，并在浏览器地址栏粘贴打开。奶蛙会继续正常运行；详情见 diagnostic.log。", 0x30)
	}()
}

func openCompanionPage(s *companion.Server) error {
	target, err := syscall.UTF16PtrFromString(s.URL())
	if err != nil {
		return err
	}
	verb, err := syscall.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	code, _, e := shellExecute.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(target)), 0, 0, 1)
	runtime.KeepAlive(verb)
	runtime.KeepAlive(target)
	if code <= 32 {
		return fmt.Errorf("ShellExecuteW browser launch result=%d: %w", code, failure("ShellExecuteW", e))
	}
	return nil
}

func droppedPaths(handle uintptr) ([]string, error) {
	defer call(dragFinish, handle)
	count, err := call(dragQuery, handle, 0xffffffff, 0, 0)
	if count == 0 {
		if err != nil && err != syscall.Errno(0) {
			return nil, failure("DragQueryFileW(count)", err)
		}
		return nil, nil
	}
	if count > 1024 {
		return nil, errors.New("at most 1024 files may be dropped together")
	}
	paths := make([]string, 0, count)
	for i := uintptr(0); i < count; i++ {
		n, e := call(dragQuery, handle, i, 0, 0)
		if n == 0 || n > 32767 {
			return nil, failure("DragQueryFileW(length)", e)
		}
		buf := make([]uint16, n+1)
		got, e := call(dragQuery, handle, i, uintptr(unsafe.Pointer(&buf[0])), n+1)
		runtime.KeepAlive(buf)
		if got != n {
			return nil, failure("DragQueryFileW(path)", e)
		}
		paths = append(paths, syscall.UTF16ToString(buf))
	}
	return paths, nil
}
func windowProc(hwnd, msg, wParam, lParam uintptr) (result uintptr) {
	a := activeHost
	defer func() {
		if p := recover(); p != nil {
			if a != nil {
				a.stop(fmt.Errorf("window callback: %v", p))
			}
			result = 0
		}
	}()
	if a == nil {
		r, _ := call(defWindowProc, hwnd, msg, wParam, lParam)
		return r
	}
	if a.tray != nil && a.tray.Handle(a, uint32(msg), uint32(lParam)) {
		return 0
	}
	switch uint32(msg) {
	case 0x0100, 0x0104:
		if wParam == 0x79 && a.chat != nil {
			a.chat.openEmotionDebug()
			return 0
		}
		if uint32(msg) == 0x0104 {
			r, _ := call(defWindowProc, hwnd, msg, wParam, lParam)
			return r
		}
		if wParam == 0x71 && a.chat != nil {
			a.chat.openPreview()
			return 0
		}
		if wParam == 0x72 && companionService != nil {
			companionService.TestCompanionship()
			return 0
		}
		if wParam >= 0x73 && wParam <= 0x78 && companionService != nil {
			names := []string{"shy", "surprised", "sad", "angry", "confused", "happy"}
			_ = companionService.RequestDiagnosticEmotion(names[wParam-0x73])
			return 0
		}
		if a.playKey(wParam, lParam&(1<<30) != 0) {
			return 0
		}
	case wmHotkey:
		a.playCommand(wParam)
		return 0
	case wmClose:
		select {
		case <-carryShutdown:
		default:
			close(carryShutdown)
		}
		if a.trip != nil {
			a.trip.err = errors.New("程序退出，未送达的文件保持原位")
			a.finishTrip()
		}
		if organizerService != nil {
			organizerService.Close()
		}
		if a.workers != nil {
			a.workers.cancel()
		}
		a.closing = true
		if r, e := call(destroyWindow, hwnd); r == 0 {
			a.terminalError = errors.Join(a.terminalError, failure("DestroyWindow", e))
			_, _ = call(postQuit, 1)
		}
		return 0
	case wmDestroy:
		if a.timerReady {
			if r, e := call(killTimer, hwnd, 1); r == 0 {
				a.terminalError = errors.Join(a.terminalError, failure("KillTimer", e))
			}
			a.timerReady = false
		}
		_, _ = call(postQuit, 0)
		return 0
	case wmEraseBackground:
		return 1
	case wmPaint:
		_, _ = call(validateRect, hwnd, 0)
		return 0
	}
	if a.hwnd == 0 || a.surface == nil {
		r, _ := call(defWindowProc, hwnd, msg, wParam, lParam)
		return r
	}
	var err error
	switch uint32(msg) {
	case wmTimer:
		if wParam == 1 && !a.closing {
			// Keep the clock claimed through rendering, not just simulation.
			// Otherwise an expensive frame continuously queues another posted
			// message, starving Windows' lower-priority hardware input queue.
			if a.play != nil && a.play.fight != nil && a.play.fight.upgraded != nil {
				clock := a.play.fight.upgraded.clock
				if clock != nil {
					if clock.rendering {
						return 0
					}
					clock.rendering = true
					defer func() {
						clock.rendering = false
						select {
						case clock.presented <- struct{}{}:
						default:
						}
					}()
				}
			}
			err = a.tick()
		}
	case wmLeftDown:
		var p pointWin
		p, err = cursor()
		if err == nil {
			a.pointerDown = true
			a.drag = false
			a.gesture.Down(a.now(), interaction.Point{X: int(p.X), Y: int(p.Y)})
			a.startCursor = p
			a.startX = a.x
			a.startY = a.y
			if a.play != nil {
				a.play.dragX = p.X
				if a.play.control {
					// Clicking the pet while steering hands it the keyboard back.
					_, _ = call(foregroundWindow, hwnd)
					_, _ = call(chatSetFocus, hwnd)
				}
			}
			_, _ = call(setCapture, hwnd)
			if captured, _ := call(getCapture); captured != hwnd {
				err = errors.New("SetCapture did not capture this window")
			}
		}
	case wmMouseMove:
		if a.pointerDown {
			var p pointWin
			p, err = cursor()
			if err == nil && a.gesture.Move(interaction.Point{X: int(p.X), Y: int(p.Y)}) {
				if !a.drag && a.emotions != nil {
					a.emotions.InterruptAnimation("drag")
				}
				if !a.drag && a.play != nil {
					// Wherever a scene put the window is now simply where the pet is.
					a.play.sceneShift = 0
				}
				a.drag = true
				if a.play != nil {
					a.shaken(int(p.X-a.play.dragX), a.now())
					a.play.dragX = p.X
				}
				a.x = a.startX + int(p.X-a.startCursor.X)
				a.y = a.startY + int(p.Y-a.startCursor.Y)
				replaceLatest(a.workers.poses, a.rectangle())
				err = a.render(false)
			}
		}
	case wmLeftUp:
		a.gesture.Up(a.now())
		if a.drag {
			a.dropped()
		}
		err = a.releaseDrag()
	case wmCaptureChanged:
		a.drag = false
		a.pointerDown = false
		a.gesture.CancelPress()
	case wmDoubleClick:
		a.gesture.Double(a.now())
		if a.chat != nil {
			a.chat.HideInput()
		}
		err = a.releaseDrag()
		if err == nil {
			_, err = a.queue.requestLaugh(a.assets.Engine, a.now())
			if err == nil {
				err = a.render(false)
			}
		}
	case wmRightUp:
		err = a.menu()
	case wmMouseWheel:
		if wParam&8 != 0 {
			delta := int16(wParam >> 16)
			if delta != 0 {
				err = a.resize(a.scale+float64(delta)/120*a.assets.Config.Scale.Step, a.dpi)
			}
		}
	case wmDPIChanged:
		dpi := uint32(wParam & 0xffff)
		err = a.resize(a.scale, dpi)
	case wmDropFiles:
		var paths []string
		paths, err = droppedPaths(wParam)
		if err == nil {
			err = a.workers.submit(paths)
		}
		if err != nil {
			_, _ = logDiagnostic(err)
			nativeMessage(hwnd, "\u6587\u4ef6\u65e0\u6cd5\u8bfb\u53d6", "\u8bf7\u68c0\u67e5\u6587\u4ef6\u8def\u5f84\u548c\u8bbf\u95ee\u6743\u9650\u3002", 0x30)
			return 0
		}
	case wmNCHitTest:
		x, y := int(int16(lParam&0xffff))-a.x, int(int16(lParam>>16))-a.y
		mask := a.assets.Mask
		if a.assets.Config.HoverMaskMode == "body" && a.currentBody != nil {
			mask = a.currentBody
		}
		if !a.drag && !maskHit(mask, x, y, a.w, a.h) {
			return ^uintptr(0)
		}
		return 1
	default:
		r, _ := call(defWindowProc, hwnd, msg, wParam, lParam)
		return r
	}
	if err != nil {
		a.stop(err)
	}
	return 0
}
