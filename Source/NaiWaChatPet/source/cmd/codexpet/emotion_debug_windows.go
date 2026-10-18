//go:build windows

package main

import (
	"runtime"
	"unsafe"
)

// Explicit, independent developer window. Ordinary speech never contains this text.
func (c *nativeChat) openEmotionDebug() {
	if companionService == nil {
		return
	}
	if c.emotionDebugWindow == 0 {
		title := utf("奶蛙 · 情绪调试面板")
		work := c.workArea()
		w, h := c.px(510), c.px(570)
		w = min(w, work.W-20)
		h = min(h, work.H-20)
		hwnd, e := call(createWindow, 0x00040000, uintptr(unsafe.Pointer(c.className)), uintptr(unsafe.Pointer(title)), 0x00C80000, uintptr(work.X+(work.W-w)/2), uintptr(work.Y+(work.H-h)/2), uintptr(w), uintptr(h), c.owner.hwnd, 0, c.module, 0)
		runtime.KeepAlive(title)
		if hwnd == 0 {
			_, _ = logDiagnostic(failure("CreateWindowExW(emotion debug)", e))
			return
		}
		c.emotionDebugWindow = hwnd
		class := utf("EDIT")
		child, e := call(createWindow, 0, uintptr(unsafe.Pointer(class)), 0, 0x40000000|0x10000000|0x10000|0x0004|0x0040|0x0800|0x00200000, uintptr(c.px(16)), uintptr(c.px(12)), uintptr(w-c.px(32)), uintptr(h-c.px(66)), hwnd, 401, c.module, 0)
		runtime.KeepAlive(class)
		if child == 0 {
			_, _ = logDiagnostic(failure("CreateWindowExW(emotion debug text)", e))
			_, _ = call(destroyWindow, hwnd)
			c.emotionDebugWindow = 0
			return
		}
		c.emotionDebugText = child
		_, _ = call(chatSend, child, 0x0030, c.font, 1)
	}
	c.emotionDebugVisible = true
	c.emotionDebugLast = ""
	c.tickEmotionDebug()
	_, _ = call(showWindow, c.emotionDebugWindow, 5)
	_, _ = call(foregroundWindow, c.emotionDebugWindow)
}

func (c *nativeChat) tickEmotionDebug() {
	if !c.emotionDebugVisible || companionService == nil {
		return
	}
	text := companionService.EmotionDebug().Text()
	if text != c.emotionDebugLast {
		setText(c.emotionDebugText, text)
		c.emotionDebugLast = text
	}
}
