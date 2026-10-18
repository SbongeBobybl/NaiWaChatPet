//go:build windows

package main

import (
	"image"
	"math"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

type buttonMotion struct {
	value, target float64
	hover         bool
	releaseAt     time.Time
}

var buttonStretch = chatGDI.NewProc("StretchDIBits")
var buttonTrack = userDLL.NewProc("TrackMouseEvent")

func (c *nativeChat) initButtons() {
	c.buttonMotions = make(map[uintptr]*buttonMotion)
	for _, h := range []uintptr{c.send, c.skipImage, c.previewButton, c.primary, c.closeButton} {
		c.buttonMotions[h] = &buttonMotion{}
		call(chatSubclass, h, syscall.NewCallback(chatButtonProc), 3, 0)
	}
}
func (c *nativeChat) tickButtons() {
	for h, m := range c.buttonMotions {
		if !m.releaseAt.IsZero() && time.Now().After(m.releaseAt) {
			m.target = 0
			m.releaseAt = time.Time{}
		}
		if math.Abs(m.value-m.target) > .005 {
			m.value += (m.target - m.value) * .42
			if math.Abs(m.value-m.target) < .005 {
				m.value = m.target
			}
			call(skinInvalidate, h, 0, 0)
		}
	}
}
func chatButtonProc(hwnd, msg, wp, lp, id, data uintptr) uintptr {
	if c := activeChat; c != nil {
		if m := c.buttonMotions[hwnd]; m != nil {
			switch msg {
			case wmLeftDown:
				c.keyboardFocus = false
				m.target, m.value, m.releaseAt = 1, .75, time.Time{}
			case wmLeftUp:
				m.target, m.value, m.releaseAt = 1, 1, time.Now().Add(70*time.Millisecond)
			case wmCaptureChanged:
				if m.releaseAt.IsZero() {
					m.target = 0
				}
			case 0x0008:
				m.target, m.releaseAt = 0, time.Time{}
			case 0x0100:
				c.keyboardFocus = true
				if wp == 32 || wp == 13 {
					m.target = 1
					m.value = .75
				}
			case 0x0101:
				if wp == 32 || wp == 13 {
					m.target, m.value, m.releaseAt = 1, 1, time.Now().Add(70*time.Millisecond)
				}
			case wmMouseMove:
				if !m.hover {
					m.hover = true
					t := struct {
						Size, Flags uint32
						Window      uintptr
						Time        uint32
					}{Size: uint32(unsafe.Sizeof(struct {
						Size, Flags uint32
						Window      uintptr
						Time        uint32
					}{})), Flags: 2, Window: hwnd}
					call(buttonTrack, uintptr(unsafe.Pointer(&t)))
				}
			case 0x02A3:
				m.hover = false
			}
			if msg == wmLeftDown || msg == wmLeftUp || msg == wmMouseMove || msg == 0x02A3 || msg == 0x0007 || msg == 0x0008 {
				call(skinInvalidate, hwnd, 0, 0)
			}
		}
	}
	r, _ := call(chatDefSubclass, hwnd, msg, wp, lp)
	return r
}
func (c *nativeChat) drawLiveButton(item drawItem) {
	r := item.Rect
	if item.ID == 107 || item.ID == 108 || item.ID == 207 || item.ID == 208 {
		c.paintControlBackground(item.DC, item.Window, r)
		c.drawScrollArrow(item.DC, r, item.ID == 107 || item.ID == 207)
		return
	}
	font := c.buttonFont
	if item.ID == 102 {
		font = c.sendFont
	}
	if font == 0 {
		font = c.font
	}
	old, _ := call(chatSelectObject, item.DC, font)
	defer call(chatSelectObject, item.DC, old)
	w, h := int(r.Right-r.Left), int(r.Bottom-r.Top)
	press, hover := 0.0, false
	if m := c.buttonMotions[item.Window]; m != nil {
		press, hover = m.value, m.hover
	}
	if item.State&1 != 0 {
		press = math.Max(.22, press)
	}
	var bg *image.NRGBA
	if s := c.skinFor(item.Window); s != nil {
		bg = s.scaled
	}
	p := c.controlPositions[item.Window]
	gold := item.ID == 102 || item.ID == 105 && c.omitNextImage
	bitmap, dy := buttonFace(bg, image.Pt(int(p.X), int(p.Y)), w, h, float64(c.dpi)/96, press, gold, hover, item.State&0x10 != 0 && c.keyboardFocus)
	header := skinDIBHeader{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, Bits: 32}
	if len(bitmap) > 0 {
		call(buttonStretch, item.DC, 0, 0, uintptr(w), uintptr(h), 0, 0, uintptr(w), uintptr(h), uintptr(unsafe.Pointer(&bitmap[0])), uintptr(unsafe.Pointer(&header)), 0, 0x00CC0020)
	}
	runtime.KeepAlive(bitmap)
	call(chatSetTextColor, item.DC, 0x173870)
	call(chatSetBkMode, item.DC, 1)
	title := utf(controlText(item.Window))
	measure := rectWin{}
	call(chatDrawText, item.DC, uintptr(unsafe.Pointer(title)), ^uintptr(0), uintptr(unsafe.Pointer(&measure)), 0x0420)
	iconW, gap := 0, 0
	if item.ID == 102 || item.ID == 105 || item.ID == 106 {
		iconW, gap = c.px(12), c.px(5)
	}
	textW := min(int(measure.Right), w-c.px(12)-iconW-gap)
	left := (w - textW - iconW - gap) / 2
	centerY := h/2 + dy
	if iconW > 0 {
		c.drawButtonIcon(item.DC, rectWin{int32(left), int32(centerY - iconW/2), int32(left + iconW), int32(centerY + iconW/2)}, item.ID)
	}
	textRect := rectWin{int32(left + iconW + gap), int32(centerY - int(measure.Bottom)/2), int32(left + iconW + gap + textW), int32(centerY + (int(measure.Bottom)+1)/2)}
	call(chatDrawText, item.DC, uintptr(unsafe.Pointer(title)), ^uintptr(0), uintptr(unsafe.Pointer(&textRect)), 0x0824)
	runtime.KeepAlive(title)
}
