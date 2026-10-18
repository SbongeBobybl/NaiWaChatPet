//go:build windows

package main

import (
	"runtime"
	"unsafe"
)

func (c *nativeChat) updateScrollControls() {
	for _, row := range []struct {
		edit, up, down uintptr
		visible        bool
	}{{c.edit, c.inputUp, c.inputDown, c.inputVisible}, {c.replyText, c.replyUp, c.replyDown, c.replyVisible}} {
		var format rectWin
		call(chatSend, row.edit, 0x00B2, 0, uintptr(unsafe.Pointer(&format))) // EM_GETRECT
		dc, _ := call(skinGetDC, row.edit)
		if dc == 0 {
			continue
		}
		old, _ := call(chatSelectObject, dc, c.font)
		measure := rectWin{}
		text := utf("国")
		call(chatDrawText, dc, uintptr(unsafe.Pointer(text)), ^uintptr(0), uintptr(unsafe.Pointer(&measure)), 0x0420)
		runtime.KeepAlive(text)
		call(chatSelectObject, dc, old)
		call(skinReleaseDC, row.edit, dc)
		lines, _ := call(chatSend, row.edit, 0x00BA, 0, 0)
		visible := max(1, int(format.Bottom-format.Top)/max(1, int(measure.Bottom)))
		show := uintptr(0)
		if row.visible && int(lines) > visible {
			show = 4
		}
		call(showWindow, row.up, show)
		call(showWindow, row.down, show)
	}
}
