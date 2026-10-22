//go:build windows

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"unsafe"
)

// Open the Windows-owned permission UI; never add/disable firewall rules or
// change the network category behind the user's back. Hotspots may be Public.
func (g *gamePanel) openLANPermissions() {
	file := utf(filepath.Join(os.Getenv("SystemRoot"), "System32", "control.exe"))
	args := utf("/name Microsoft.WindowsFirewall /page pageConfigureApps")
	verb := utf("open")
	result, _, _ := shellExecute.Call(g.hwnd, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), uintptr(unsafe.Pointer(args)), 0, 1)
	runtime.KeepAlive(file)
	runtime.KeepAlive(args)
	runtime.KeepAlive(verb)
	if result <= 32 {
		g.statusError = "请打开 Windows 安全中心 → 防火墙 → 允许应用。\n为当前版本奶蛙勾选当前网络；手机热点可能属于“公用”。"
		return
	}
	g.statusError = "房主在系统页面找到或添加当前版本 NaiWaChatPet.exe，允许当前网络。\n热点可能是“公用”；保存后让好友刷新房间，无需关闭防火墙。"
}
