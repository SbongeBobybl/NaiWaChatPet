package rig

import "image"

// BlendToExistingPose blends rendered poses without changing either skeleton
// or any authored keyframe. Inputs are immutable and share canvas dimensions.
func BlendToExistingPose(from, to *image.NRGBA, weight float64) *image.NRGBA {
	if from == nil || to == nil || from.Rect != to.Rect {
		return to
	}
	mix := uint32(clamp01(weight)*255 + .5)
	keep := 255 - mix
	out := image.NewNRGBA(to.Rect)
	for i := 0; i < len(out.Pix); i += 4 {
		a0, a1 := uint32(from.Pix[i+3]), uint32(to.Pix[i+3])
		alpha := a0*keep + a1*mix
		if alpha == 0 {
			continue
		}
		out.Pix[i+3] = uint8((alpha + 127) / 255)
		for ch := 0; ch < 3; ch++ {
			value := uint32(from.Pix[i+ch])*a0*keep + uint32(to.Pix[i+ch])*a1*mix
			out.Pix[i+ch] = uint8((value + alpha/2) / alpha)
		}
	}
	return out
}
