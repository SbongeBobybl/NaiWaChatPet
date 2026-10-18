//go:build windows

package main

import (
	"errors"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

// deskIcon is an item on the desktop: what it is and where its picture is.
type deskIcon struct {
	path string
	// x and y are the middle of the icon picture in screen pixels.
	x, y int
}

var (
	iconShell      = syscall.NewLazyDLL("shlwapi.dll").NewProc("StrRetToBufW")
	iconFindWindow = userDLL.NewProc("FindWindowExW")
	iconToScreen   = userDLL.NewProc("ClientToScreen")
	iconFree       = shortcutOLE.NewProc("CoTaskMemFree")
)

// GUIDs of the shell interfaces that lead from the shell's window list to the
// desktop's folder view.
var (
	shellWindowsClass = shortcutGUID{0x9BA05972, 0xF6A8, 0x11CF, [8]byte{0xA4, 0x42, 0x00, 0xA0, 0xC9, 0x0A, 0x8F, 0x39}}
	shellWindowsID    = shortcutGUID{0x85CB6900, 0x4D95, 0x11CF, [8]byte{0x96, 0x0C, 0x00, 0x80, 0xC7, 0xF4, 0xEE, 0x85}}
	serviceProviderID = shortcutGUID{0x6D5140C1, 0x7436, 0x11CE, [8]byte{0x80, 0x34, 0x00, 0xAA, 0x00, 0x60, 0x09, 0xFA}}
	topBrowserService = shortcutGUID{0x4C96BE40, 0x915C, 0x11CF, [8]byte{0x99, 0xD3, 0x00, 0xAA, 0x00, 0x4A, 0xE8, 0x37}}
	shellBrowserID    = shortcutGUID{0x000214E2, 0, 0, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	folderViewID      = shortcutGUID{0xCDE725B0, 0xCCC9, 0x4519, [8]byte{0x91, 0x7E, 0x32, 0x5D, 0x72, 0xFA, 0xB4, 0xCE}}
	shellFolderID     = shortcutGUID{0x000214E6, 0, 0, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
)

// readDesktopIcons asks Explorer where every desktop icon is, through the
// desktop's own folder view: the shell's supported route, with no reading of
// another process's memory. It locks its own thread for COM, so call it from
// a goroutine; a cross-process call made on the pet's thread could re-enter
// its message loop.
func readDesktopIcons() (icons []deskIcon, err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := shortcutOLE.NewProc("CoInitializeEx").Call(0, 2)
	if e := shortcutError("CoInitializeEx", hr); e != nil {
		return nil, e
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
		return nil, e
	}
	defer release(windows)
	// Two empty VARIANTs, then IShellWindows::FindWindowSW for the desktop
	// (SWC_DESKTOP) with its automation object (SWFO_NEEDDISPATCH).
	location, root := new([3]uint64), new([3]uint64)
	hwnd, desk := new(int32), out()
	hr = comMethod(*windows, 15, uintptr(unsafe.Pointer(location)), uintptr(unsafe.Pointer(root)), 8, uintptr(unsafe.Pointer(hwnd)), 1, uintptr(unsafe.Pointer(desk)))
	if hr != 0 || *desk == 0 {
		return nil, errors.New("the desktop window is not available")
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
			return nil, e
		}
	}
	// Item positions are in the desktop list view's own coordinates.
	viewWindow := out()
	if e := shortcutError("IShellView.GetWindow", comMethod(*view, 3, uintptr(unsafe.Pointer(viewWindow)))); e != nil {
		return nil, e
	}
	class := utf("SysListView32")
	list, _, _ := iconFindWindow.Call(*viewWindow, 0, uintptr(unsafe.Pointer(class)), 0)
	runtime.KeepAlive(class)
	if list == 0 {
		return nil, errors.New("the desktop list view is not available")
	}
	spacing := new(pointWin)
	comMethod(*folderView, 12, uintptr(unsafe.Pointer(spacing)))
	count := new(int32)
	// SVGIO_ALLVIEW
	if e := shortcutError("IFolderView.ItemCount", comMethod(*folderView, 7, 2, uintptr(unsafe.Pointer(count)))); e != nil {
		return nil, e
	}
	name := new([1024]uint16)
	strret := new([66]uint64)
	for i := 0; i < int(*count) && i < 4096; i++ {
		item := out()
		if comMethod(*folderView, 6, uintptr(i), uintptr(unsafe.Pointer(item))) != 0 || *item == 0 {
			continue
		}
		at := new(pointWin)
		placed := comMethod(*folderView, 11, *item, uintptr(unsafe.Pointer(at))) == 0
		// SHGDN_FORPARSING gives the full path of a file-system item.
		*strret = [66]uint64{}
		named := comMethod(*folder, 11, *item, 0x8000, uintptr(unsafe.Pointer(strret))) == 0
		if named {
			ok, _, _ := iconShell.Call(uintptr(unsafe.Pointer(strret)), *item, uintptr(unsafe.Pointer(name)), uintptr(len(name)))
			named = ok == 0
		}
		iconFree.Call(*item)
		if !placed || !named {
			continue
		}
		iconToScreen.Call(list, uintptr(unsafe.Pointer(at)))
		// The picture sits centred across the cell, in its upper part.
		icons = append(icons, deskIcon{
			path: syscall.UTF16ToString(name[:]),
			x:    int(at.X) + int(spacing.X)/2,
			y:    int(at.Y) + int(spacing.X)*2/5,
		})
	}
	runtime.KeepAlive(location)
	runtime.KeepAlive(root)
	runtime.KeepAlive(hwnd)
	return icons, nil
}

// iconIndex finds icons by path, ignoring case as Windows does.
func iconIndex(icons []deskIcon) map[string]deskIcon {
	index := make(map[string]deskIcon, len(icons))
	for _, i := range icons {
		index[strings.ToLower(i.path)] = i
	}
	return index
}
