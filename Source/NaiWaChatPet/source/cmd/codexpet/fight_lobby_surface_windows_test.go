//go:build windows

package main

import (
	"image"
	"runtime"
	"testing"
)

// TestLobbySurfaceBlend guards the compositing path the lobby depends on: a
// lobby surface must really land on a device context. AlphaBlend takes its
// BLENDFUNCTION by value, and passing a pointer instead fails quietly, which
// once cost the whole card layer its background.
func TestLobbySurfaceBlend(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	src := newLobbySurface(8, 8)
	if src == nil {
		t.Fatal("surface allocation failed")
	}
	defer src.close()
	im := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			im.SetNRGBA(x, y, lobbyColor(0xFF0000))
		}
	}
	src.upload(im)

	dst := newLobbySurface(8, 8)
	if dst == nil {
		t.Fatal("destination allocation failed")
	}
	defer dst.close()
	for i := 0; i < len(dst.pixels); i += 4 {
		dst.pixels[i], dst.pixels[i+1], dst.pixels[i+2], dst.pixels[i+3] = 0, 0, 0, 255
	}
	src.blend(dst.dc, 0, 0, 255)
	if dst.pixels[0] != 0 || dst.pixels[1] != 0 || dst.pixels[2] < 200 || dst.pixels[3] != 255 {
		t.Fatalf("opaque blend did not composite: %v", dst.pixels[:4])
	}

	// A half-transparent source must land halfway, proving per-pixel alpha works.
	for i := 0; i < len(im.Pix); i += 4 {
		im.Pix[i+3] = 128
	}
	src.upload(im)
	for i := 0; i < len(dst.pixels); i += 4 {
		dst.pixels[i], dst.pixels[i+1], dst.pixels[i+2], dst.pixels[i+3] = 0, 0, 0, 255
	}
	src.blend(dst.dc, 0, 0, 255)
	if v := dst.pixels[2]; v < 100 || v > 150 {
		t.Fatalf("alpha blend ignored per-pixel alpha: %v", dst.pixels[:4])
	}
}
