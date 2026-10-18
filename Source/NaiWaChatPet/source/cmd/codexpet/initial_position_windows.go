//go:build windows

package main

import "math"

func (a *desktopHost) reserveInitialChatSpace() {
	c := nativeChat{owner: a, dpi: a.dpi}
	work := c.workArea()
	ground := int(math.Round(a.assets.Config.Drop.GroundY * float64(a.h) / float64(a.assets.Canvas.Dy())))
	a.y = max(work.Y, min(a.y, work.Bottom()-c.px(122)-ground))
	a.x = max(work.X, min(a.x, work.Right()-a.w))
}
