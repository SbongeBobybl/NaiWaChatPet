package winlayer

import (
	"errors"
	"math"
	"strconv"
	"testing"
)

func TestDimensionsAndStride(t *testing.T) {
	g, err := dimensions(13, 7)
	if err != nil {
		t.Fatal(err)
	}
	if g.stride != 52 || g.byteLen != 364 {
		t.Fatalf("incorrect 32-bit DIB geometry: %+v", g)
	}
	for _, size := range [][2]int{{0, 1}, {1, 0}, {-1, 1}, {1, -1}, {MaxSurfaceBytes, 1}, {math.MaxInt32, math.MaxInt32}} {
		if _, err := dimensions(size[0], size[1]); !errors.Is(err, ErrInvalidDimensions) {
			t.Fatalf("accepted invalid size %v: %v", size, err)
		}
	}
}

func TestValidatesCompletePremultipliedCanvas(t *testing.T) {
	g, err := dimensions(2, 1)
	if err != nil {
		t.Fatal(err)
	}
	valid := []byte{10, 20, 40, 128, 0, 0, 0, 0}
	if err := validateFrame(g, valid); err != nil {
		t.Fatal(err)
	}
	if err := validateFrame(g, valid[:4]); !errors.Is(err, ErrFrameSize) {
		t.Fatalf("accepted partial canvas: %v", err)
	}
	if err := validateFrame(g, append(append([]byte{}, valid...), 0)); !errors.Is(err, ErrFrameSize) {
		t.Fatalf("accepted oversized canvas: %v", err)
	}
	for _, invalid := range [][]byte{{129, 0, 0, 128, 0, 0, 0, 0}, {0, 129, 0, 128, 0, 0, 0, 0}, {0, 0, 129, 128, 0, 0, 0, 0}, {0, 0, 0, 0, 0, 0, 1, 0}} {
		if err := validateFrame(g, invalid); !errors.Is(err, ErrPremultiplied) {
			t.Fatalf("accepted invalid alpha bounds: %v", err)
		}
	}
}

func TestCoordinatesAllowNegativeMonitorsAndRejectOverflow(t *testing.T) {
	if err := validateCoordinates(-1920, -1080); err != nil {
		t.Fatal(err)
	}
	if strconv.IntSize == 64 {
		tooLarge := int64(math.MaxInt32) + 1
		tooSmall := int64(math.MinInt32) - 1
		if !errors.Is(validateCoordinates(int(tooLarge), 0), ErrCoordinates) || !errors.Is(validateCoordinates(0, int(tooSmall)), ErrCoordinates) {
			t.Fatal("accepted overflowing coordinates")
		}
	}
}
