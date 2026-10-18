//go:build windows

package winlayer

import (
	"bytes"
	"errors"
	"runtime"
	"syscall"
	"testing"
	"unsafe"
)

type gdiBitmap struct {
	Type, Width, Height, WidthBytes int32
	Planes, BitsPixel               uint16
	Bits                            unsafe.Pointer
}
type dibSection struct {
	Bitmap    gdiBitmap
	Header    bitmapInfoHeader
	Bitfields [3]uint32
	Section   uintptr
	Offset    uint32
}

func inspectDIB(t *testing.T, bitmap uintptr) dibSection {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	getObject := gdi32.NewProc("GetObjectW")
	var info dibSection
	n, err := invoke(getObject, bitmap, unsafe.Sizeof(info), uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(info)
	if n != unsafe.Sizeof(info) {
		t.Fatalf("GetObjectW(DIBSECTION) returned %d: %v", n, err)
	}
	return info
}

func TestRealDIBIsTopDown32BitWithExactStride(t *testing.T) {
	s, err := New(5, 3)
	if err != nil {
		if s != nil {
			_ = s.Close()
		}
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	info := inspectDIB(t, s.native.bitmap)
	// GetObject on this Windows build reports absolute DIB height. Verify row
	// orientation through GDI's actual pixel reads, not the returned sign alone.
	if info.Header.Size != 40 || info.Header.Width != 5 || (info.Header.Height != -3 && info.Header.Height != 3) || info.Header.Planes != 1 || info.Header.BitCount != 32 || info.Header.Compression != 0 {
		t.Fatalf("wrong native DIB header: %+v", info.Header)
	}
	if info.Bitmap.WidthBytes != 20 || info.Bitmap.BitsPixel != 32 || s.geometry.byteLen != 60 || info.Bitmap.Bits != s.native.bits {
		t.Fatalf("incorrect native stride or memory: %+v", info.Bitmap)
	}
	if !bytes.Equal(s.pixelBytes(), make([]byte, 60)) {
		t.Fatal("new native canvas is not fully transparent")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := flushGDI(); err != nil {
		t.Fatal(err)
	}
	pixels := s.pixelBytes()
	for y := 0; y < 3; y++ {
		for x := 0; x < 5; x++ {
			offset := y*20 + x*4
			pixels[offset+3] = 255
			if y == 0 {
				pixels[offset] = 255
			} else {
				pixels[offset+2] = 255
			}
		}
	}
	getPixel := gdi32.NewProc("GetPixel")
	top, err := invoke(getPixel, s.native.dc, 0, 0)
	if top == 0xffffffff {
		t.Fatalf("GetPixel(top): %v", err)
	}
	bottom, err := invoke(getPixel, s.native.dc, 0, 2)
	if bottom == 0xffffffff {
		t.Fatalf("GetPixel(bottom): %v", err)
	}
	if top != 0x00ff0000 || bottom != 0x000000ff {
		t.Fatalf("native rows are not top-down BGRA: top=%#x bottom=%#x", top, bottom)
	}
	if err := flushGDI(); err != nil {
		t.Fatal(err)
	}
}

func TestRealDIBResizeAndResourceCleanup(t *testing.T) {
	s, err := New(5, 3)
	if err != nil {
		if s != nil {
			_ = s.Close()
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	oldBitmap := s.native.bitmap
	if err := s.Resize(0, 2); !errors.Is(err, ErrInvalidDimensions) {
		t.Fatalf("invalid resize outcome: %v", err)
	}
	if s.native.bitmap != oldBitmap || s.geometry.width != 5 || s.geometry.height != 3 {
		t.Fatal("invalid resize changed the current canvas")
	}
	if err := s.Resize(7, 2); err != nil {
		t.Fatal(err)
	}
	info := inspectDIB(t, s.native.bitmap)
	if info.Header.Width != 7 || (info.Header.Height != -2 && info.Header.Height != 2) || info.Bitmap.WidthBytes != 28 {
		t.Fatalf("wrong resized DIB geometry: %+v", info)
	}
	if s.native.bitmap == oldBitmap {
		t.Fatal("resize did not create a replacement DIB")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if s.native.hasResources() || len(s.pending) != 0 || s.native.bits != nil {
		t.Fatal("native handles remain after Close")
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close is not idempotent: %v", err)
	}
	if err := s.Resize(1, 1); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed surface accepted resize: %v", err)
	}
	if err := s.Present(0, 0, 0, nil); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed surface accepted a frame: %v", err)
	}
}

func TestPresentRejectsInputWithoutCreatingOrShowingAWindow(t *testing.T) {
	s, err := New(2, 1)
	if err != nil {
		if s != nil {
			_ = s.Close()
		}
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := s.Present(0, 0, 0, []byte{0, 0, 0, 0}); !errors.Is(err, ErrFrameSize) {
		t.Fatalf("partial frame accepted: %v", err)
	}
	if err := s.Present(0, 0, 0, []byte{129, 0, 0, 128, 0, 0, 0, 0}); !errors.Is(err, ErrPremultiplied) {
		t.Fatalf("invalid alpha input accepted: %v", err)
	}
	if err := s.Present(0, -1920, -1080, make([]byte, 8)); !errors.Is(err, ErrInvalidWindow) {
		t.Fatalf("null HWND accepted: %v", err)
	}
	if !bytes.Equal(s.pixelBytes(), make([]byte, 8)) {
		t.Fatal("rejected Present input changed native memory")
	}
}

func TestRepeatedNativeCreateResizeCloseDoesNotAccumulateGDIObjects(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	warm, err := New(2, 2)
	if err != nil {
		if warm != nil {
			_ = warm.Close()
		}
		t.Fatal(err)
	}
	if err := warm.Close(); err != nil {
		t.Fatal(err)
	}
	getCurrentProcess := kernel32.NewProc("GetCurrentProcess")
	getGUIResources := user32.NewProc("GetGuiResources")
	process, _ := invoke(getCurrentProcess)
	before, err := invoke(getGUIResources, process, 0)
	if err != syscall.Errno(0) {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		s, err := New(8, 5)
		if err != nil {
			if s != nil {
				_ = s.Close()
			}
			t.Fatal(err)
		}
		if err := s.Resize(11, 7); err != nil {
			_ = s.Close()
			t.Fatal(err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
	after, err := invoke(getGUIResources, process, 0)
	if err != syscall.Errno(0) {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("GDI object count changed across closed surfaces: before=%d after=%d", before, after)
	}
}
