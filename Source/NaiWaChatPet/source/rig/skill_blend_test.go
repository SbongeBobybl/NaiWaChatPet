package rig

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

func TestSkillBlendKeepsInputsAndEndpoints(t *testing.T) {
	a, b := image.NewNRGBA(image.Rect(0, 0, 4, 4)), image.NewNRGBA(image.Rect(0, 0, 4, 4))
	a.SetNRGBA(1, 1, color.NRGBA{255, 0, 0, 255})
	b.SetNRGBA(1, 1, color.NRGBA{0, 255, 0, 255})
	original := append([]byte(nil), a.Pix...)
	if !bytes.Equal(BlendToExistingPose(a, b, 0).Pix, a.Pix) || !bytes.Equal(BlendToExistingPose(a, b, 1).Pix, b.Pix) {
		t.Fatal("blend endpoints")
	}
	mid := BlendToExistingPose(a, b, .5).NRGBAAt(1, 1)
	if mid.R < 120 || mid.G < 120 || mid.A != 255 {
		t.Fatal("blend midpoint")
	}
	if !bytes.Equal(a.Pix, original) {
		t.Fatal("blend modified source")
	}
}
