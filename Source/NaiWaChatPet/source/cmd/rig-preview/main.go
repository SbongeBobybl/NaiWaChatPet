// Command rig-preview renders contact sheets of the skinned rigs so a change to
// a joint, mask or motion curve can be judged by eye before it ships.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"

	"codexpet/reconstruction/rig"
)

func load(dir, name string) (*image.NRGBA, error) {
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	im, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	n, ok := im.(*image.NRGBA)
	if !ok {
		return nil, fmt.Errorf("%s: expected straight RGBA", name)
	}
	return n, nil
}

type row struct {
	name  string
	cells []*image.NRGBA
}

func main() {
	body := flag.String("body", "cmd/codexpet/embedded_assets/body", "directory of body frames")
	out := flag.String("out", "rig-preview.png", "contact sheet to write")
	zoom := flag.Int("shrink", 2, "divide each cell by this factor")
	flag.Parse()
	if err := run(*body, *out, max(1, *zoom)); err != nil {
		fmt.Fprintln(os.Stderr, "rig-preview:", err)
		os.Exit(1)
	}
}

func run(body, out string, shrink int) error {
	idle, err := load(body, "idle_000.png")
	if err != nil {
		return err
	}
	fight, err := load(body, "fight_012.png")
	if err != nil {
		return err
	}
	stand, err := rig.Stand()
	if err != nil {
		return err
	}
	stance, err := rig.Stance()
	if err != nil {
		return err
	}
	figure, err := rig.Figure()
	if err != nil {
		return err
	}
	idleTex, fightTex := rig.NewTexture(idle), rig.NewTexture(fight)

	var rows []row
	add := func(name string, puppet *rig.Puppet, tex *rig.Texture, poses ...rig.Pose) error {
		r := row{name: name}
		for _, pose := range poses {
			frame, err := puppet.Render(tex, pose)
			if err != nil {
				return err
			}
			r.cells = append(r.cells, frame)
		}
		rows = append(rows, r)
		return nil
	}
	var walk, sprint []rig.Pose
	for i := 0; i < 8; i++ {
		walk = append(walk, rig.StandPose(rig.Gait{Stride: 1, Phase: float64(i) / 8}))
		sprint = append(sprint, rig.StandPose(rig.Gait{Stride: 2, Phase: float64(i) / 8}))
	}
	jump := []rig.Pose{
		rig.StandPose(rig.Gait{}),
		rig.StandPose(rig.Gait{Squash: .12}),
		rig.StandPose(rig.Gait{Squash: -.07, Reach: .6}),
		rig.StandPose(rig.Gait{Squash: -.03, Tuck: .6}),
		rig.StandPose(rig.Gait{Tuck: 1}),
		rig.StandPose(rig.Gait{Squash: -.05, Reach: 1}),
		rig.StandPose(rig.Gait{Squash: .14}),
		rig.StandPose(rig.Gait{Squash: -.03}),
	}
	var strike []rig.Pose
	for _, t := range []float64{0, .14, .28, .34, .40, .50, .70, 1} {
		strike = append(strike, rig.StrikePose(t, 1))
	}
	var gestures []rig.Pose
	for _, g := range []string{"nod", "shake", "sway", "bounce", "sigh", "bow", "puff", "tilt"} {
		gestures = append(gestures, rig.GesturePose(g, .45))
	}
	for _, e := range []error{
		add("walk", stand, idleTex, walk...),
		add("run", stand, idleTex, sprint...),
		add("jump", stand, idleTex, jump...),
		add("strike", stance, fightTex, strike...),
		add("gesture", figure, idleTex, gestures...),
	} {
		if e != nil {
			return e
		}
	}

	// Rest poses must reproduce their source frames; report the worst pixel.
	for _, check := range []struct {
		name   string
		puppet *rig.Puppet
		tex    *rig.Texture
		source *image.NRGBA
	}{{"stand", stand, idleTex, idle}, {"stance", stance, fightTex, fight}, {"figure", figure, idleTex, idle}} {
		frame, err := check.puppet.Render(check.tex, check.puppet.Rest())
		if err != nil {
			return err
		}
		// Compare what reaches the screen: colour weighted by its own alpha.
		worst, count := 0, 0
		for i := 0; i < len(frame.Pix); i += 4 {
			for k := 0; k < 4; k++ {
				got, want := int(frame.Pix[i+k]), int(check.source.Pix[i+k])
				if k < 3 {
					got, want = got*int(frame.Pix[i+3])/255, want*int(check.source.Pix[i+3])/255
				}
				d := got - want
				if d < 0 {
					d = -d
				}
				if d > 2 {
					count++
				}
				worst = max(worst, d)
			}
		}
		fmt.Printf("%-7s rest pose: worst visible channel error %d, channels off by more than 2: %d\n", check.name, worst, count)
	}

	cw, ch := rig.CanvasWidth/shrink, rig.CanvasHeight/shrink
	sheet := image.NewRGBA(image.Rect(0, 0, cw*8, ch*len(rows)))
	draw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.RGBA{70, 90, 120, 255}), image.Point{}, draw.Src)
	for j, r := range rows {
		for i, cell := range r.cells {
			for y := 0; y < ch; y++ {
				for x := 0; x < cw; x++ {
					// Box-filter each cell down in premultiplied space.
					var sum [4]int
					for dy := 0; dy < shrink; dy++ {
						for dx := 0; dx < shrink; dx++ {
							c := cell.NRGBAAt(x*shrink+dx, y*shrink+dy)
							sum[0] += int(c.R) * int(c.A)
							sum[1] += int(c.G) * int(c.A)
							sum[2] += int(c.B) * int(c.A)
							sum[3] += int(c.A)
						}
					}
					n := shrink * shrink * 255
					under := sheet.RGBAAt(i*cw+x, j*ch+y)
					keep := n - sum[3]
					sheet.SetRGBA(i*cw+x, j*ch+y, color.RGBA{
						uint8((sum[0] + int(under.R)*keep) / n),
						uint8((sum[1] + int(under.G)*keep) / n),
						uint8((sum[2] + int(under.B)*keep) / n), 255})
				}
			}
		}
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	if err := png.Encode(f, sheet); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
