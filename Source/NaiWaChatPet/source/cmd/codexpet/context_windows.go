//go:build windows

package main

import (
	"codexpet/reconstruction/companion"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

var foreMu sync.Mutex
var lastForeground uintptr
var getForeground = userDLL.NewProc("GetForegroundWindow")
var contextOpen = kernelDLL.NewProc("OpenProcess")
var contextQuery = kernelDLL.NewProc("QueryFullProcessImageNameW")
var contextClose = kernelDLL.NewProc("CloseHandle")

func appName(h uintptr) string {
	var pid uint32
	scopePID.Call(h, uintptr(unsafe.Pointer(&pid)))
	p, _, _ := contextOpen.Call(0x1000, 0, uintptr(pid))
	if p == 0 {
		return "unknown"
	}
	defer contextClose.Call(p)
	b := make([]uint16, 1024)
	n := uint32(len(b))
	ok, _, _ := contextQuery.Call(p, 0, uintptr(unsafe.Pointer(&b[0])), uintptr(unsafe.Pointer(&n)))
	if ok == 0 {
		return "unknown"
	}
	return filepath.Base(syscall.UTF16ToString(b[:n]))
}
func rememberForeground() {
	h, _, _ := getForeground.Call()
	if h == 0 {
		return
	}
	var pid uint32
	scopePID.Call(h, uintptr(unsafe.Pointer(&pid)))
	if pid == uint32(os.Getpid()) {
		return
	}
	foreMu.Lock()
	lastForeground = h
	foreMu.Unlock()
}
func resolveForeground() uintptr {
	rememberForeground()
	foreMu.Lock()
	defer foreMu.Unlock()
	return lastForeground
}
func excludedApp(name, list string) bool {
	for _, v := range strings.FieldsFunc(strings.ToLower(list), func(r rune) bool { return r == '\n' || r == '\r' || r == ',' || r == ';' }) {
		if strings.TrimSpace(v) != "" && strings.Contains(strings.ToLower(name), strings.TrimSpace(v)) {
			return true
		}
	}
	return false
}

var excludeList string
var excludedVisible bool
var excludeCallback = syscall.NewCallback(func(h, data uintptr) uintptr {
	visible, _, _ := scopeVisible.Call(h)
	minimized, _, _ := scopeMinimized.Call(h)
	if visible != 0 && minimized == 0 && excludedApp(appName(h), excludeList) {
		excludedVisible = true
		return 0
	}
	return 1
})

func nativeScreenContext(scope, list string) (companion.ScreenContext, error) {
	h := resolveForeground()
	if scope == "window:active" {
		if h == 0 {
			return companion.ScreenContext{}, errors.New("尚无可用的前台应用，请先激活需要观察的窗口")
		}
		scope = "window:" + strconv.FormatUint(uint64(h), 10)
	}
	if strings.HasPrefix(scope, "window:") {
		id, e := strconv.ParseUint(strings.TrimPrefix(scope, "window:"), 10, 64)
		if e != nil {
			return companion.ScreenContext{}, e
		}
		h = uintptr(id)
	}
	c := companion.ScreenContext{Scope: scope}
	if h != 0 {
		c.App = appName(h)
		c.Window = controlText(h)
		r := []rune(c.Window)
		if len(r) > 100 {
			c.Window = string(r[:100])
		}
		c.Excluded = excludedApp(c.App, list)
	}
	if strings.HasPrefix(scope, "monitor:") && strings.TrimSpace(list) != "" {
		scopeMu.Lock()
		excludeList = list
		excludedVisible = false
		scopeEnumWindows.Call(excludeCallback, 0)
		c.Excluded = c.Excluded || excludedVisible
		scopeMu.Unlock()
	}
	return c, nil
}
