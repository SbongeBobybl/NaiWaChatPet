//go:build windows

package main

import (
	"codexpet/reconstruction/companion"
	"codexpet/reconstruction/desktopchat"
	"errors"
	"fmt"
	"math"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	chatDrawText       = userDLL.NewProc("DrawTextW")
	chatDrawFocus      = userDLL.NewProc("DrawFocusRect")
	chatSetBkMode      = syscall.NewLazyDLL("gdi32.dll").NewProc("SetBkMode")
	chatSelectObject   = syscall.NewLazyDLL("gdi32.dll").NewProc("SelectObject")
	chatRoundRect      = syscall.NewLazyDLL("gdi32.dll").NewProc("RoundRect")
	chatGDI            = syscall.NewLazyDLL("gdi32.dll")
	chatControls       = syscall.NewLazyDLL("comctl32.dll")
	chatCreateBrush    = chatGDI.NewProc("CreateSolidBrush")
	chatCreateFont     = chatGDI.NewProc("CreateFontW")
	chatDeleteObject   = chatGDI.NewProc("DeleteObject")
	chatRoundRgn       = chatGDI.NewProc("CreateRoundRectRgn")
	chatPolygonRgn     = chatGDI.NewProc("CreatePolygonRgn")
	chatCombineRgn     = chatGDI.NewProc("CombineRgn")
	chatSetTextColor   = chatGDI.NewProc("SetTextColor")
	chatSetBkColor     = chatGDI.NewProc("SetBkColor")
	chatSetWindowRgn   = userDLL.NewProc("SetWindowRgn")
	chatSetAlpha       = userDLL.NewProc("SetLayeredWindowAttributes")
	chatSend           = userDLL.NewProc("SendMessageW")
	chatSetText        = userDLL.NewProc("SetWindowTextW")
	chatGetText        = userDLL.NewProc("GetWindowTextW")
	chatTextLength     = userDLL.NewProc("GetWindowTextLengthW")
	chatSetFocus       = userDLL.NewProc("SetFocus")
	chatGetFocus       = userDLL.NewProc("GetFocus")
	chatGetKeyState    = userDLL.NewProc("GetKeyState")
	chatBeginPaint     = userDLL.NewProc("BeginPaint")
	chatEndPaint       = userDLL.NewProc("EndPaint")
	chatFillRect       = userDLL.NewProc("FillRect")
	chatGetClientRect  = userDLL.NewProc("GetClientRect")
	chatMonitor        = userDLL.NewProc("MonitorFromWindow")
	chatMonitorInfo    = userDLL.NewProc("GetMonitorInfoW")
	chatDoubleTime     = userDLL.NewProc("GetDoubleClickTime")
	chatSubclass       = chatControls.NewProc("SetWindowSubclass")
	chatRemoveSubclass = chatControls.NewProc("RemoveWindowSubclass")
	chatDefSubclass    = chatControls.NewProc("DefSubclassProc")
	activeChat         *nativeChat
)

type paintStruct struct {
	DC              uintptr
	Erase           int32
	Rect            rectWin
	Restore, Update int32
	Reserved        [32]byte
}
type monitorInfo struct {
	Size          uint32
	Monitor, Work rectWin
	Flags         uint32
}
type drawItem struct {
	Type, ID, Item, Action, State uint32
	Window, DC                    uintptr
	Rect                          rectWin
	Data                          uintptr
}
type nativeChat struct {
	buttonMotions                                                           map[uintptr]*buttonMotion
	keyboardFocus                                                           bool
	inputSkin, replySkin                                                    *nativeSkin
	chromaBrush, buttonFont, sendFont, smallFont, inkBrush                  uintptr
	inputUp, inputDown, replyUp, replyDown                                  uintptr
	controlPositions                                                        map[uintptr]pointWin
	emotionDebugWindow, emotionDebugText                                    uintptr
	emotionDebugLast                                                        string
	emotionDebugVisible                                                     bool
	nativePreviewSequence                                                   uint64
	preview                                                                 *nativePreview
	shareStatus, skipImage, previewButton                                   uintptr
	omitNextImage                                                           bool
	shareText                                                               string
	buttonBrush                                                             uintptr
	owner                                                                   *desktopHost
	module                                                                  uintptr
	className                                                               *uint16
	input, reply, edit, send, replyText, primary, closeButton               uintptr
	brush, font                                                             uintptr
	dpi                                                                     uint32
	inputVisible, replyVisible, expanded, reading, composing, suppressEnter bool
	draft, failedInput                                                      string
	local                                                                   *companion.ReplyView
	last                                                                    companion.ReplyView
	inputRect                                                               desktopchat.Rect
	bubble                                                                  desktopchat.Placement
	appeared, deadline                                                      time.Time
	alpha                                                                   int
}

func utf(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }
func setText(hwnd uintptr, s string) {
	p := utf(s)
	if p == nil {
		return
	}
	_, _ = call(chatSetText, hwnd, uintptr(unsafe.Pointer(p)))
	runtime.KeepAlive(p)
}
func controlText(hwnd uintptr) string {
	n, _ := call(chatTextLength, hwnd)
	n = min(n, 32768)
	b := make([]uint16, n+1)
	_, _ = call(chatGetText, hwnd, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
	runtime.KeepAlive(b)
	return syscall.UTF16ToString(b)
}
func newNativeChat(a *desktopHost) (c *nativeChat, err error) {
	c = &nativeChat{owner: a, dpi: 96, alpha: -1}
	c.module, _ = call(moduleHandle, 0)
	c.className = utf(fmt.Sprintf("CodexPetV13Chat_%d", c.module))
	c.brush, _ = call(chatCreateBrush, 0xC9E9FF) // RGB(255,233,201), warm milk.
	c.buttonBrush, _ = call(chatCreateBrush, 0x96D2EE)
	if c.brush == 0 {
		return nil, errors.New("CreateSolidBrush for native chat failed")
	}
	k := classWin{Size: uint32(unsafe.Sizeof(classWin{})), Procedure: syscall.NewCallback(chatProc), Instance: c.module, ClassName: c.className}
	k.Cursor, _ = call(loadCursor, 0, 32512)
	if r, e := call(registerClass, uintptr(unsafe.Pointer(&k))); r == 0 {
		_, _ = call(chatDeleteObject, c.brush)
		return nil, failure("RegisterClassExW(chat)", e)
	}
	runtime.KeepAlive(k)
	activeChat = c
	defer func() {
		if err != nil {
			c.Close()
			c = nil
		}
	}()
	makeWindow := func(title string) (uintptr, error) {
		t := utf(title)
		exStyle := uintptr(wsExLayered | wsExToolWindow | wsExTopmost)
		if startMode == "demo" {
			exStyle = wsExLayered | wsExTopmost | 0x00040000
		}
		h, e := call(createWindow, exStyle, uintptr(unsafe.Pointer(c.className)), uintptr(unsafe.Pointer(t)), wsPopup|0x02000000, 0, 0, 340, 120, a.hwnd, 0, c.module, 0)
		runtime.KeepAlive(t)
		if h == 0 {
			return 0, failure("CreateWindowExW(chat)", e)
		}
		_, _ = call(chatSetAlpha, h, 0, 255, 2)
		return h, nil
	}
	c.input, err = makeWindow("NaiWaChatPet · 输入")
	if err != nil {
		return c, err
	}
	c.reply, err = makeWindow("NaiWaChatPet · 回复")
	if err != nil {
		return c, err
	}
	child := func(parent uintptr, class, title string, style, id uintptr) (uintptr, error) {
		cl, t := utf(class), utf(title)
		h, e := call(createWindow, 0, uintptr(unsafe.Pointer(cl)), uintptr(unsafe.Pointer(t)), 0x40000000|0x10000000|0x10000|style, 0, 0, 10, 10, parent, id, c.module, 0)
		runtime.KeepAlive(cl)
		runtime.KeepAlive(t)
		if h == 0 {
			return 0, failure("CreateWindowExW(chat control)", e)
		}
		return h, nil
	}
	c.edit, err = child(c.input, "EDIT", "", 0x0004|0x0040|0x1000, 101)
	if err != nil {
		return c, err
	}
	c.send, err = child(c.input, "BUTTON", "发送", 11, 102)
	if err != nil {
		return c, err
	}
	c.replyText, err = child(c.reply, "EDIT", "奶蛙正在想……", 0x0004|0x0040|0x0800, 201)
	if err != nil {
		return c, err
	}
	c.primary, err = child(c.reply, "BUTTON", "取消", 11, 202)
	if err != nil {
		return c, err
	}
	c.closeButton, err = child(c.reply, "BUTTON", "关闭", 11, 203)
	if err != nil {
		return c, err
	}
	_, _ = call(chatSend, c.edit, 0x00C5, 16000, 0)
	if r, e := call(chatSubclass, c.edit, syscall.NewCallback(chatEditProc), 1, 0); r == 0 {
		return c, failure("SetWindowSubclass(input)", e)
	}
	_, _ = call(chatSubclass, c.replyText, syscall.NewCallback(chatReadProc), 2, 0)
	_, _ = call(chatSend, c.replyText, 0x00C5, 262144, 0)
	c.shareStatus, err = child(c.input, "STATIC", "", 0, 104)
	if err != nil {
		return c, err
	}
	c.skipImage, err = child(c.input, "BUTTON", "仅文字", 11, 105)
	if err != nil {
		return c, err
	}
	c.previewButton, err = child(c.input, "BUTTON", "预览截图", 11, 106)
	if err != nil {
		return c, err
	}
	for _, item := range []struct {
		parent uintptr
		id     uintptr
		title  string
		dest   *uintptr
	}{{c.input, 107, "向上滚动输入", &c.inputUp}, {c.input, 108, "向下滚动输入", &c.inputDown}, {c.reply, 207, "向上滚动回复", &c.replyUp}, {c.reply, 208, "向下滚动回复", &c.replyDown}} {
		*item.dest, err = child(item.parent, "BUTTON", item.title, 11, item.id)
		if err != nil {
			return c, err
		}
	}
	if err = c.initSkins(); err != nil {
		return c, err
	}
	c.initButtons()
	err = c.setFont(a.dpi)
	if err != nil {
		return c, err
	}
	return c, nil
}
func (c *nativeChat) Close() {
	if c.emotionDebugWindow != 0 {
		_, _ = call(destroyWindow, c.emotionDebugWindow)
		c.emotionDebugWindow = 0
	}
	if c.preview != nil {
		c.closePreview()
		_, _ = call(destroyWindow, c.preview.window)
	}
	for _, h := range []uintptr{c.input, c.reply} {
		if h != 0 {
			if valid, _ := call(userDLL.NewProc("IsWindow"), h); valid != 0 {
				_, _ = call(destroyWindow, h)
			}
		}
	}
	if c.inputSkin != nil {
		c.inputSkin.Close()
	}
	if c.replySkin != nil {
		c.replySkin.Close()
	}
	for _, h := range []uintptr{c.chromaBrush, c.buttonFont, c.sendFont, c.smallFont, c.inkBrush} {
		if h != 0 {
			call(chatDeleteObject, h)
		}
	}
	if c.className != nil {
		_, _ = call(unregisterClass, uintptr(unsafe.Pointer(c.className)), c.module)
		runtime.KeepAlive(c.className)
	}
	if c.font != 0 {
		_, _ = call(chatDeleteObject, c.font)
	}
	if c.brush != 0 {
		_, _ = call(chatDeleteObject, c.brush)
	}
	if c.buttonBrush != 0 {
		_, _ = call(chatDeleteObject, c.buttonBrush)
	}
	if activeChat == c {
		activeChat = nil
	}
}
func (c *nativeChat) setFont(dpi uint32) error {
	if dpi < 48 || dpi > 768 {
		dpi = 96
	}
	if c.font != 0 && c.dpi == dpi {
		return nil
	}
	height := int32(-int(math.Round(12 * float64(dpi) / 96)))
	face := utf("Microsoft YaHei UI")
	font, e := call(chatCreateFont, uintptr(height), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(face)))
	runtime.KeepAlive(face)
	if font == 0 {
		return failure("CreateFontW(chat)", e)
	}
	for _, h := range []uintptr{c.buttonFont, c.sendFont, c.smallFont} {
		if h != 0 {
			call(chatDeleteObject, h)
		}
	}
	makeFont := func(size, weight int) uintptr {
		h, _ := call(chatCreateFont, uintptr(int32(-int(math.Round(float64(size)*float64(dpi)/96)))), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(face)))
		runtime.KeepAlive(face)
		return h
	}
	c.buttonFont = makeFont(11, 600)
	c.sendFont = makeFont(14, 600)
	c.smallFont = makeFont(9, 400)
	if c.inkBrush == 0 {
		c.inkBrush, _ = call(chatCreateBrush, 0x173870)
	}
	old := c.font
	c.font = font
	c.dpi = dpi
	for _, h := range []uintptr{c.edit, c.send, c.replyText, c.primary, c.closeButton, c.emotionDebugText, c.shareStatus, c.skipImage, c.previewButton, c.inputUp, c.inputDown, c.replyUp, c.replyDown} {
		_, _ = call(chatSend, h, 0x0030, font, 1)
	}
	call(chatSend, c.shareStatus, 0x0030, c.smallFont, 1)
	if old != 0 {
		_, _ = call(chatDeleteObject, old)
	}
	return nil
}
func (c *nativeChat) workArea() desktopchat.Rect {
	m, _ := call(chatMonitor, c.owner.hwnd, 2)
	info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if m != 0 {
		if ok, _ := call(chatMonitorInfo, m, uintptr(unsafe.Pointer(&info))); ok != 0 {
			return desktopchat.Rect{X: int(info.Work.Left), Y: int(info.Work.Top), W: int(info.Work.Right - info.Work.Left), H: int(info.Work.Bottom - info.Work.Top)}
		}
	}
	w, _ := call(getSystemMetrics, 0)
	h, _ := call(getSystemMetrics, 1)
	return desktopchat.Rect{W: int(w), H: int(h)}
}
func (c *nativeChat) px(v int) int { return max(1, int(math.Round(float64(v)*float64(c.dpi)/96))) }
func (c *nativeChat) position() error {
	if e := c.setFont(c.owner.dpi); e != nil {
		return e
	}
	a := c.owner
	work := c.workArea()
	pet := desktopchat.Rect{X: a.x, Y: a.y, W: a.w, H: a.h}
	ground := a.y + int(math.Round(a.assets.Config.Drop.GroundY*float64(a.h)/float64(a.assets.Canvas.Dy())))
	if c.inputVisible {
		// A pet on the move is not shoved aside to make room for the box; the
		// box follows it instead and keeps to the screen.
		if moving := a.play != nil && (a.play.control || a.play.ctl.Busy()); !moving {
			adjusted := desktopchat.PetForInput(pet, work, ground, c.dpi)
			if adjusted != pet {
				ground += adjusted.Y - pet.Y
				a.x, a.y = adjusted.X, adjusted.Y
				pet = adjusted
				if err := a.render(false); err != nil {
					return err
				}
			}
		}
		r := desktopchat.Input(pet, work, ground, c.dpi)
		if r != c.inputRect {
			if e := c.place(c.input, r, desktopchat.Placement{Rect: r}); e != nil {
				return e
			}
			c.inputRect = r
			c.moveControl(c.edit, r.W*80/1000, r.H*30/100, r.W*62/100, r.H*30/100)
			c.moveControl(c.send, r.W*775/1000, r.H*35/100, r.W*195/1000, r.H*30/100)
			c.moveControl(c.skipImage, r.W*42/100, r.H*63/100, r.W*23/100, c.px(26))
			c.moveControl(c.previewButton, r.W*665/1000, r.H*63/100, r.W*285/1000, c.px(26))
			c.moveControl(c.shareStatus, r.W*10/100, r.H*69/100, r.W*27/100, c.px(16))
			c.moveControl(c.inputUp, r.W*72/100, r.H*35/100, c.px(12), c.px(12))
			c.moveControl(c.inputDown, r.W*72/100, r.H*56/100, c.px(12), c.px(12))
		}
	}
	if c.replyVisible {
		mouth := a.mouthAnchor()
		anchor := desktopchat.Point{X: a.x + int(math.Round(float64(mouth.X)*float64(a.w)/420)), Y: a.y + int(math.Round(float64(mouth.Y)*float64(a.h)/560))}
		lines := 1 + len([]rune(c.last.Text))/26 + strings.Count(c.last.Text, "\n")
		p := desktopchat.Bubble(pet, work, anchor, c.dpi, lines, c.expanded)
		if p != c.bubble {
			if e := c.place(c.reply, p.Rect, p); e != nil {
				return e
			}
			c.bubble = p
			c.moveControl(c.replyText, p.Rect.W*11/100, p.Rect.H*19/100, p.Rect.W*74/100, p.Rect.H*45/100)
			c.moveControl(c.primary, p.Rect.W*64/100, p.Rect.H*66/100, c.px(52), c.px(26))
			c.moveControl(c.closeButton, p.Rect.W*79/100, p.Rect.H*66/100, c.px(46), c.px(26))
			c.moveControl(c.replyUp, p.Rect.W*88/100, p.Rect.H*30/100, c.px(12), c.px(12))
			c.moveControl(c.replyDown, p.Rect.W*88/100, p.Rect.H*52/100, c.px(12), c.px(12))
		}
	}
	c.updateScrollControls()
	return nil
}
func (c *nativeChat) moveControl(h uintptr, x, y, w, height int) {
	if c.controlPositions != nil {
		c.controlPositions[h] = pointWin{int32(x), int32(y)}
	}
	_, _ = call(setWindowPos, h, 0, uintptr(x), uintptr(y), uintptr(max(1, w)), uintptr(max(1, height)), 0x0014)
}
func (c *nativeChat) place(hwnd uintptr, r desktopchat.Rect, p desktopchat.Placement) error {
	if ok, e := call(setWindowPos, hwnd, 0, uintptr(r.X), uintptr(r.Y), uintptr(r.W), uintptr(r.H), 0x0014); ok == 0 {
		return failure("SetWindowPos(chat)", e)
	}
	if c.skinFor(hwnd) != nil {
		return c.placeSkin(hwnd, r, p)
	}
	tail := c.px(12)
	left, top, right, bottom := 0, 0, r.W+1, r.H+1
	switch p.Tail {
	case "left":
		left = tail
	case "right":
		right -= tail
	case "top":
		top = tail
	case "bottom":
		bottom -= tail
	}
	region, e := call(chatRoundRgn, uintptr(left), uintptr(top), uintptr(right), uintptr(bottom), uintptr(c.px(22)), uintptr(c.px(22)))
	if region == 0 {
		return failure("CreateRoundRectRgn", e)
	}
	if p.Tail != "" {
		y := max(c.px(22), min(p.Tip.Y-r.Y, bottom-c.px(50)))
		x := max(c.px(30), min(p.Tip.X-r.X, right-c.px(30)))
		half := c.px(8)
		var pts [3]pointWin
		switch p.Tail {
		case "left":
			pts = [3]pointWin{{int32(left), int32(y - half)}, {int32(left), int32(y + half)}, {0, int32(y)}}
		case "right":
			pts = [3]pointWin{{int32(right), int32(y - half)}, {int32(right), int32(y + half)}, {int32(r.W), int32(y)}}
		case "top":
			pts = [3]pointWin{{int32(x - half), int32(top)}, {int32(x + half), int32(top)}, {int32(x), 0}}
		case "bottom":
			pts = [3]pointWin{{int32(x - half), int32(bottom)}, {int32(x + half), int32(bottom)}, {int32(x), int32(r.H)}}
		}
		polygon, _ := call(chatPolygonRgn, uintptr(unsafe.Pointer(&pts[0])), 3, 1)
		runtime.KeepAlive(pts)
		if polygon != 0 {
			_, _ = call(chatCombineRgn, region, region, polygon, 2)
			_, _ = call(chatDeleteObject, polygon)
		}
	}
	if ok, e := call(chatSetWindowRgn, hwnd, region, 1); ok == 0 {
		_, _ = call(chatDeleteObject, region)
		return failure("SetWindowRgn(chat)", e)
	}
	return nil
}
func (c *nativeChat) ShowInput() error {
	// Steering and walking carry on: the chat box simply has the keyboard
	// until the message is sent or put away.
	c.inputVisible = true
	c.inputRect = desktopchat.Rect{}
	if e := c.position(); e != nil {
		return e
	}
	setText(c.edit, c.draft)
	_, _ = call(showWindow, c.input, 1)
	c.showSkin(c.input, true)
	_, _ = call(foregroundWindow, c.input)
	_, _ = call(chatSetFocus, c.edit)
	return nil
}
func (c *nativeChat) HideInput() {
	if c.inputVisible {
		c.draft = controlText(c.edit)
		c.inputVisible = false
		_, _ = call(showWindow, c.input, 0)
		c.showSkin(c.input, false)
	}
}
func (c *nativeChat) hideReply() {
	c.replyVisible = false
	_, _ = call(showWindow, c.reply, 0)
	c.showSkin(c.reply, false)
}
func (c *nativeChat) sendText(text string) {
	c.failedInput = text
	if companionService == nil {
		c.showLocalError("聊天服务暂不可用；奶蛙的原有动作仍可使用。")
		return
	}
	if _, e := companionService.SubmitWithSharing(text, c.omitNextImage); e != nil {
		c.showLocalError(e.Error())
		return
	}
	c.failedInput = ""
	c.omitNextImage = false
	c.local = nil
	c.draft = ""
	setText(c.edit, "")
	c.HideInput()
	c.returnKeyboard()
	_ = c.Tick()
}

// returnKeyboard hands the keys back to the pet after a message, so a pet
// being steered can walk on while it answers.
func (c *nativeChat) returnKeyboard() {
	if p := c.owner.play; p != nil && p.control {
		_, _ = call(foregroundWindow, c.owner.hwnd)
		_, _ = call(chatSetFocus, c.owner.hwnd)
	}
}

// say shows a line of the pet's own in the reply bubble.
func (c *nativeChat) say(text string) {
	v := companion.ReplyView{RequestID: fmt.Sprint("pet-", time.Now().UnixNano()), Stage: "local", Text: text, UpdatedAt: time.Now()}
	c.local = &v
	if err := c.Tick(); err != nil {
		_, _ = logDiagnostic(err)
	}
}
func (c *nativeChat) showLocalError(message string) {
	v := companion.ReplyView{RequestID: fmt.Sprint("local-", time.Now().UnixNano()), Stage: "error", Error: message, UpdatedAt: time.Now()}
	c.local = &v
	_ = c.Tick()
}
func (c *nativeChat) Tick() error {
	c.tickButtons()
	if companionService != nil {
		companionService.DesktopInputVisible(c.inputVisible)
	}
	c.tickEmotionDebug()
	c.previewTick()
	if companionService != nil {
		seq := companionService.NativePreviewRequest()
		if seq != c.nativePreviewSequence {
			c.nativePreviewSequence = seq
			c.openPreview()
		}
	}
	if companionService != nil {
		defer func() { companionService.DesktopReplyVisible(c.replyVisible) }()
		v := companionService.Sharing()
		t := "屏幕关闭"
		if v.Allowed {
			t = "已允许附图"
		}
		if v.Mode == "smart" && v.Allowed {
			t = "按需附图"
		}
		if v.Uploading {
			t = "正在上传图像"
		}
		if v.Capturing {
			t = "正在截图"
		}
		if c.omitNextImage {
			t = "本次不附图"
		}
		t = "▣ " + t
		if t != c.shareText {
			setText(c.shareStatus, t)
			c.shareText = t
		}
	}

	var v companion.ReplyView
	if companionService != nil {
		v = companionService.View()
	}
	if c.local != nil && (!v.UpdatedAt.After(c.local.UpdatedAt)) {
		v = *c.local
	} else {
		c.local = nil
	}
	if v.RequestID == "" && c.last.RequestID != "" {
		c.last = companion.ReplyView{}
		c.hideReply()
	}
	if v.RequestID != "" && v != c.last {
		fresh := v.RequestID != c.last.RequestID
		if fresh {
			c.reading = false
			c.expanded = false
			c.appeared = time.Now()
			c.alpha = -1
			c.deadline = time.Time{}
			c.bubble = desktopchat.Placement{}
		}
		c.last = v
		content := v.Text
		if v.Stage == "waiting" {
			content = "奶蛙正在想……"
		}
		if v.Error != "" {
			if strings.TrimSpace(content) != "" {
				content += "\n\n"
			}
			content += v.Error
		}
		if strings.TrimSpace(content) == "" {
			content = "这次没有收到正文，请重试。"
		}
		first, _ := call(chatSend, c.replyText, 0x00CE, 0, 0)
		setText(c.replyText, strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\n", "\r\n"))
		if c.reading {
			_, _ = call(chatSend, c.replyText, 0x00B6, 0, first)
		} else if v.Stage == "completed" && desktopchat.DismissAfter(v.Text) == 0 {
			// A completed unread long answer opens at its beginning.
			_, _ = call(chatSend, c.replyText, 0x00B1, 0, 0)
			_, _ = call(chatSend, c.replyText, 0x00B7, 0, 0)
		} else {
			_, _ = call(chatSend, c.replyText, 0x00B1, ^uintptr(0), ^uintptr(0))
			_, _ = call(chatSend, c.replyText, 0x00B7, 0, 0)
		}
		action := "展开"
		switch v.Stage {
		case "waiting", "streaming":
			action = "取消"
		case "error", "cancelled":
			action = "重试"
		}
		setText(c.primary, action)
		c.replyVisible = true
		if e := c.position(); e != nil {
			return e
		}
		_, _ = call(showWindow, c.reply, 4)
		c.showSkin(c.reply, true)
		if v.Stage == "completed" || v.Stage == "local" {
			if duration := desktopchat.DismissAfter(v.Text); duration > 0 {
				c.deadline = time.Now().Add(duration)
			}
		}
	}
	if c.replyVisible {
		if !c.reading && !c.deadline.IsZero() && time.Now().After(c.deadline) {
			c.hideReply()
			return nil
		}
		alpha := min(255, max(24, int(255*time.Since(c.appeared)/(200*time.Millisecond))))
		if alpha != c.alpha {
			c.chatOpacity(alpha)
			c.alpha = alpha
		}
	}
	return c.position()
}
func (c *nativeChat) primaryAction() {
	switch c.last.Stage {
	case "waiting", "streaming":
		if companionService != nil {
			companionService.Cancel()
		}
	case "error", "cancelled":
		if c.failedInput != "" {
			c.sendText(c.failedInput)
		} else if companionService != nil {
			if _, e := companionService.Retry(); e != nil {
				c.showLocalError(e.Error())
			}
		}
	default:
		c.expanded = !c.expanded
		c.reading = true
		c.bubble = desktopchat.Placement{}
		if c.expanded {
			setText(c.primary, "收起")
		} else {
			setText(c.primary, "展开")
		}
		_ = c.position()
	}
}
func (c *nativeChat) tab(message messageWin) bool {
	if !c.inputVisible || message.Message != 0x0100 || message.WParam != 9 {
		return false
	}
	c.keyboardFocus = true
	focus, _ := call(chatGetFocus)
	order := []uintptr{c.edit, c.send, c.skipImage, c.previewButton}
	for _, h := range []uintptr{c.inputUp, c.inputDown} {
		if visible, _ := call(userDLL.NewProc("IsWindowVisible"), h); visible != 0 {
			order = append(order, h)
		}
	}
	shift, _ := call(chatGetKeyState, 0x10)
	for i, h := range order {
		if focus == h {
			next := (i + 1) % len(order)
			if shift&0x8000 != 0 {
				next = (i + len(order) - 1) % len(order)
			}
			call(chatSetFocus, order[next])
			return true
		}
	}
	return false
}
func chatEditProc(hwnd, msg, wParam, lParam, id, data uintptr) (result uintptr) {
	defer func() {
		if p := recover(); p != nil {
			_, _ = logDiagnostic(fmt.Errorf("native input callback failure: %T", p))
			result = 0
		}
	}()
	c := activeChat
	if c == nil {
		r, _ := call(chatDefSubclass, hwnd, msg, wParam, lParam)
		return r
	}
	switch msg {
	case wmEraseBackground:
		if c.eraseTextBackground(hwnd, wParam) {
			return 1
		}
	case wmPaint:
		c.prepareTextPaint(hwnd)
	case 0x010D:
		c.composing = true
	case 0x010E:
		c.composing = false
	case 0x0100:
		if wParam == 27 {
			c.HideInput()
			c.returnKeyboard()
			return 0
		}
		shift, _ := call(chatGetKeyState, 0x10)
		if wParam == 13 && shift&0x8000 == 0 && !c.composing {
			c.suppressEnter = true
			c.sendText(controlText(c.edit))
			return 0
		}
	case 0x0102:
		if wParam == 13 && c.suppressEnter {
			c.suppressEnter = false
			return 0
		}
	}
	r, _ := call(chatDefSubclass, hwnd, msg, wParam, lParam)
	if textViewportChanged(msg) {
		c.repaintText(hwnd)
	}
	if msg == wmPaint {
		c.paintInputHint(hwnd)
	}
	return r
}
func chatReadProc(hwnd, msg, wParam, lParam, id, data uintptr) uintptr {
	if c := activeChat; c != nil {
		if msg == wmEraseBackground && c.eraseTextBackground(hwnd, wParam) {
			return 1
		}
		if msg == wmPaint {
			c.prepareTextPaint(hwnd)
		}
		if msg == wmMouseWheel || msg == wmLeftDown || msg == 0x0100 {
			c.reading = true
		}
	}
	r, _ := call(chatDefSubclass, hwnd, msg, wParam, lParam)
	if c := activeChat; c != nil && textViewportChanged(msg) {
		c.repaintText(hwnd)
	}
	return r
}
func chatProc(hwnd, msg, wParam, lParam uintptr) (result uintptr) {
	c := activeChat
	defer func() {
		if p := recover(); p != nil {
			_, _ = logDiagnostic(fmt.Errorf("native chat callback failure: %T", p))
			result = 0
		}
	}()
	if c == nil {
		r, _ := call(defWindowProc, hwnd, msg, wParam, lParam)
		return r
	}
	if c.inputSkin != nil && hwnd == c.inputSkin.window || c.replySkin != nil && hwnd == c.replySkin.window {
		if msg == wmPaint {
			call(validateRect, hwnd, 0)
			return 0
		}
		if msg == wmEraseBackground {
			return 1
		}
	}
	switch msg {
	case 0x0100, 0x0104:
		if wParam == 0x79 {
			c.openEmotionDebug()
			return 0
		}
	case 0x002B:
		if lParam == 0 {
			return 0
		}
		var item drawItem
		clipboardCopyMemory.Call(uintptr(unsafe.Pointer(&item)), lParam, unsafe.Sizeof(item))
		c.drawLiveButton(item)
		return 1
	case wmClose:
		if hwnd == c.emotionDebugWindow {
			_, _ = call(showWindow, hwnd, 0)
			c.emotionDebugVisible = false
		} else if c.preview != nil && hwnd == c.preview.window {
			c.closePreview()
		} else if hwnd == c.input {
			c.HideInput()
		} else {
			c.hideReply()
		}
		return 0
	case wmEraseBackground:
		return 1
	case wmPaint:
		var ps paintStruct
		dc, _ := call(chatBeginPaint, hwnd, uintptr(unsafe.Pointer(&ps)))
		var rect rectWin
		_, _ = call(chatGetClientRect, hwnd, uintptr(unsafe.Pointer(&rect)))
		brush := c.brush
		if c.skinFor(hwnd) != nil {
			brush = c.chromaBrush
		}
		_, _ = call(chatFillRect, dc, uintptr(unsafe.Pointer(&rect)), brush)
		if c.preview != nil && hwnd == c.preview.window {
			c.paintPreview(dc)
		}
		_, _ = call(chatEndPaint, hwnd, uintptr(unsafe.Pointer(&ps)))
		runtime.KeepAlive(ps)
		return 0
	case 0x0133, 0x0138:
		if skin := c.skinFor(lParam); skin != nil && skin.brush != 0 {
			p := c.controlPositions[lParam]
			call(skinBrushOrigin, wParam, uintptr(-int(p.X)), uintptr(-int(p.Y)), 0)
			call(chatSetTextColor, wParam, 0x173870)
			call(chatSetBkMode, wParam, 1)
			return skin.brush
		}
		_, _ = call(chatSetTextColor, wParam, 0x263A53)
		_, _ = call(chatSetBkColor, wParam, 0xC9E9FF)
		return c.brush
	case 0x0006:
		if hwnd == c.input && wParam&0xffff == 0 && lParam != c.reply {
			c.HideInput()
		} else if hwnd == c.reply && wParam&0xffff != 0 {
			c.reading = true
		}
		return 0
	case 0x0111:
		if (lParam == c.edit || lParam == c.replyText) && (wParam>>16 == 0x0300 || wParam>>16 == 0x0400 || wParam>>16 == 0x0602) {
			// The EDIT repaints its entire skin on changes, so a manually drawn
			// empty placeholder cannot survive underneath typed or IME text.
			c.repaintText(lParam)
			return 0
		}
		if wParam>>16 == 0 {
			switch wParam & 0xffff {
			case 107, 108, 207, 208:
				target := c.edit
				if wParam&0xffff >= 200 {
					target = c.replyText
					c.reading = true
				}
				lines := 3
				if wParam&0xffff == 107 || wParam&0xffff == 207 {
					lines = -3
				}
				call(chatSend, target, 0x00B6, 0, uintptr(lines))
				return 0
			case 105:
				c.omitNextImage = !c.omitNextImage
				c.shareText = ""
				call(skinInvalidate, c.skipImage, 0, 0)
				return 0
			case 106:
				c.openPreview()
				return 0
			case 303:
				c.sendNativePreview()
				return 0
			case 304:
				c.closePreview()
				return 0
			case 305:
				c.retakePreview()
				return 0
			case 102:
				c.sendText(controlText(c.edit))
			case 202:
				c.primaryAction()
			case 203:
				c.hideReply()
			}
		}
		return 0
	case wmLeftUp:
		if c.preview != nil && hwnd == c.preview.window {
			c.preview.large = !c.preview.large
			c.previewPosition()
			return 0
		}
	case wmMouseWheel:
		if hwnd == c.reply {
			c.reading = true
			delta := int16(wParam >> 16)
			lines := int(-delta/120) * 3
			_, _ = call(chatSend, c.replyText, 0x00B6, 0, uintptr(lines))
			return 0
		}
	}
	r, _ := call(defWindowProc, hwnd, msg, wParam, lParam)
	return r
}
