//go:build windows

package main

import (
	"errors"
	"runtime"
	"syscall"
	"unsafe"

	"codexpet/reconstruction/winlayer"
)

const (
	wsExTransparent = 0x00000020
	wsExNoActivate  = 0x08000000
)

// sprite is a small click-through layered window: the ground shadow, a fly, a
// burst or the score. It never takes focus and never appears in the taskbar.
type sprite struct {
	hwnd    uintptr
	surface *winlayer.Surface
	w, h    int
	shown   bool
}

type spriteClass struct {
	name   *uint16
	module uintptr
}

func registerSpriteClass(module uintptr) (*spriteClass, error) {
	name, err := syscall.UTF16PtrFromString("NaiWaChatPetSprite")
	if err != nil {
		return nil, err
	}
	// Sprites handle no messages of their own, so the default procedure serves.
	klass := classWin{Size: uint32(unsafe.Sizeof(classWin{})), Procedure: defWindowProc.Addr(), Instance: module, ClassName: name}
	atom, e := call(registerClass, uintptr(unsafe.Pointer(&klass)))
	runtime.KeepAlive(&klass)
	runtime.KeepAlive(name)
	if atom == 0 {
		return nil, failure("RegisterClassExW(sprite)", e)
	}
	return &spriteClass{name, module}, nil
}

func (c *spriteClass) close() {
	if c != nil {
		_, _ = call(unregisterClass, uintptr(unsafe.Pointer(c.name)), c.module)
		runtime.KeepAlive(c.name)
	}
}

func (c *spriteClass) open() (*sprite, error) {
	if c == nil {
		return nil, errors.New("sprite window class is unavailable")
	}
	style := uintptr(wsExLayered | wsExTransparent | wsExTopmost | wsExToolWindow | wsExNoActivate)
	hwnd, e := call(createWindow, style, uintptr(unsafe.Pointer(c.name)), 0, wsPopup, 0, 0, 1, 1, 0, 0, c.module, 0)
	runtime.KeepAlive(c.name)
	if hwnd == 0 {
		return nil, failure("CreateWindowExW(sprite)", e)
	}
	return &sprite{hwnd: hwnd}, nil
}

// draw shows w by h premultiplied BGRA pixels with their top-left at (x, y).
func (s *sprite) draw(x, y, w, h int, bgra []byte) error {
	if s == nil || w < 1 || h < 1 || len(bgra) != w*h*4 {
		return errors.New("sprite: invalid picture")
	}
	if s.surface == nil {
		surface, err := winlayer.New(w, h)
		if err != nil {
			if surface != nil {
				_ = surface.Close()
			}
			return err
		}
		s.surface = surface
	} else if w != s.w || h != s.h {
		if err := s.surface.Resize(w, h); err != nil {
			return err
		}
	}
	s.w, s.h = w, h
	if err := s.surface.Present(s.hwnd, x, y, bgra); err != nil {
		return err
	}
	if !s.shown {
		_, _ = call(showWindow, s.hwnd, 4)
		s.shown = true
	}
	return nil
}

// under slides the sprite just beneath another window, so the shadow sits
// behind the pet rather than over its feet.
func (s *sprite) under(hwnd uintptr) {
	if s != nil && s.shown {
		_, _ = call(setWindowPos, s.hwnd, hwnd, 0, 0, 0, 0, 0x0001|0x0002|0x0010)
	}
}

func (s *sprite) hide() {
	if s != nil && s.shown {
		_, _ = call(showWindow, s.hwnd, 0)
		s.shown = false
	}
}

func (s *sprite) close() {
	if s == nil {
		return
	}
	if s.surface != nil {
		_ = s.surface.Close()
		s.surface = nil
	}
	if s.hwnd != 0 {
		_, _ = call(destroyWindow, s.hwnd)
		s.hwnd = 0
	}
}
