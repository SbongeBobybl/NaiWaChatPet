package compositor

import (
	"image"
	"image/color"
	"testing"
)

func TestResizeIdentitySubimageKeepsPremultiplicationAndOwnership(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 6, 6))
	src.SetRGBA(3, 3, color.RGBA{R: 90, G: 50, B: 20, A: 100})
	sub := src.SubImage(image.Rect(2, 2, 5, 5)).(*image.RGBA)
	out, e := Resize(sub, 3, 3)
	if e != nil || out.RGBAAt(1, 1) != src.RGBAAt(3, 3) {
		t.Fatal("identity resize changed pixels")
	}
	out.SetRGBA(1, 1, color.RGBA{})
	if src.RGBAAt(3, 3).A != 100 {
		t.Fatal("identity resize aliases source")
	}
	src.SetRGBA(3, 3, color.RGBA{R: 250, A: 50})
	out, e = Resize(sub, 3, 3)
	if e != nil || out.RGBAAt(1, 1).R > out.RGBAAt(1, 1).A {
		t.Fatal("invalid premultiplication")
	}
}
