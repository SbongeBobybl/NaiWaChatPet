//go:build windows

package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"runtime"
	"syscall"
	"unsafe"
)

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}
type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}

var (
	captureUser32   = syscall.NewLazyDLL("user32.dll")
	captureGdi32    = syscall.NewLazyDLL("gdi32.dll")
	capGetDC        = captureUser32.NewProc("GetDC")
	capReleaseDC    = captureUser32.NewProc("ReleaseDC")
	capGetMetrics   = captureUser32.NewProc("GetSystemMetrics")
	capCreateDC     = captureGdi32.NewProc("CreateCompatibleDC")
	capDeleteDC     = captureGdi32.NewProc("DeleteDC")
	capCreateBitmap = captureGdi32.NewProc("CreateCompatibleBitmap")
	capDeleteObject = captureGdi32.NewProc("DeleteObject")
	capSelectObject = captureGdi32.NewProc("SelectObject")
	capBitBlt       = captureGdi32.NewProc("BitBlt")
	capGetDIBits    = captureGdi32.NewProc("GetDIBits")
)

// captureScreenJPEG captures only the primary display. The returned bytes stay
// in memory, are downscaled (up to 1280x720), and are never written to disk.
// Capture is invoked only by explicit user request / enabled observation.
func captureRectJPEG(ctx context.Context, x, y, w, h int, sourceHWND uintptr) ([]byte, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if w < 16 || h < 16 || w > 16384 || h > 16384 || int64(w)*int64(h) > 64*1024*1024 {
		return nil, errors.New("主屏幕分辨率异常")
	}
	desktop, _, _ := capGetDC.Call(0)
	if desktop == 0 {
		return nil, errors.New("无法读取屏幕 DC")
	}
	defer capReleaseDC.Call(0, desktop)
	mem, _, _ := capCreateDC.Call(desktop)
	if mem == 0 {
		return nil, errors.New("无法创建截图缓冲区")
	}
	defer capDeleteDC.Call(mem)
	bmp, _, _ := capCreateBitmap.Call(desktop, uintptr(w), uintptr(h))
	if bmp == 0 {
		return nil, errors.New("无法创建截图位图")
	}
	defer capDeleteObject.Call(bmp)
	old, _, _ := capSelectObject.Call(mem, bmp)
	if old == 0 || old == ^uintptr(0) {
		return nil, errors.New("无法绑定截图位图")
	}
	var r uintptr
	if sourceHWND != 0 {
		r, _, _ = captureUser32.NewProc("PrintWindow").Call(sourceHWND, mem, 2)
	} else {
		r, _, _ = capBitBlt.Call(mem, 0, 0, uintptr(w), uintptr(h), desktop, uintptr(x), uintptr(y), 0x00CC0020)
	}
	if r == 0 {
		capSelectObject.Call(mem, old)
		return nil, errors.New("Windows 截屏失败")
	}
	// GetDIBits requires that the bitmap is NOT selected into a DC.
	capSelectObject.Call(mem, old)
	info := bitmapInfo{Header: bitmapInfoHeader{Size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32, Compression: 0}}
	pixels := make([]byte, w*h*4)
	lines, _, _ := capGetDIBits.Call(desktop, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&info)), 0)
	runtime.KeepAlive(pixels)
	runtime.KeepAlive(&info)
	if lines != uintptr(h) {
		return nil, errors.New("Windows 无法读取截图像素")
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if sourceHWND != 0 {
		nonblack := false
		for i := 0; i < len(pixels); i += 4 {
			if int(pixels[i])+int(pixels[i+1])+int(pixels[i+2]) != 0 {
				nonblack = true
				break
			}
		}
		if !nonblack {
			return nil, errors.New("这个窗口无法可靠捕获，请选择显示器范围并预览")
		}
	}
	maxW, maxH := 1280, 720
	factor := 1.0
	if float64(w)/float64(maxW) > factor {
		factor = float64(w) / float64(maxW)
	}
	if float64(h)/float64(maxH) > factor {
		factor = float64(h) / float64(maxH)
	}
	dw, dh := int(float64(w)/factor), int(float64(h)/factor)
	if dw < 1 || dh < 1 {
		return nil, errors.New("截图尺寸无效")
	}
	dest := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		sy := y * h / dh
		for x := 0; x < dw; x++ {
			sx := x * w / dw
			i := (sy*w + sx) * 4
			dest.SetRGBA(x, y, color.RGBA{R: pixels[i+2], G: pixels[i+1], B: pixels[i], A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dest, &jpeg.Options{Quality: 70}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
