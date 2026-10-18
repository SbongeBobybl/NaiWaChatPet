//go:build windows

package main

import (
	"codexpet/reconstruction/desktoplayout"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

func layoutSnapshot(fv, folder, list uintptr) (desktoplayout.Snapshot, map[string]uintptr, error) {
	s := desktoplayout.Snapshot{ID: fmt.Sprint(time.Now().UnixNano())}
	items := map[string]uintptr{}
	fail := func(e error) (desktoplayout.Snapshot, map[string]uintptr, error) {
		for _, p := range items {
			iconFree.Call(p)
		}
		return s, nil, e
	}
	spacing := new(pointWin)
	if e := shortcutError("GetSpacing", comMethod(fv, 12, uintptr(unsafe.Pointer(spacing)))); e != nil {
		return fail(e)
	}
	s.Spacing = desktoplayout.Point{X: spacing.X, Y: spacing.Y}
	hr := comMethod(fv, 14)
	if hr != 0 && hr != 1 {
		return fail(errors.New("无法读取自动排列状态"))
	}
	s.AutoArrange = hr == 0
	dpi, _, _ := userDLL.NewProc("GetDpiForWindow").Call(list)
	s.DPI = uint32(dpi)
	// The callback is cached globally; Windows callback slots cannot be freed.
	state := &layoutMonitorState{list: list}
	layoutMonitorMu.Lock()
	activeLayoutMonitors = state
	ok, _, _ := userDLL.NewProc("EnumDisplayMonitors").Call(0, 0, layoutMonitorCallback, 0)
	activeLayoutMonitors = nil
	layoutMonitorMu.Unlock()
	runtime.KeepAlive(state)
	s.Monitors = state.monitors
	if ok == 0 || len(s.Monitors) == 0 {
		return fail(errors.New("无法读取显示器工作区"))
	}
	sort.Slice(s.Monitors, func(i, j int) bool { return s.Monitors[i].ID < s.Monitors[j].ID })
	n := new(int32)
	if e := shortcutError("ItemCount", comMethod(fv, 7, 2, uintptr(unsafe.Pointer(n)))); e != nil {
		return fail(e)
	}
	if *n > 4096 {
		return fail(errors.New("图标数量超过安全上限"))
	}
	for i := int32(0); i < *n; i++ {
		pidl := new(uintptr)
		if comMethod(fv, 6, uintptr(i), uintptr(unsafe.Pointer(pidl))) != 0 || *pidl == 0 {
			return fail(errors.New("图标列表变化，请重新扫描"))
		}
		at := new(pointWin)
		name := new([32768]uint16)
		strret := new([66]uint64)
		named := comMethod(folder, 11, *pidl, 0x8000, uintptr(unsafe.Pointer(strret))) == 0
		if named {
			r, _, _ := iconShell.Call(uintptr(unsafe.Pointer(strret)), *pidl, uintptr(unsafe.Pointer(name)), 32768)
			named = r == 0
		}
		if !named || comMethod(fv, 11, *pidl, uintptr(unsafe.Pointer(at))) != 0 {
			iconFree.Call(*pidl)
			return fail(errors.New("无法安全读取全部图标，停止布局"))
		}
		key := syscall.UTF16ToString(name[:])
		if _, dup := items[key]; dup {
			iconFree.Call(*pidl)
			return fail(errors.New("Shell 标识重复"))
		}
		items[key] = *pidl
		display := filepath.Base(key)
		virtual := !filepath.IsAbs(key) || strings.HasPrefix(key, "::")
		category := "references"
		identity := key
		if virtual {
			display = "系统图标"
			category = "system"
		} else if info, e := os.Stat(key); e == nil {
			var identityError error
			identity, identityError = layoutFileIdentity(key)
			if identityError != nil {
				virtual = true
			}
			if info.IsDir() {
				category = "projects"
			} else if ext := strings.ToLower(filepath.Ext(key)); ext == ".lnk" || ext == ".url" || ext == ".exe" {
				category = "apps"
			} else if strings.Contains(strings.ToLower(display), "截图") {
				category = "temp"
			}
		} else {
			virtual = true
		}
		s.Icons = append(s.Icons, desktoplayout.Icon{Key: key, Name: display, Category: category, Identity: identity, Position: desktoplayout.Point{X: at.X, Y: at.Y}, Fixed: virtual})
	}
	sort.Slice(s.Icons, func(i, j int) bool { return s.Icons[i].Key < s.Icons[j].Key })
	return s, items, nil
}

type layoutMonitorState struct {
	list     uintptr
	monitors []desktoplayout.Monitor
}

var layoutMonitorMu sync.Mutex
var activeLayoutMonitors *layoutMonitorState

var layoutMonitorCallback = syscall.NewCallback(func(h, dc, rc, data uintptr) uintptr {
	state := activeLayoutMonitors
	if state == nil {
		return 0
	}
	mi := new(monitorInfo)
	mi.Size = uint32(unsafe.Sizeof(*mi))
	ok, _, _ := chatMonitorInfo.Call(h, uintptr(unsafe.Pointer(mi)))
	if ok == 0 {
		return 0
	}
	a := pointWin{X: mi.Work.Left, Y: mi.Work.Top}
	b := pointWin{X: mi.Work.Right, Y: mi.Work.Bottom}
	toClient := userDLL.NewProc("ScreenToClient")
	toClient.Call(state.list, uintptr(unsafe.Pointer(&a)))
	toClient.Call(state.list, uintptr(unsafe.Pointer(&b)))
	state.monitors = append(state.monitors, desktoplayout.Monitor{ID: fmt.Sprintf("%d,%d,%d,%d", mi.Monitor.Left, mi.Monitor.Top, mi.Monitor.Right, mi.Monitor.Bottom), Area: desktoplayout.Rect{Left: a.X, Top: a.Y, Right: b.X, Bottom: b.Y}, Primary: mi.Flags&1 != 0})
	return 1
})

func readLayout() (s desktoplayout.Snapshot, e error) {
	e = withLayoutView(func(fv, folder, list uintptr) error {
		var items map[string]uintptr
		var err error
		s, items, err = layoutSnapshot(fv, folder, list)
		for _, p := range items {
			iconFree.Call(p)
		}
		return err
	})
	return
}
func applyLayout(p desktoplayout.Plan) ([]string, error) {
	if e := desktoplayout.Validate(p); e != nil {
		return nil, e
	}
	var done []string
	err := withLayoutView(func(fv, folder, list uintptr) error {
		current, items, e := layoutSnapshot(fv, folder, list)
		if e != nil {
			return e
		}
		defer func() {
			for _, pidl := range items {
				iconFree.Call(pidl)
			}
		}()
		if current.AutoArrange || !desktoplayout.Compatible(p.Snapshot, current) {
			return errors.New("桌面设置变化，请重新预览")
		}
		if len(current.Icons) != len(p.Snapshot.Icons) {
			return errors.New("桌面图标列表变化，请重新预览")
		}
		for i, a := range current.Icons {
			b := p.Snapshot.Icons[i]
			if a.Key != b.Key || a.Position != b.Position || a.Identity != b.Identity {
				return errors.New("图标已移动或替换，请重新预览")
			}
		}
		var applyError error
		for _, m := range p.Moves {
			pidl, ok := items[m.Key]
			if !ok {
				return errors.New("图标已消失")
			}
			pt := pointWin{X: m.Target.X, Y: m.Target.Y}
			hr := comMethod(fv, 16, 1, uintptr(unsafe.Pointer(&pidl)), uintptr(unsafe.Pointer(&pt)), 0x80)
			runtime.KeepAlive(pt)
			runtime.KeepAlive(pidl)
			if e := shortcutError("SelectAndPositionItems", hr); e != nil {
				applyError = e
				break
			}
		}
		time.Sleep(200 * time.Millisecond)
		for _, m := range p.Moves {
			pt := new(pointWin)
			if comMethod(fv, 11, items[m.Key], uintptr(unsafe.Pointer(pt))) == 0 && pt.X == m.Target.X && pt.Y == m.Target.Y {
				done = append(done, m.Key)
			}
		}
		if len(done) != len(p.Moves) {
			return errors.New("Explorer 未保留部分目标坐标；请查看实际结果后恢复或重新预览")
		}
		return applyError
	})
	return done, err
}

func layoutFileIdentity(path string) (string, error) {
	p, e := syscall.UTF16PtrFromString(path)
	if e != nil {
		return "", e
	}
	h, e := syscall.CreateFile(p, 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS|syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return "", e
	}
	defer syscall.CloseHandle(h)
	var info syscall.ByHandleFileInformation
	if e = syscall.GetFileInformationByHandle(h, &info); e != nil {
		return "", e
	}
	if info.FileAttributes&0x400 != 0 {
		return "", errors.New("重解析项保持原位")
	}
	return fmt.Sprintf("%d:%d:%d:%d:%d:%d", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow, info.FileSizeHigh, info.FileSizeLow, info.LastWriteTime.Nanoseconds()), nil
}
func withLayoutView(fn func(uintptr, uintptr, uintptr) error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := shortcutOLE.NewProc("CoInitializeEx").Call(0, 2)
	if e := shortcutError("CoInitializeEx", hr); e != nil {
		return e
	}
	defer shortcutOLE.NewProc("CoUninitialize").Call()
	// Heap cells for every out-parameter: the GC never moves the heap, so
	// their addresses stay valid across the calls into Explorer.
	out := func() *uintptr { return new(uintptr) }
	release := func(p *uintptr) {
		if *p != 0 {
			comMethod(*p, 2)
		}
	}
	windows := out()
	hr, _, _ = shortcutOLE.NewProc("CoCreateInstance").Call(uintptr(unsafe.Pointer(&shellWindowsClass)), 0, 0x17, uintptr(unsafe.Pointer(&shellWindowsID)), uintptr(unsafe.Pointer(windows)))
	if e := shortcutError("CoCreateInstance(ShellWindows)", hr); e != nil {
		return e
	}
	defer release(windows)
	// Two empty VARIANTs, then IShellWindows::FindWindowSW for the desktop
	// (SWC_DESKTOP) with its automation object (SWFO_NEEDDISPATCH).
	location, root := new([3]uint64), new([3]uint64)
	hwnd, desk := new(int32), out()
	hr = comMethod(*windows, 15, uintptr(unsafe.Pointer(location)), uintptr(unsafe.Pointer(root)), 8, uintptr(unsafe.Pointer(hwnd)), 1, uintptr(unsafe.Pointer(desk)))
	if hr != 0 || *desk == 0 {
		return errors.New("the desktop window is not available")
	}
	defer release(desk)
	provider, browser, view, folderView, folder := out(), out(), out(), out(), out()
	defer release(provider)
	defer release(browser)
	defer release(view)
	defer release(folderView)
	defer release(folder)
	steps := []struct {
		name string
		hr   func() uintptr
	}{
		{"IServiceProvider", func() uintptr {
			return comMethod(*desk, 0, uintptr(unsafe.Pointer(&serviceProviderID)), uintptr(unsafe.Pointer(provider)))
		}},
		{"QueryService(ShellBrowser)", func() uintptr {
			return comMethod(*provider, 3, uintptr(unsafe.Pointer(&topBrowserService)), uintptr(unsafe.Pointer(&shellBrowserID)), uintptr(unsafe.Pointer(browser)))
		}},
		{"QueryActiveShellView", func() uintptr { return comMethod(*browser, 15, uintptr(unsafe.Pointer(view))) }},
		{"IFolderView", func() uintptr {
			return comMethod(*view, 0, uintptr(unsafe.Pointer(&folderViewID)), uintptr(unsafe.Pointer(folderView)))
		}},
		{"IFolderView.GetFolder", func() uintptr {
			return comMethod(*folderView, 5, uintptr(unsafe.Pointer(&shellFolderID)), uintptr(unsafe.Pointer(folder)))
		}},
	}
	for _, s := range steps {
		if e := shortcutError(s.name, s.hr()); e != nil {
			return e
		}
	}
	// Item positions are in the desktop list view's own coordinates.
	viewWindow := out()
	if e := shortcutError("IShellView.GetWindow", comMethod(*view, 3, uintptr(unsafe.Pointer(viewWindow)))); e != nil {
		return e
	}
	class := utf("SysListView32")
	list, _, _ := iconFindWindow.Call(*viewWindow, 0, uintptr(unsafe.Pointer(class)), 0)
	runtime.KeepAlive(class)
	if list == 0 {
		return errors.New("the desktop list view is not available")
	}

	err := fn(*folderView, *folder, list)
	runtime.KeepAlive(location)
	runtime.KeepAlive(root)
	runtime.KeepAlive(hwnd)
	return err
}
