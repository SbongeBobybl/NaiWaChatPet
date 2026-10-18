//go:build windows

package main

import (
	"codexpet/reconstruction/compositor"
	"errors"
	"runtime"
	"syscall"
	"unsafe"
)

const trayMessage = 0x8000 + 51

var trayNotify = shellDLL.NewProc("Shell_NotifyIconW")
var trayCreateIcon = userDLL.NewProc("CreateIcon")
var trayDestroyIcon = userDLL.NewProc("DestroyIcon")
var trayRegisterMessage = userDLL.NewProc("RegisterWindowMessageW")

type trayData struct {
	Size                uint32
	Window              uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Version             uint32
	InfoTitle           [64]uint16
	InfoFlags           uint32
	GUID                [16]byte
	BalloonIcon         uintptr
}
type nativeTray struct {
	data    trayData
	restart uint32
}

func (a *desktopHost) initTray() (*nativeTray, error) {
	frame, e := a.assets.Bodies.BodyCopy("idle", 0)
	if e != nil {
		return nil, e
	}
	small, e := compositor.Resize(frame, 24, 32)
	if e != nil {
		return nil, e
	}
	bits := make([]byte, 32*32*4)
	mask := make([]byte, 4*32)
	pixels := compositor.BGRA(small)
	for y := 0; y < 32; y++ {
		copy(bits[(y*32+4)*4:(y*32+28)*4], pixels[y*24*4:(y+1)*24*4])
	}
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			if bits[(y*32+x)*4+3] == 0 {
				mask[y*4+x/8] |= byte(0x80 >> uint(x%8))
			}
		}
	}
	module, _ := call(moduleHandle, 0)
	icon, _ := call(trayCreateIcon, module, 32, 32, 1, 32, uintptr(unsafe.Pointer(&mask[0])), uintptr(unsafe.Pointer(&bits[0])))
	runtime.KeepAlive(mask)
	runtime.KeepAlive(bits)
	if icon == 0 {
		return nil, errors.New("tray icon creation failed")
	}
	t := &nativeTray{}
	t.data = trayData{Window: a.hwnd, ID: 1, Flags: 1 | 2 | 4, Callback: trayMessage, Icon: icon}
	t.data.Size = uint32(unsafe.Sizeof(t.data))
	copy(t.data.Tip[:], syscall.StringToUTF16("奶蛙 · 单击聊天，右键菜单"))
	name := utf("TaskbarCreated")
	restart, _ := call(trayRegisterMessage, uintptr(unsafe.Pointer(name)))
	runtime.KeepAlive(name)
	t.restart = uint32(restart)
	if ok, _ := call(trayNotify, 0, uintptr(unsafe.Pointer(&t.data))); ok == 0 {
		t.Close()
		return nil, errors.New("tray registration failed")
	}
	return t, nil
}
func (t *nativeTray) Close() {
	if t != nil {
		_, _ = call(trayNotify, 2, uintptr(unsafe.Pointer(&t.data)))
		_, _ = call(trayDestroyIcon, t.data.Icon)
	}
}
func (t *nativeTray) Handle(a *desktopHost, msg, lParam uint32) bool {
	if msg == t.restart && msg != 0 {
		_, _ = call(trayNotify, 0, uintptr(unsafe.Pointer(&t.data)))
		return true
	}
	if msg != trayMessage {
		return false
	}
	switch lParam {
	case wmLeftUp, wmDoubleClick:
		if a.chat != nil {
			_ = a.chat.ShowInput()
		}
	case wmRightUp:
		if e := a.menu(); e != nil {
			_, _ = logDiagnostic(e)
		}
	}
	return true
}
