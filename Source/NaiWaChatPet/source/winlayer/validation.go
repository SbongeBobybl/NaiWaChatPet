// Package winlayer adapts an already-composited premultiplied BGRA canvas to
// UpdateLayeredWindow. It does not create a window or run an application loop.
package winlayer

import (
	"errors"
	"fmt"
	"math"
)

var (
	ErrUnsupported       = errors.New("Win32 layered surfaces are supported only on Windows")
	ErrClosed            = errors.New("surface is closed or uninitialized")
	ErrInvalidDimensions = errors.New("invalid canvas dimensions")
	ErrFrameSize         = errors.New("frame length does not match the full canvas")
	ErrPremultiplied     = errors.New("BGRA input violates premultiplied alpha bounds")
	ErrCoordinates       = errors.New("screen coordinates exceed Win32 LONG range")
	ErrInvalidWindow     = errors.New("window must be a valid window owned by this process")
	ErrWindowNotLayered  = errors.New("window must already have WS_EX_LAYERED")
)

// MaxSurfaceBytes bounds native allocations, including while resizing. A resize
// can temporarily retain both the old and replacement canvas.
const MaxSurfaceBytes = 256 << 20

type geometry struct{ width, height, stride, byteLen int }

func dimensions(width, height int) (geometry, error) {
	if width <= 0 || height <= 0 || int64(width) > math.MaxInt32 || int64(height) > math.MaxInt32 {
		return geometry{}, ErrInvalidDimensions
	}
	// Divide before multiplying to avoid overflowing even on 32-bit builds.
	if int64(width) > int64(MaxSurfaceBytes)/4/int64(height) {
		return geometry{}, fmt.Errorf("%w: canvas exceeds %d bytes", ErrInvalidDimensions, MaxSurfaceBytes)
	}
	return geometry{width, height, width * 4, width * height * 4}, nil
}

func validateFrame(g geometry, bgra []byte) error {
	if len(bgra) != g.byteLen {
		return fmt.Errorf("%w: got %d bytes, want %d", ErrFrameSize, len(bgra), g.byteLen)
	}
	for i := 0; i < len(bgra); i += 4 {
		a := bgra[i+3]
		if bgra[i] > a || bgra[i+1] > a || bgra[i+2] > a {
			return fmt.Errorf("%w at pixel %d", ErrPremultiplied, i/4)
		}
	}
	return nil
}

func validateCoordinates(x, y int) error {
	if int64(x) < math.MinInt32 || int64(x) > math.MaxInt32 || int64(y) < math.MinInt32 || int64(y) > math.MaxInt32 {
		return ErrCoordinates
	}
	return nil
}
