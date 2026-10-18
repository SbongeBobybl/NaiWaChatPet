//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

type shortcutGUID struct {
	A    uint32
	B, C uint16
	D    [8]byte
}

var shortcutOLE = syscall.NewLazyDLL("ole32.dll")

func comMethod(object uintptr, index uintptr, args ...uintptr) uintptr {
	table := *(*uintptr)(unsafe.Pointer(object))
	method := *(*uintptr)(unsafe.Pointer(table + index*unsafe.Sizeof(uintptr(0))))
	values := append([]uintptr{object}, args...)
	r, _, _ := syscall.SyscallN(method, values...)
	return r
}
func shortcutError(op string, hr uintptr) error {
	if int32(hr) < 0 {
		return fmt.Errorf("%s HRESULT 0x%x", op, uint32(hr))
	}
	return nil
}
func writeShortcut(executable, path string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := shortcutOLE.NewProc("CoInitializeEx").Call(0, 2)
	if e := shortcutError("CoInitializeEx", hr); e != nil {
		return e
	}
	defer shortcutOLE.NewProc("CoUninitialize").Call()
	tail := [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}
	class := shortcutGUID{A: 0x00021401, D: tail}
	iid := shortcutGUID{A: 0x000214f9, D: tail}
	var link uintptr
	hr, _, _ = shortcutOLE.NewProc("CoCreateInstance").Call(uintptr(unsafe.Pointer(&class)), 0, 1, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&link)))
	if e := shortcutError("CoCreateInstance", hr); e != nil {
		return e
	}
	defer comMethod(link, 2)
	for _, set := range []struct {
		index uintptr
		text  string
	}{{20, executable}, {9, filepath.Dir(executable)}, {7, "NaiWaChatPet · 奶蛙桌面陪伴"}} {
		p := utf(set.text)
		hr = comMethod(link, set.index, uintptr(unsafe.Pointer(p)))
		runtime.KeepAlive(p)
		if e := shortcutError("IShellLinkW", hr); e != nil {
			return e
		}
	}
	p := utf(executable)
	hr = comMethod(link, 17, uintptr(unsafe.Pointer(p)), 0)
	runtime.KeepAlive(p)
	if e := shortcutError("SetIconLocation", hr); e != nil {
		return e
	}
	persistID := shortcutGUID{A: 0x0000010b, D: tail}
	var persist uintptr
	hr = comMethod(link, 0, uintptr(unsafe.Pointer(&persistID)), uintptr(unsafe.Pointer(&persist)))
	if e := shortcutError("IPersistFile", hr); e != nil {
		return e
	}
	defer comMethod(persist, 2)
	p = utf(path)
	hr = comMethod(persist, 6, uintptr(unsafe.Pointer(p)), 1)
	runtime.KeepAlive(p)
	return shortcutError("IPersistFile.Save", hr)
}
func ensureDesktopShortcut() error {
	if startMode == "demo" {
		return nil
	}
	executable, e := os.Executable()
	if e != nil {
		return e
	}
	desktop, e := desktopFolder()
	if e != nil {
		return e
	}
	return writeShortcut(executable, filepath.Join(desktop, petShortcut))
}
