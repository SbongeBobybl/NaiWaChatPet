//go:build windows

package main

import (
	"bytes"
	"codexpet/reconstruction/compositor"
	"codexpet/reconstruction/desktopchat"
	"codexpet/reconstruction/winlayer"
	"embed"
	"image"
	"image/draw"
	"image/png"
	"runtime"
	"unsafe"
)

//go:embed ui_assets/input.png ui_assets/reply.png
var chatSkinAssets embed.FS

const chatChroma = 0x010001 // color-key used only by native control overlays
var skinCreateDC = chatGDI.NewProc("CreateCompatibleDC")
var skinCreateDIB = chatGDI.NewProc("CreateDIBSection")
var skinCreatePattern = chatGDI.NewProc("CreatePatternBrush")
var skinDeleteDC = chatGDI.NewProc("DeleteDC")
var skinBitBlt = chatGDI.NewProc("BitBlt")
var skinBrushOrigin = chatGDI.NewProc("SetBrushOrgEx")
var skinGetDC = userDLL.NewProc("GetDC")
var skinReleaseDC = userDLL.NewProc("ReleaseDC")
var skinInvalidate = userDLL.NewProc("InvalidateRect")

type skinDIBHeader struct {
	Size                   uint32
	Width, Height          int32
	Planes, Bits           uint16
	Compression, SizeImage uint32
	X, Y                   int32
	Used, Important        uint32
}
type nativeSkin struct {
	window, dc, bitmap, oldBitmap, brush uintptr
	surface                              *winlayer.Surface
	source                               *image.NRGBA
	scaled                               *image.NRGBA
	pixels                               []byte
	width, height, x, y, alpha           int
	mirror                               bool
}

func (c *nativeChat) newSkin(name string) (*nativeSkin, error) {
	b, e := chatSkinAssets.ReadFile("ui_assets/" + name + ".png")
	if e != nil {
		return nil, e
	}
	im, e := png.Decode(bytes.NewReader(b))
	if e != nil {
		return nil, e
	}
	s := &nativeSkin{source: skinStraight(im), alpha: 255}
	title := utf("")
	w, e := call(createWindow, wsExLayered|wsExToolWindow|wsExTopmost|0x20|0x08000000, uintptr(unsafe.Pointer(c.className)), uintptr(unsafe.Pointer(title)), wsPopup, 0, 0, 1, 1, c.owner.hwnd, 0, c.module, 0)
	runtime.KeepAlive(title)
	if w == 0 {
		return nil, failure("CreateWindowExW(skin)", e)
	}
	s.window = w
	return s, nil
}
func (s *nativeSkin) releaseBitmap() {
	if s.brush != 0 {
		call(chatDeleteObject, s.brush)
		s.brush = 0
	}
	if s.dc != 0 && s.oldBitmap != 0 {
		call(chatSelectObject, s.dc, s.oldBitmap)
	}
	if s.bitmap != 0 {
		call(chatDeleteObject, s.bitmap)
		s.bitmap = 0
	}
	if s.dc != 0 {
		call(skinDeleteDC, s.dc)
		s.dc = 0
	}
}
func (s *nativeSkin) Close() {
	if s == nil {
		return
	}
	if s.window != 0 {
		call(destroyWindow, s.window)
		s.window = 0
	}
	s.releaseBitmap()
	if s.surface != nil {
		s.surface.Close()
	}
}
func (s *nativeSkin) resize(w, h int, mirror bool) error {
	if s.width == w && s.height == h && s.mirror == mirror {
		return nil
	}
	resized, e := compositor.Resize(s.source, w, h)
	if e != nil {
		return e
	}
	im := skinStraight(resized)
	if mirror {
		for y := 0; y < h; y++ {
			for x := 0; x < w/2; x++ {
				a, b := im.NRGBAAt(x, y), im.NRGBAAt(w-1-x, y)
				im.SetNRGBA(x, y, b)
				im.SetNRGBA(w-1-x, y, a)
			}
		}
	}
	s.pixels = compositor.BGRA(im)
	s.scaled = im
	if s.surface == nil {
		s.surface, e = winlayer.New(w, h)
	} else {
		e = s.surface.Resize(w, h)
	}
	if e != nil {
		return e
	}
	s.releaseBitmap()
	s.dc, _ = call(skinCreateDC, 0)
	header := skinDIBHeader{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, Bits: 32}
	var bits uintptr
	s.bitmap, e = call(skinCreateDIB, s.dc, uintptr(unsafe.Pointer(&header)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	runtime.KeepAlive(header)
	if s.bitmap == 0 || bits == 0 {
		s.releaseBitmap()
		return failure("CreateDIBSection(skin)", e)
	}
	rgb := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := im.NRGBAAt(x, y)
			if p.A < 16 {
				p.R, p.G, p.B = 1, 0, 1
			}
			i := (y*w + x) * 4
			rgb[i], rgb[i+1], rgb[i+2], rgb[i+3] = p.B, p.G, p.R, 255
		}
	}
	clipboardCopyMemory.Call(bits, uintptr(unsafe.Pointer(&rgb[0])), uintptr(len(rgb)))
	runtime.KeepAlive(rgb)
	s.oldBitmap, _ = call(chatSelectObject, s.dc, s.bitmap)
	s.brush, e = call(skinCreatePattern, s.bitmap)
	if s.brush == 0 {
		s.releaseBitmap()
		return failure("CreatePatternBrush(skin)", e)
	}
	s.width, s.height, s.mirror = w, h, mirror
	return nil
}
func (s *nativeSkin) present() error {
	if s.surface == nil {
		return nil
	}
	pixels := s.pixels
	if s.alpha != 255 {
		pixels = append([]byte(nil), pixels...)
		for i := range pixels {
			pixels[i] = byte(int(pixels[i]) * s.alpha / 255)
		}
	}
	return s.surface.Present(s.window, s.x, s.y, pixels)
}
func (c *nativeChat) initSkins() error {
	var e error
	c.chromaBrush, _ = call(chatCreateBrush, chatChroma)
	c.controlPositions = map[uintptr]pointWin{}
	c.inputSkin, e = c.newSkin("input")
	if e != nil {
		return e
	}
	c.replySkin, e = c.newSkin("reply")
	if e != nil {
		return e
	}
	call(chatSetAlpha, c.input, chatChroma, 255, 3)
	call(chatSetAlpha, c.reply, chatChroma, 255, 3)
	return nil
}
func (c *nativeChat) skinFor(window uintptr) *nativeSkin {
	if window == c.input || window == c.edit || window == c.send || window == c.shareStatus || window == c.skipImage || window == c.previewButton || window == c.inputUp || window == c.inputDown {
		return c.inputSkin
	}
	if window == c.reply || window == c.replyText || window == c.primary || window == c.closeButton || window == c.replyUp || window == c.replyDown {
		return c.replySkin
	}
	return nil
}
func (c *nativeChat) placeSkin(hwnd uintptr, r desktopchat.Rect, p desktopchat.Placement) error {
	s := c.skinFor(hwnd)
	if e := s.resize(r.W, r.H, p.Tail == "right"); e != nil {
		return e
	}
	s.x, s.y = r.X, r.Y
	if e := s.present(); e != nil {
		return e
	}
	call(setWindowPos, s.window, hwnd, uintptr(r.X), uintptr(r.Y), uintptr(r.W), uintptr(r.H), 0x10)
	call(skinInvalidate, hwnd, 0, 1)
	return nil
}
func (c *nativeChat) showSkin(hwnd uintptr, visible bool) {
	s := c.skinFor(hwnd)
	if s == nil {
		return
	}
	show := uintptr(0)
	if visible {
		show = 4
	}
	call(showWindow, s.window, show)
	if visible {
		call(setWindowPos, s.window, hwnd, uintptr(s.x), uintptr(s.y), uintptr(s.width), uintptr(s.height), 0x10)
	}
}
func (c *nativeChat) chatOpacity(alpha int) {
	call(chatSetAlpha, c.reply, chatChroma, uintptr(alpha), 3)
	if c.replySkin != nil {
		c.replySkin.alpha = alpha
		c.replySkin.present()
	}
}
func (c *nativeChat) paintControlBackground(dc, window uintptr, r rectWin) bool {
	s := c.skinFor(window)
	if s == nil || s.dc == 0 {
		return false
	}
	p := c.controlPositions[window]
	call(skinBitBlt, dc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), s.dc, uintptr(int(p.X)+int(r.Left)), uintptr(int(p.Y)+int(r.Top)), 0x00CC0020)
	return true
}
func (c *nativeChat) paintInputHint(hwnd uintptr) {
	if hwnd != c.edit || controlText(hwnd) != "" {
		return
	}
	dc, _ := call(skinGetDC, hwnd)
	if dc == 0 {
		return
	}
	defer call(skinReleaseDC, hwnd, dc)
	var r rectWin
	call(chatGetClientRect, hwnd, uintptr(unsafe.Pointer(&r)))
	c.paintControlBackground(dc, hwnd, r)
	old, _ := call(chatSelectObject, dc, c.font)
	defer call(chatSelectObject, dc, old)
	call(chatSetTextColor, dc, 0x71838A)
	call(chatSetBkMode, dc, 1)
	p := utf("和奶蛙说点什么…")
	r.Left += int32(c.px(3))
	r.Top += int32(c.px(3))
	call(chatDrawText, dc, uintptr(unsafe.Pointer(p)), ^uintptr(0), uintptr(unsafe.Pointer(&r)), 0x0000)
	runtime.KeepAlive(p)
}

func skinStraight(im image.Image) *image.NRGBA {
	result := image.NewNRGBA(image.Rect(0, 0, im.Bounds().Dx(), im.Bounds().Dy()))
	draw.Draw(result, result.Bounds(), im, im.Bounds().Min, draw.Src)
	return result
}
