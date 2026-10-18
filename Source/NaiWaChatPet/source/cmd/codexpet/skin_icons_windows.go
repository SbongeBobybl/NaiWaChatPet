//go:build windows

package main

import (
	"runtime"
	"unsafe"
)

var skinPolygon = chatGDI.NewProc("Polygon")
var skinMoveTo = chatGDI.NewProc("MoveToEx")
var skinLineTo = chatGDI.NewProc("LineTo")
var skinCreatePen = chatGDI.NewProc("CreatePen")

func (c *nativeChat) drawScrollArrow(dc uintptr, r rectWin, up bool) {
	x := (r.Left + r.Right) / 2
	y := (r.Top + r.Bottom) / 2
	d := int32(c.px(3))
	sign := int32(1)
	if up {
		sign = -1
	}
	pts := [3]pointWin{{x - d, y - sign*d/2}, {x + d, y - sign*d/2}, {x, y + sign*d}}
	brush, _ := call(chatCreateBrush, 0x899398)
	old, _ := call(chatSelectObject, dc, brush)
	pen, _ := call(skinCreatePen, 0, 1, 0x899398)
	oldPen, _ := call(chatSelectObject, dc, pen)
	call(skinPolygon, dc, uintptr(unsafe.Pointer(&pts[0])), 3)
	runtime.KeepAlive(pts)
	call(chatSelectObject, dc, old)
	call(chatSelectObject, dc, oldPen)
	call(chatDeleteObject, pen)
	call(chatDeleteObject, brush)
}
func (c *nativeChat) drawButtonIcon(dc uintptr, r rectWin, id uint32) {
	x := (r.Left + r.Right) / 2
	y := (r.Top + r.Bottom) / 2
	d := (r.Right - r.Left) / 2
	pen, _ := call(skinCreatePen, 0, uintptr(c.px(2)), 0x173870)
	oldPen, _ := call(chatSelectObject, dc, pen)
	defer func() { call(chatSelectObject, dc, oldPen); call(chatDeleteObject, pen) }()
	line := func(x1, y1, x2, y2 int32) {
		call(skinMoveTo, dc, uintptr(x1), uintptr(y1), 0)
		call(skinLineTo, dc, uintptr(x2), uintptr(y2))
	}
	if id == 102 {
		pts := [5]pointWin{{x - d, y - d/3}, {x + d, y - d}, {x + d/3, y + d}, {x, y + d/4}, {x - d, y - d/3}}
		old, _ := call(chatSelectObject, dc, c.inkBrush)
		call(skinPolygon, dc, uintptr(unsafe.Pointer(&pts[0])), 4)
		runtime.KeepAlive(pts)
		call(chatSelectObject, dc, old)
	} else if id == 105 {
		// Two aligned vector T shapes; no font-dependent symbols or emoji.
		line(x-d, y-d, x+d/3, y-d)
		line(x-d/3, y-d, x-d/3, y+d)
		line(x+d/5, y-d/5, x+d, y-d/5)
		line(x+d*3/5, y-d/5, x+d*3/5, y+d)
	} else {
		for _, p := range []pointWin{{x - d, y - d}, {x + d, y - d}, {x - d, y + d}, {x + d, y + d}} {
			line(p.X, p.Y, p.X+(x-p.X)/2, p.Y)
			line(p.X, p.Y, p.X, p.Y+(y-p.Y)/2)
		}
		for _, dx := range []int32{-d / 2, 0, d / 2} {
			line(x+dx, y, x+dx+1, y)
		}
	}
}
