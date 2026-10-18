//go:build !windows

package winlayer

import (
	"errors"
	"testing"
)

func TestNonWindowsFailsClosed(t *testing.T) {
	if s, err := New(1, 1); s != nil || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported New: %v, %v", s, err)
	}
	var surface Surface
	if !errors.Is(surface.Present(0, 0, 0, nil), ErrUnsupported) || !errors.Is(surface.Resize(1, 1), ErrUnsupported) || !errors.Is(surface.Close(), ErrUnsupported) {
		t.Fatal("non-Windows methods did not report unsupported")
	}
}
