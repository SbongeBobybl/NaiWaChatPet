package rig

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

func TestRenderSizedPreservesRigAndFullResolution(t *testing.T) {
	p, err := Stand()
	if err != nil {
		t.Fatal(err)
	}
	im := image.NewNRGBA(p.Bounds())
	for y := 0; y < 560; y++ {
		for x := 0; x < 420; x++ {
			im.SetNRGBA(x, y, color.NRGBA{uint8(x % 255), uint8(y % 255), 100, 255})
		}
	}
	tex := NewTexture(im)
	before, err := p.Render(tex, p.Rest())
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range [][2]int{{210, 280}, {168, 224}, {630, 840}} {
		out, err := p.RenderSized(tex, StandPose(Gait{Stride: 1, Phase: .2}), size[0], size[1])
		if err != nil || out.Bounds().Dx() != size[0] {
			t.Fatalf("sized render %v: %v", size, err)
		}
	}
	after, err := p.Render(tex, p.Rest())
	if err != nil || !bytes.Equal(before.Pix, after.Pix) {
		t.Fatal("sized render changed original rig")
	}
}
