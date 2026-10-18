//go:build windows

package winlayer

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

const (
	wsExLayered = 0x00080000
	ulwAlpha    = 0x00000002
	acSrcAlpha  = 1
	acSrcOver   = 0
)

var (
	user32                   = syscall.NewLazyDLL("user32.dll")
	gdi32                    = syscall.NewLazyDLL("gdi32.dll")
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	getDC                    = user32.NewProc("GetDC")
	releaseDC                = user32.NewProc("ReleaseDC")
	isWindow                 = user32.NewProc("IsWindow")
	getWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	getWindowLongPtr         = user32.NewProc("GetWindowLongPtrW")
	getWindowLong            = user32.NewProc("GetWindowLongW")
	updateLayeredWindow      = user32.NewProc("UpdateLayeredWindow")
	createCompatibleDC       = gdi32.NewProc("CreateCompatibleDC")
	createDIBSection         = gdi32.NewProc("CreateDIBSection")
	selectObject             = gdi32.NewProc("SelectObject")
	deleteObject             = gdi32.NewProc("DeleteObject")
	deleteDC                 = gdi32.NewProc("DeleteDC")
	gdiFlush                 = gdi32.NewProc("GdiFlush")
	setLastError             = kernel32.NewProc("SetLastError")
)

type bitmapInfoHeader struct {
	Size                         uint32
	Width, Height                int32
	Planes, BitCount             uint16
	Compression, SizeImage       uint32
	XPelsPerMeter, YPelsPerMeter int32
	ClrUsed, ClrImportant        uint32
}
type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}
type point struct{ X, Y int32 }
type windowSize struct{ Width, Height int32 }
type blendFunction struct{ BlendOp, BlendFlags, SourceConstantAlpha, AlphaFormat byte }

// APIError names the failed Win32 call. Some GDI APIs do not promise a last-error
// value; in that case Cause explicitly describes their failure return value.
type APIError struct {
	Op    string
	Cause error
}

func (e *APIError) Error() string { return "winlayer " + e.Op + ": " + e.Cause.Error() }
func (e *APIError) Unwrap() error { return e.Cause }

type nativeSurface struct {
	dc, bitmap, oldBitmap uintptr
	bits                  unsafe.Pointer
}

func (n *nativeSurface) hasResources() bool { return n.dc != 0 || n.bitmap != 0 }

// Surface owns a memory DC and top-down 32-bit DIB. Use New; its zero value is
// not a drawable surface. Methods serialize native access. The caller chooses a
// stable canvas size and supplies every pixel, including transparent padding.
// A Surface must not be copied after construction.
type Surface struct {
	mu       sync.Mutex
	geometry geometry
	native   nativeSurface
	pending  []nativeSurface
	closed   bool
}

// New allocates a transparent canvas without creating or showing a window. In
// the rare case that construction cleanup fails, it returns a closed non-nil
// surface with the error so the caller can retry Close to release retained GDI
// handles. All other construction failures return a nil surface.
func New(width, height int) (*Surface, error) {
	g, err := dimensions(width, height)
	if err != nil {
		return nil, err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	native, err := allocateNative(g)
	if err != nil {
		if native.hasResources() {
			return &Surface{geometry: g, native: native, closed: true}, err
		}
		return nil, err
	}
	return &Surface{geometry: g, native: native}, nil
}

// Present uploads exactly width*height*4 bytes. BGRA must already be
// premultiplied; this adapter never multiplies the channels again. The hwnd must
// be a live layered window owned by the calling process. A successful API return
// does not by itself establish visual or animation quality.
// The caller must not modify bgra concurrently before Present returns.
func (s *Surface) Present(hwnd uintptr, x, y int, bgra []byte) (err error) {
	if s == nil {
		return ErrClosed
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.native.dc == 0 || s.native.bits == nil {
		return ErrClosed
	}
	if err := validateFrame(s.geometry, bgra); err != nil {
		return err
	}
	if err := validateCoordinates(x, y); err != nil {
		return err
	}
	if hwnd == 0 {
		return ErrInvalidWindow
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := validateWindow(hwnd); err != nil {
		return err
	}
	if err := flushGDI(); err != nil {
		return err
	}
	// Replacing the full canvas means old frame pixels do not survive. Inputs
	// remain unchanged; transparent padding and premultiplication are upstream.
	copy(s.pixelBytes(), bgra)
	screen, callErr := invoke(getDC, 0)
	if screen == 0 {
		return apiFailure("GetDC", callErr)
	}
	defer func() {
		// Flush on the same OS thread before releasing serialized GDI ownership.
		err = errors.Join(err, flushGDI(), releaseScreen(screen))
	}()
	destination := point{int32(x), int32(y)}
	size := windowSize{int32(s.geometry.width), int32(s.geometry.height)}
	source := point{}
	blend := blendFunction{acSrcOver, 0, 255, acSrcAlpha}
	ok, callErr := invoke(updateLayeredWindow, hwnd, screen, uintptr(unsafe.Pointer(&destination)), uintptr(unsafe.Pointer(&size)), s.native.dc, uintptr(unsafe.Pointer(&source)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)
	runtime.KeepAlive(destination)
	runtime.KeepAlive(size)
	runtime.KeepAlive(source)
	runtime.KeepAlive(blend)
	runtime.KeepAlive(bgra)
	if ok == 0 {
		return apiFailure("UpdateLayeredWindow", callErr)
	}
	return nil
}

// Resize replaces the canvas only when requested; Present never changes it.
// Allocation failure leaves the current canvas intact. If releasing the previous
// canvas fails after replacement, the resize is committed and the error states
// this explicitly; retained old handles are retried by Close.
func (s *Surface) Resize(width, height int) error {
	if s == nil {
		return ErrClosed
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.native.dc == 0 {
		return ErrClosed
	}
	g, err := dimensions(width, height)
	if err != nil {
		return err
	}
	if g == s.geometry {
		return nil
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	next, err := allocateNative(g)
	if err != nil {
		if next.hasResources() {
			s.pending = append(s.pending, next)
		}
		return err
	}
	previous := s.native
	s.native, s.geometry = next, g
	if err := previous.release(); err != nil {
		if previous.hasResources() {
			s.pending = append(s.pending, previous)
		}
		return fmt.Errorf("resize committed; previous canvas cleanup failed: %w", err)
	}
	return nil
}

// Close restores the DC's original bitmap, deletes owned DIBs and memory DCs,
// and is idempotent after successful cleanup. Failed handles remain owned for a
// subsequent Close attempt. Close never destroys the caller's window.
func (s *Surface) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var errs []error
	if s.native.hasResources() {
		errs = append(errs, s.native.release())
	}
	remaining := s.pending[:0]
	for i := range s.pending {
		native := s.pending[i]
		errs = append(errs, native.release())
		if native.hasResources() {
			remaining = append(remaining, native)
		}
	}
	s.pending = remaining
	return errors.Join(errs...)
}

func (s *Surface) pixelBytes() []byte {
	return unsafe.Slice((*byte)(s.native.bits), s.geometry.byteLen)
}

// invoke resolves the procedure before clearing last error. The surrounding
// public method locks the OS thread so GetDC/ReleaseDC and GDI flush pairs stay
// on that thread. Success values, rather than potentially stale errors, decide
// whether each Win32 API failed.
func invoke(proc *syscall.LazyProc, args ...uintptr) (uintptr, error) {
	if err := proc.Find(); err != nil {
		return 0, err
	}
	if err := setLastError.Find(); err != nil {
		return 0, err
	}
	setLastError.Call(0)
	r, _, err := proc.Call(args...)
	return r, err
}

func apiFailure(op string, cause error) error {
	if cause == nil || cause == syscall.Errno(0) {
		cause = errors.New("API returned failure without a Win32 error code")
	}
	return &APIError{op, cause}
}
func failedObject(handle uintptr) bool {
	return handle == 0 || handle == ^uintptr(0) || handle == 0xffffffff
}
func flushGDI() error {
	ok, err := invoke(gdiFlush)
	if ok == 0 {
		return apiFailure("GdiFlush", err)
	}
	return nil
}
func releaseScreen(screen uintptr) error {
	ok, err := invoke(releaseDC, 0, screen)
	if ok == 0 {
		return apiFailure("ReleaseDC", err)
	}
	return nil
}

func allocateNative(g geometry) (native nativeSurface, err error) {
	screen, callErr := invoke(getDC, 0)
	if screen == 0 {
		return native, apiFailure("GetDC", callErr)
	}
	defer func() {
		err = errors.Join(err, releaseScreen(screen))
		if err != nil {
			err = errors.Join(err, native.release())
		}
	}()
	native.dc, callErr = invoke(createCompatibleDC, screen)
	if native.dc == 0 {
		return native, apiFailure("CreateCompatibleDC", callErr)
	}
	info := bitmapInfo{Header: bitmapInfoHeader{Size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), Width: int32(g.width), Height: -int32(g.height), Planes: 1, BitCount: 32, Compression: 0, SizeImage: uint32(g.byteLen)}}
	var bits unsafe.Pointer
	native.bitmap, callErr = invoke(createDIBSection, screen, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	runtime.KeepAlive(info)
	runtime.KeepAlive(bits)
	if native.bitmap == 0 || bits == nil {
		return native, apiFailure("CreateDIBSection", callErr)
	}
	native.bits = bits
	native.oldBitmap, callErr = invoke(selectObject, native.dc, native.bitmap)
	if failedObject(native.oldBitmap) {
		native.oldBitmap = 0
		return native, apiFailure("SelectObject", callErr)
	}
	if err = flushGDI(); err != nil {
		return native, err
	}
	clear(unsafe.Slice((*byte)(native.bits), g.byteLen))
	return native, nil
}

func (native *nativeSurface) release() error {
	if !native.hasResources() {
		return nil
	}
	if err := flushGDI(); err != nil {
		return err
	}
	var errs []error
	if native.dc != 0 && native.oldBitmap != 0 {
		replaced, err := invoke(selectObject, native.dc, native.oldBitmap)
		if failedObject(replaced) {
			errs = append(errs, apiFailure("SelectObject(restore)", err))
		} else {
			native.oldBitmap = 0
		}
	}
	deleteBitmap := func() {
		ok, err := invoke(deleteObject, native.bitmap)
		if ok == 0 {
			errs = append(errs, apiFailure("DeleteObject(DIB)", err))
		} else {
			native.bitmap = 0
			native.bits = nil
		}
	}
	if native.bitmap != 0 && native.oldBitmap == 0 {
		deleteBitmap()
	}
	if native.dc != 0 {
		ok, err := invoke(deleteDC, native.dc)
		if ok == 0 {
			errs = append(errs, apiFailure("DeleteDC", err))
		} else {
			native.dc = 0
			native.oldBitmap = 0
		}
	}
	// If restoring selection failed, deleting the DC first removes its reference
	// to the owned bitmap. Retry deletion only after the DC no longer exists.
	if native.bitmap != 0 && native.dc == 0 {
		deleteBitmap()
	}
	errs = append(errs, flushGDI())
	return errors.Join(errs...)
}

func validateWindow(hwnd uintptr) error {
	valid, _ := invoke(isWindow, hwnd)
	if valid == 0 {
		return ErrInvalidWindow
	}
	var pid uint32
	threadID, callErr := invoke(getWindowThreadProcessID, hwnd, uintptr(unsafe.Pointer(&pid)))
	runtime.KeepAlive(pid)
	if threadID == 0 {
		return apiFailure("GetWindowThreadProcessId", callErr)
	}
	if pid != uint32(os.Getpid()) {
		return ErrInvalidWindow
	}
	index := int32(-20) // GWL_EXSTYLE
	proc := getWindowLongPtr
	if unsafe.Sizeof(uintptr(0)) == 4 {
		proc = getWindowLong
	}
	style, callErr := invoke(proc, hwnd, uintptr(index))
	if style == 0 && callErr != syscall.Errno(0) {
		return apiFailure("GetWindowLong(EXSTYLE)", callErr)
	}
	if style&wsExLayered == 0 {
		return ErrWindowNotLayered
	}
	return nil
}
