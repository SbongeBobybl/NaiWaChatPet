//go:build windows

package main

import "unsafe"

// Classic EDIT scrolls existing pixels as an optimization. Skin backgrounds
// stay anchored to the control, so every edited/scrolled viewport must erase
// and redraw all text instead of reusing the previous transparent glyphs.
func (c *nativeChat) eraseTextBackground(hwnd, dc uintptr) bool {
	if hwnd != c.edit && hwnd != c.replyText {
		return false
	}
	var r rectWin
	call(chatGetClientRect, hwnd, uintptr(unsafe.Pointer(&r)))
	return c.paintControlBackground(dc, hwnd, r)
}
func textViewportChanged(msg uintptr) bool {
	switch msg {
	case 0x000C, 0x0005, 0x0100, 0x0102, 0x010F, 0x0114, 0x0115, wmMouseWheel, 0x00B1, 0x00B5, 0x00B6, 0x00B7, 0x0300, 0x0302, 0x0303, 0x0304:
		return true
	}
	return false
}
func (c *nativeChat) repaintText(hwnd uintptr)      { call(skinInvalidate, hwnd, 0, 1) }
func (c *nativeChat) prepareTextPaint(hwnd uintptr) { c.repaintText(hwnd) }
