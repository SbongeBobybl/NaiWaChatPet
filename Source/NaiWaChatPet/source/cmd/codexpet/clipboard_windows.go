//go:build windows

package main

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	cfUnicodeText = 13
	gmMoveable    = 0x0002
)

var (
	clipboardOpen       = userDLL.NewProc("OpenClipboard")
	clipboardClose      = userDLL.NewProc("CloseClipboard")
	clipboardEmpty      = userDLL.NewProc("EmptyClipboard")
	clipboardSetData    = userDLL.NewProc("SetClipboardData")
	globalAlloc         = kernelDLL.NewProc("GlobalAlloc")
	globalLock          = kernelDLL.NewProc("GlobalLock")
	globalUnlock        = kernelDLL.NewProc("GlobalUnlock")
	globalFree          = kernelDLL.NewProc("GlobalFree")
	clipboardCopyMemory = syscall.NewLazyDLL("ntdll.dll").NewProc("RtlMoveMemory")
)

// copyChatURL writes the current ephemeral localhost chat URL to the clipboard
// only after the user expressly clicks the fallback menu entry. No API key is
// copied or written. Clipboard data is handed to Windows upon success.
func copyChatURL(url string) error {
	encoded, err := syscall.UTF16FromString(url)
	if err != nil {
		return err
	}
	opened, e := call(clipboardOpen, 0)
	if opened == 0 {
		return failure("OpenClipboard", e)
	}
	defer func() { _, _ = call(clipboardClose) }()

	n := uintptr(len(encoded) * 2)
	hmem, e := call(globalAlloc, gmMoveable, n)
	if hmem == 0 {
		return failure("GlobalAlloc(clipboard)", e)
	}
	transferred := false
	defer func() {
		if !transferred {
			_, _ = call(globalFree, hmem)
		}
	}()
	ptr, e := call(globalLock, hmem)
	if ptr == 0 {
		return failure("GlobalLock(clipboard)", e)
	}
	if e := clipboardCopyMemory.Find(); e != nil {
		_, _ = call(globalUnlock, hmem)
		return e
	}
	clipboardCopyMemory.Call(ptr, uintptr(unsafe.Pointer(&encoded[0])), n)
	runtime.KeepAlive(encoded)
	_, _ = call(globalUnlock, hmem)

	if ok, e := call(clipboardEmpty); ok == 0 {
		return failure("EmptyClipboard", e)
	}
	if ok, e := call(clipboardSetData, cfUnicodeText, hmem); ok == 0 {
		return failure("SetClipboardData", e)
	}
	transferred = true
	return nil
}

func copyChatURLFromMenu() error {
	if companionService == nil {
		return fmt.Errorf("chat service is unavailable")
	}
	return copyChatURL(companionService.URL())
}
