package main

import (
	"fmt"
	"image"
	"testing"

	"codexpet/reconstruction/compositor"
)

func checkBGRAAlpha(t *testing.T, label string, data []byte) {
	t.Helper()
	for i := 0; i < len(data); i += 4 {
		a := data[i+3]
		if data[i] > a || data[i+1] > a || data[i+2] > a {
			t.Fatalf("%s: invalid premultiplied pixel=%d BGRA=%v", label, i/4, data[i:i+4])
		}
	}
}

func TestEveryReleasedBodyFramePremulAtDefaultScale(t *testing.T) {
	loaded, err := loadAssets(bundledAssets, "runtime.json")
	if err != nil {
		t.Fatal(err)
	}
	tested := 0
	for state, count := range loaded.Counts {
		for i := 0; i < count; i++ {
			body, err := loaded.Bodies.BodyCopy(state, i)
			if err != nil {
				t.Fatal(err)
			}
			scene := compositor.Compose(loaded.Canvas, compositor.Layer{Image: body}, nil, nil)
			resized, err := compositor.Resize(scene, 153, 204)
			if err != nil {
				t.Fatal(err)
			}
			label := fmt.Sprintf("%s_%03d", state, i)
			checkBGRAAlpha(t, label, compositor.BGRA(resized))
			tested++
		}
	}
	t.Logf("checked %d authentic PNG frames at 153 x 204", tested)
}

func TestHighDPIAndOverlaidPropPremul(t *testing.T) {
	loaded, err := loadAssets(bundledAssets, "runtime.json")
	if err != nil {
		t.Fatal(err)
	}
	// 96, 120, 144, 192 DPI, plus some user-configurable larger sizes.
	sizes := [][2]int{{153, 204}, {192, 256}, {230, 307}, {307, 409}, {273, 364}}
	for _, state := range []string{"idle", "toLaugh", "video", "fromLaugh", "poop", "eat"} {
		count := loaded.Counts[state]
		for _, i := range []int{0, count / 2, count - 1} {
			body, err := loaded.Bodies.BodyCopy(state, i)
			if err != nil {
				t.Fatal(err)
			}
			prop := &compositor.Layer{Image: loaded.Prop, At: image.Pt(200, 380)}
			scene := compositor.Compose(loaded.Canvas, compositor.Layer{Image: body}, prop, nil)
			for _, wh := range sizes {
				resized, err := compositor.Resize(scene, wh[0], wh[1])
				if err != nil {
					t.Fatal(err)
				}
				checkBGRAAlpha(t, fmt.Sprintf("%s_%03d_%dx%d", state, i, wh[0], wh[1]), compositor.BGRA(resized))
			}
		}
	}
}
