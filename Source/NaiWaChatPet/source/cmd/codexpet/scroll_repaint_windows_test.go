//go:build windows

package main

import (
	"bytes"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"unsafe"
)

func captureNativeEdit(t *testing.T, hwnd uintptr) []byte {
	t.Helper()
	call(userDLL.NewProc("UpdateWindow"), hwnd)
	var r rectWin
	call(chatGetClientRect, hwnd, uintptr(unsafe.Pointer(&r)))
	w, h := int(r.Right-r.Left), int(r.Bottom-r.Top)
	dc, _ := call(skinGetDC, hwnd)
	defer call(skinReleaseDC, hwnd, dc)
	copyDC, _ := call(skinCreateDC, dc)
	defer call(skinDeleteDC, copyDC)
	header := skinDIBHeader{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, Bits: 32}
	var bits uintptr
	bitmap, e := call(skinCreateDIB, dc, uintptr(unsafe.Pointer(&header)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bitmap == 0 || bits == 0 {
		t.Fatal(e)
	}
	defer call(chatDeleteObject, bitmap)
	old, _ := call(chatSelectObject, copyDC, bitmap)
	defer call(chatSelectObject, copyDC, old)
	if ok, e := call(skinBitBlt, copyDC, 0, 0, uintptr(w), uintptr(h), dc, 0, 0, 0x00CC0020); ok == 0 {
		t.Fatal(e)
	}
	call(chatGDI.NewProc("GdiFlush"))
	data := append([]byte(nil), unsafe.Slice((*byte)(unsafe.Pointer(bits)), w*h*4)...)
	runtime.KeepAlive(header)
	return data
}

func TestNativeInputAndReplyScrollingDoesNotAccumulateGlyphs(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	assets, e := loadAssets(bundledAssets, "runtime.json")
	if e != nil {
		t.Fatal(e)
	}
	class, title := utf("STATIC"), utf("NaiWaChatPet text repaint verification")
	owner, e := call(createWindow, wsExToolWindow, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)), wsPopup, 10, 10, 153, 204, 0, 0, 0, 0)
	if owner == 0 {
		t.Fatal(e)
	}
	defer call(destroyWindow, owner)
	a := desktopHost{assets: assets, hwnd: owner, x: 10, y: 10, w: 153, h: 204, dpi: 96}
	c, e := newNativeChat(&a)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	c.inputVisible, c.replyVisible = true, true
	if e = c.position(); e != nil {
		t.Fatal(e)
	}
	call(showWindow, c.input, 4)
	c.showSkin(c.input, true)
	call(showWindow, c.reply, 4)
	c.showSkin(c.reply, true)
	defer func() { call(showWindow, c.input, 0); call(showWindow, c.reply, 0) }()
	var lines []string
	for i := 0; i < 50; i++ {
		lines = append(lines, fmt.Sprintf("Line %02d: scrolling text stays clear. 奶蛙文字不重叠。", i))
	}
	text := strings.Join(lines, "\r\n")
	for _, row := range []struct {
		name   string
		window uintptr
	}{{"input", c.edit}, {"reply", c.replyText}} {
		setText(row.window, text)
		call(chatSend, row.window, 0x00B1, 0, 0)
		baseline := captureNativeEdit(t, row.window)
		for i := 0; i < 20; i++ {
			call(chatSend, row.window, 0x00B6, 0, 7)
			captureNativeEdit(t, row.window)
			back := -7
			call(chatSend, row.window, 0x00B6, 0, uintptr(back))
			actual := captureNativeEdit(t, row.window)
			if !bytes.Equal(baseline, actual) {
				t.Fatalf("%s scroll loop %d left stale pixels instead of restoring the identical viewport", row.name, i)
			}
		}
		if controlText(row.window) != text {
			t.Fatalf("%s scrolling changed its text", row.name)
		}
		t.Log(row.name + ": 20 down/up loops restored identical native pixels and preserved text")
	}
}
