//go:build windows

package main

import (
	"codexpet/reconstruction/companion"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

var scopeMu sync.Mutex
var scopeOutput []companion.CaptureScope
var scopeEnumMonitors = captureUser32.NewProc("EnumDisplayMonitors")
var scopeEnumWindows = captureUser32.NewProc("EnumWindows")
var scopeWindowRect = captureUser32.NewProc("GetWindowRect")
var scopeVisible = captureUser32.NewProc("IsWindowVisible")
var scopeMinimized = captureUser32.NewProc("IsIconic")
var scopePID = captureUser32.NewProc("GetWindowThreadProcessId")
var scopeMonitorCallback = syscall.NewCallback(func(m, dc, rect, data uintptr) uintptr {
	info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ok, _, _ := chatMonitorInfo.Call(m, uintptr(unsafe.Pointer(&info))); ok != 0 {
		scopeOutput = append(scopeOutput, companion.CaptureScope{ID: "monitor:" + strconv.FormatUint(uint64(m), 10), Label: fmt.Sprintf("显示器 %d · %d×%d", len(scopeOutput), info.Monitor.Right-info.Monitor.Left, info.Monitor.Bottom-info.Monitor.Top)})
	}
	return 1
})
var scopeWindowCallback = syscall.NewCallback(func(h, data uintptr) uintptr {
	visible, _, _ := scopeVisible.Call(h)
	minimized, _, _ := scopeMinimized.Call(h)
	if visible == 0 || minimized != 0 {
		return 1
	}
	var pid uint32
	scopePID.Call(h, uintptr(unsafe.Pointer(&pid)))
	if pid == uint32(os.Getpid()) {
		return 1
	}
	title := controlText(h)
	if strings.TrimSpace(title) == "" {
		return 1
	}
	var r rectWin
	ok, _, _ := scopeWindowRect.Call(h, uintptr(unsafe.Pointer(&r)))
	if ok == 0 || r.Right-r.Left < 100 || r.Bottom-r.Top < 60 {
		return 1
	}
	rr := []rune(title)
	if len(rr) > 80 {
		title = string(rr[:80]) + "…"
	}
	scopeOutput = append(scopeOutput, companion.CaptureScope{ID: "window:" + strconv.FormatUint(uint64(h), 10), Label: "窗口 · " + title})
	if len(scopeOutput) > 128 {
		return 0
	}
	return 1
})

func listCaptureScopes() []companion.CaptureScope {
	scopeMu.Lock()
	defer scopeMu.Unlock()
	scopeOutput = []companion.CaptureScope{{ID: "monitor:primary", Label: "主显示器"}}
	scopeOutput = append(scopeOutput, companion.CaptureScope{ID: "window:active", Label: "当前活动窗口（跟随）"})
	scopeEnumMonitors.Call(0, 0, scopeMonitorCallback, 0)
	scopeEnumWindows.Call(scopeWindowCallback, 0)
	return append([]companion.CaptureScope(nil), scopeOutput...)
}
func captureScreenJPEG() ([]byte, error) {
	return captureScreenScopeJPEG(context.Background(), "monitor:primary")
}
func captureScreenScopeJPEG(ctx context.Context, scope string) ([]byte, error) {
	if scope == "window:active" {
		h := resolveForeground()
		if h == 0 {
			return nil, errors.New("没有可观察的活动窗口")
		}
		scope = "window:" + strconv.FormatUint(uint64(h), 10)
	}
	if scope == "monitor:primary" {
		w, _, _ := capGetMetrics.Call(0)
		h, _, _ := capGetMetrics.Call(1)
		return captureRectJPEG(ctx, 0, 0, int(w), int(h), 0)
	}
	found := false
	for _, s := range listCaptureScopes() {
		if s.ID == scope {
			found = true
			break
		}
	}
	if !found {
		return nil, errors.New("所选窗口或显示器已经失效")
	}
	parts := strings.SplitN(scope, ":", 2)
	if len(parts) != 2 {
		return nil, errors.New("截屏范围无效")
	}
	id, e := strconv.ParseUint(parts[1], 10, 64)
	if e != nil {
		return nil, e
	}
	var r rectWin
	var source uintptr
	if parts[0] == "monitor" {
		info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
		ok, _, _ := chatMonitorInfo.Call(uintptr(id), uintptr(unsafe.Pointer(&info)))
		if ok == 0 {
			return nil, errors.New("显示器信息不可用")
		}
		r = info.Monitor
	} else {
		ok, _, _ := scopeWindowRect.Call(uintptr(id), uintptr(unsafe.Pointer(&r)))
		if ok == 0 {
			return nil, errors.New("窗口信息不可用")
		}
		source = uintptr(id)
	}
	return captureRectJPEG(ctx, int(r.Left), int(r.Top), int(r.Right-r.Left), int(r.Bottom-r.Top), source)
}
