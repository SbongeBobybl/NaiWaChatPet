package main

import (
	"fmt"
	"image"
	"math"
	"sort"

	"codexpet/reconstruction/compositor"
)

// renderPreflight validates all clip types through the same RGB/alpha processing
// chain as the Windows layered-window renderer, without creating a GUI window.
// It intentionally uses sampled frames so this check remains quick at startup.
func renderPreflight(a *loadedAssets) (int, error) {
	checked := 0
	names := make([]string, 0, len(a.Counts))
	for name := range a.Counts {
		names = append(names, name)
	}
	sort.Strings(names)
	factor := a.Config.Scale.Default
	sizes := [][2]int{
		{int(math.Round(float64(a.Canvas.Dx()) * factor)), int(math.Round(float64(a.Canvas.Dy()) * factor))},
		{int(math.Round(float64(a.Canvas.Dx()) * factor * 1.25)), int(math.Round(float64(a.Canvas.Dy()) * factor * 1.25))},
	}
	for _, name := range names {
		n := a.Counts[name]
		selected := map[int]bool{0: true, n / 2: true, n - 1: true}
		for idx := range selected {
			body, err := a.Bodies.BodyCopy(name, idx)
			if err != nil {
				return checked, err
			}
			scene := compositor.Compose(a.Canvas, compositor.Layer{Image: body, At: image.Point{}}, nil, nil)
			for _, s := range sizes {
				scaled, err := compositor.Resize(scene, s[0], s[1])
				if err != nil {
					return checked, err
				}
				bgra := compositor.BGRA(scaled)
				for j := 0; j < len(bgra); j += 4 {
					alpha := bgra[j+3]
					if bgra[j] > alpha || bgra[j+1] > alpha || bgra[j+2] > alpha {
						return checked, fmt.Errorf("%s frame %03d size %dx%d: premultiplied BGRA violation at pixel %d (B,G,R,A=%v)", name, idx, s[0], s[1], j/4, bgra[j:j+4])
					}
				}
				checked++
			}
		}
	}
	return checked, nil
}
