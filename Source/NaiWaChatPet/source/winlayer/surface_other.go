//go:build !windows

package winlayer

type Surface struct{}

func New(width, height int) (*Surface, error)                      { return nil, ErrUnsupported }
func (*Surface) Present(hwnd uintptr, x, y int, bgra []byte) error { return ErrUnsupported }
func (*Surface) Resize(width, height int) error                    { return ErrUnsupported }
func (*Surface) Close() error                                      { return ErrUnsupported }
