//go:build windows

package main

import (
	"bytes"
	"codexpet/reconstruction/companion"
	"codexpet/reconstruction/compositor"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"runtime"
	"time"
	"unsafe"
)

type previewResult struct {
	generation uint64
	id         string
	image      image.Image
	err        error
}
type nativePreview struct {
	generation                                 uint64
	window, scope, send, cancel, retry, status uintptr
	choices                                    []companion.CaptureScope
	results                                    chan previewResult
	cancelWork                                 context.CancelFunc
	image                                      image.Image
	id                                         string
	large, visible, busy                       bool
	rect                                       rectWin
	pixels                                     []byte
}

var previewStretch = chatGDI.NewProc("StretchDIBits")
var previewInvalidate = userDLL.NewProc("InvalidateRect")

func (c *nativeChat) initPreview() error {
	p := &nativePreview{results: make(chan previewResult, 1)}
	c.preview = p
	title := utf("奶蛙截图预览")
	style := uintptr(wsExLayered | wsExToolWindow | wsExTopmost)
	if startMode == "demo" {
		style = wsExLayered | wsExTopmost | 0x40000
	}
	h, e := call(createWindow, style, uintptr(unsafe.Pointer(c.className)), uintptr(unsafe.Pointer(title)), wsPopup|0x02000000, 0, 0, 640, 460, c.owner.hwnd, 0, c.module, 0)
	runtime.KeepAlive(title)
	if h == 0 {
		return failure("CreateWindowExW(preview)", e)
	}
	p.window = h
	_, _ = call(chatSetAlpha, h, 0, 255, 2)
	child := func(class, text string, style, id uintptr) uintptr {
		cl, t := utf(class), utf(text)
		h, _ := call(createWindow, 0, uintptr(unsafe.Pointer(cl)), uintptr(unsafe.Pointer(t)), 0x40000000|0x10000000|0x10000|style, 0, 0, 10, 10, p.window, id, c.module, 0)
		runtime.KeepAlive(cl)
		runtime.KeepAlive(t)
		return h
	}
	p.scope = child("COMBOBOX", "", 3|0x200000, 301)
	p.status = child("STATIC", "", 0, 302)
	p.send = child("BUTTON", "发送", 11, 303)
	p.cancel = child("BUTTON", "取消", 11, 304)
	p.retry = child("BUTTON", "重新截图", 11, 305)
	for _, v := range []uintptr{p.scope, p.status, p.send, p.cancel, p.retry} {
		if v == 0 {
			return fmt.Errorf("preview control creation failed")
		}
		_, _ = call(chatSend, v, 0x0030, c.font, 1)
	}
	return nil
}
func (c *nativeChat) openPreview() {
	traceStartup("native preview requested")
	if companionService == nil {
		return
	}
	if c.preview == nil {
		if e := c.initPreview(); e != nil {
			c.showLocalError(e.Error())
			return
		}
	}
	p := c.preview
	if c.inputVisible {
		c.draft = controlText(c.edit)
	}
	p.visible = true
	p.large = false
	p.choices = companionService.CaptureScopes()
	_, _ = call(chatSend, p.scope, 0x014B, 0, 0)
	selected := 0
	sharing := companionService.Sharing()
	for i, v := range p.choices {
		t := utf(v.Label)
		_, _ = call(chatSend, p.scope, 0x0143, 0, uintptr(unsafe.Pointer(t)))
		runtime.KeepAlive(t)
		if v.ID == sharing.Scope {
			selected = i
		}
	}
	_, _ = call(chatSend, p.scope, 0x014E, uintptr(selected), 0)
	c.previewPosition()
	_, _ = call(showWindow, p.window, 1)
	traceStartup("native preview window shown")
	_, _ = call(foregroundWindow, p.window)
	c.retakePreview()
}
func (c *nativeChat) previewPosition() {
	p := c.preview
	if p == nil || !p.visible {
		return
	}
	work := c.workArea()
	w, h := c.px(640), c.px(460)
	if p.large {
		w, h = work.W-c.px(32), work.H-c.px(32)
	}
	w = min(w, work.W-c.px(24))
	h = min(h, work.H-c.px(24))
	r := struct{ X, Y, W, H int }{work.X + (work.W-w)/2, work.Y + (work.H-h)/2, w, h}
	_, _ = call(setWindowPos, p.window, 0, uintptr(r.X), uintptr(r.Y), uintptr(w), uintptr(h), 0x0014)
	region, _ := call(chatRoundRgn, 0, 0, uintptr(w+1), uintptr(h+1), uintptr(c.px(22)), uintptr(c.px(22)))
	_, _ = call(chatSetWindowRgn, p.window, region, 1)
	c.moveControl(p.scope, c.px(16), c.px(12), w-c.px(32), c.px(240))
	c.moveControl(p.status, c.px(16), c.px(49), w-c.px(32), c.px(46))
	y := h - c.px(46)
	c.moveControl(p.send, c.px(16), y, c.px(80), c.px(30))
	c.moveControl(p.cancel, c.px(108), y, c.px(80), c.px(30))
	c.moveControl(p.retry, w-c.px(140), y, c.px(124), c.px(30))
	p.rect = rectWin{int32(c.px(16)), int32(c.px(100)), int32(w - c.px(16)), int32(y - c.px(10))}
	_, _ = call(previewInvalidate, p.window, 0, 1)
}
func (c *nativeChat) retakePreview() {
	p := c.preview
	if p == nil || companionService == nil {
		return
	}
	if p.cancelWork != nil {
		p.cancelWork()
	}
	companionService.DiscardNativePreview()
	p.generation++
	generation := p.generation
	p.id = ""
	p.image = nil
	p.busy = true
	p.pixels = nil
	index, _ := call(chatSend, p.scope, 0x0147, 0, 0)
	if int(index) >= len(p.choices) {
		return
	}
	scope := p.choices[index].ID
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	p.cancelWork = cancel
	setText(p.status, "正在截取所选范围；尚未上传。点击图片可查看大图。")
	_, _ = call(previewInvalidate, p.window, 0, 1)
	go func() {
		id, b, e := companionService.Preview(ctx, scope)
		var im image.Image
		if e == nil {
			im, _, e = image.Decode(bytes.NewReader(b))
		}
		if ctx.Err() != nil {
			return
		}
		select {
		case p.results <- previewResult{generation, id, im, e}:
		case <-ctx.Done():
		}
	}()
}
func (c *nativeChat) previewTick() {
	p := c.preview
	if p == nil {
		return
	}
	select {
	case r := <-p.results:
		if r.generation != p.generation {
			return
		}
		if !p.visible {
			return
		}
		p.busy = false
		if r.err != nil {
			setText(p.status, r.err.Error())
			return
		}
		p.id = r.id
		p.image = r.image
		traceStartup("native preview pixels ready in memory")
		p.pixels = nil
		setText(p.status, "尚未上传，发送目标："+companionService.Sharing().Target+"。请检查敏感信息后发送。")
		_, _ = call(previewInvalidate, p.window, 0, 1)
	default:
	}
}
func (c *nativeChat) closePreview() {
	p := c.preview
	if p == nil {
		return
	}
	p.visible = false
	p.generation++
	p.id = ""
	p.image = nil
	p.pixels = nil
	if p.cancelWork != nil {
		p.cancelWork()
	}
	_, _ = call(showWindow, p.window, 0)
	if companionService != nil {
		companionService.DiscardNativePreview()
	}
}
func (c *nativeChat) sendNativePreview() {
	p := c.preview
	if p == nil || p.id == "" || p.busy {
		return
	}
	text := c.draft
	if text == "" {
		text = "你看我在干什么？请结合画面自然回应，只关注与当前任务有关的内容。"
	}
	if _, e := companionService.SendPreview(text, p.id); e != nil {
		setText(p.status, e.Error())
		return
	}
	p.id = ""
	c.draft = ""
	setText(c.edit, "")
	c.closePreview()
	c.HideInput()
}
func (c *nativeChat) paintPreview(dc uintptr) {
	p := c.preview
	if p == nil || p.image == nil {
		return
	}
	r := p.rect
	w, h := int(r.Right-r.Left), int(r.Bottom-r.Top)
	b := p.image.Bounds()
	factor := min(float64(w)/float64(b.Dx()), float64(h)/float64(b.Dy()))
	iw, ih := max(1, int(float64(b.Dx())*factor)), max(1, int(float64(b.Dy())*factor))
	im, e := compositor.Resize(p.image, iw, ih)
	if e != nil {
		return
	}
	pixels := compositor.BGRA(im)
	info := bitmapInfo{Header: bitmapInfoHeader{Size: 40, Width: int32(iw), Height: -int32(ih), Planes: 1, BitCount: 32}}
	_, _ = call(previewStretch, dc, uintptr(int(r.Left)+(w-iw)/2), uintptr(int(r.Top)+(h-ih)/2), uintptr(iw), uintptr(ih), 0, 0, uintptr(iw), uintptr(ih), uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&info)), 0, 0x00CC0020)
	runtime.KeepAlive(pixels)
	runtime.KeepAlive(info)
	if companionService != nil && p.id != "" {
		companionService.MarkNativePreviewViewed(p.id)
	}
}
