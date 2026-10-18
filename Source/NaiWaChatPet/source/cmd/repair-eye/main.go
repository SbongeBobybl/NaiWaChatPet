// repair-eye restores source-video eye pixels removed by the original matte.
// It changes alpha protection only, keeps body pixels outside that ROI intact,
// and regenerates every surprised bridge's hash. No generative redraw is used.
package main

import (
	"codexpet/reconstruction/compositor"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
)

type geometry struct {
	Box [4]int `json:"box"`
}
type component struct {
	pixels []image.Point
	box    image.Rectangle
}

func load(p string) *image.NRGBA {
	f, e := os.Open(p)
	if e != nil {
		panic(e)
	}
	defer f.Close()
	im, e := png.Decode(f)
	if e != nil {
		panic(e)
	}
	out := image.NewNRGBA(im.Bounds())
	draw.Draw(out, out.Bounds(), im, im.Bounds().Min, draw.Src)
	return out
}
func bounds(im *image.NRGBA) image.Rectangle {
	b := im.Bounds()
	r := image.Rectangle{Min: b.Max}
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			if im.NRGBAAt(x, y).A > 0 {
				r.Min.X = min(r.Min.X, x)
				r.Min.Y = min(r.Min.Y, y)
				r.Max.X = max(r.Max.X, x+1)
				r.Max.Y = max(r.Max.Y, y+1)
			}
		}
	}
	return r
}
func components(im *image.NRGBA, roi image.Rectangle, predicate func(color.NRGBA) bool) []component {
	roi = roi.Intersect(im.Bounds())
	seen := map[image.Point]bool{}
	var found []component
	for y := roi.Min.Y; y < roi.Max.Y; y++ {
		for x := roi.Min.X; x < roi.Max.X; x++ {
			start := image.Pt(x, y)
			if seen[start] || !predicate(im.NRGBAAt(x, y)) {
				continue
			}
			q := []image.Point{start}
			seen[start] = true
			r := image.Rect(x, y, x+1, y+1)
			for at := 0; at < len(q); at++ {
				p := q[at]
				r = r.Union(image.Rect(p.X, p.Y, p.X+1, p.Y+1))
				for _, d := range []image.Point{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
					n := p.Add(d)
					if n.In(roi) && !seen[n] && predicate(im.NRGBAAt(n.X, n.Y)) {
						seen[n] = true
						q = append(q, n)
					}
				}
			}
			found = append(found, component{q, r})
		}
	}
	return found
}
func findLeftEye(im *image.NRGBA, g geometry) (image.Rectangle, bool) {
	b := image.Rect(g.Box[0], g.Box[1], g.Box[2], g.Box[3])
	roi := image.Rect(b.Min.X+b.Dx()/6, b.Min.Y+5, b.Max.X-b.Dx()/6, b.Min.Y+125)
	cs := components(im, roi, func(c color.NRGBA) bool { return c.R < 85 && c.G < 85 && c.B < 85 })
	var pupils []component
	for _, c := range cs {
		if len(c.pixels) >= 15 && len(c.pixels) <= 900 && c.box.Dx() >= 5 && c.box.Dx() <= 42 && c.box.Dy() >= 5 && c.box.Dy() <= 52 {
			pupils = append(pupils, c)
		}
	}
	if len(pupils) == 0 {
		return image.Rectangle{}, false
	}
	sort.Slice(pupils, func(i, j int) bool { return pupils[i].box.Min.X < pupils[j].box.Min.X })
	p := pupils[0].box
	cx, cy := (p.Min.X+p.Max.X)/2, (p.Min.Y+p.Max.Y)/2
	outline := components(im, image.Rect(cx-38, cy-46, cx+39, cy+47), eyePixel)
	eye := p
	for _, v := range outline {
		mx, my := (v.box.Min.X+v.box.Max.X)/2, (v.box.Min.Y+v.box.Max.Y)/2
		if abs(mx-cx) < 24 && abs(my-cy) < 29 && v.box.Dx() <= 65 && v.box.Dy() <= 80 && len(v.pixels) >= 5 {
			eye = eye.Union(v.box)
		}
	}
	if eye.Dx() < 18 {
		eye = eye.Inset(-5)
	}
	if eye.Dx() > 65 || eye.Dy() > 80 {
		return image.Rectangle{}, false
	}
	return eye.Inset(-1), true
}
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
func eyePixel(c color.NRGBA) bool {
	return c.R < 155 && c.G < 150 && c.B < 145 || max(c.R, c.G, c.B)-min(c.R, c.G, c.B) < 40 && max(c.R, c.G, c.B) < 212
}

func main() {
	if len(os.Args) != 4 {
		panic("repair-eye old-v13-root new-root source-dir")
	}
	old, newRoot, source := os.Args[1], os.Args[2], os.Args[3]
	var geo []geometry
	b, e := os.ReadFile(filepath.Join(old, "audit", "matte", "surprised", "geometry.json"))
	if e != nil {
		panic(e)
	}
	json.Unmarshal(b, &geo)
	ref := load(filepath.Join(source, "cmd", "codexpet", "embedded_assets", "body", "idle_000.png"))
	rb := bounds(ref)
	scale := float64(rb.Dy()) / float64(geo[0].Box[3]-geo[0].Box[1])
	center := float64(rb.Min.X+rb.Max.X) / 2
	type record struct {
		Source          int
		Eye             image.Rectangle
		RecoveredPixels int
	}
	var report []record
	eyePatches := map[int]*image.NRGBA{}
	for i, g := range geo {
		raw := load(filepath.Join(old, "audit", "raw", "surprised", fmt.Sprintf("frame_%04d.png", i+1)))
		eye, ok := findLeftEye(raw, g)
		if !ok {
			report = append(report, record{Source: i})
			continue
		}
		patch := image.NewNRGBA(raw.Bounds())
		// Preserve the source contour rather than imposing an axis-aligned ellipse.
		mask := map[image.Point]bool{}
		outside := map[image.Point]bool{}
		var q []image.Point
		for y := eye.Min.Y; y < eye.Max.Y; y++ {
			for x := eye.Min.X; x < eye.Max.X; x++ {
				p := image.Pt(x, y)
				mask[p] = eyePixel(raw.NRGBAAt(x, y))
				if !mask[p] && (x == eye.Min.X || x == eye.Max.X-1 || y == eye.Min.Y || y == eye.Max.Y-1) {
					outside[p] = true
					q = append(q, p)
				}
			}
		}
		for at := 0; at < len(q); at++ {
			for _, delta := range []image.Point{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				n := q[at].Add(delta)
				if n.In(eye) && !mask[n] && !outside[n] {
					outside[n] = true
					q = append(q, n)
				}
			}
		}
		for y := eye.Min.Y; y < eye.Max.Y; y++ {
			for x := eye.Min.X; x < eye.Max.X; x++ {
				if mask[image.Pt(x, y)] || !outside[image.Pt(x, y)] {
					patch.SetNRGBA(x, y, raw.NRGBAAt(x, y))
				}
			}
		}
		box := image.Rect(g.Box[0], g.Box[1], g.Box[2], g.Box[3])
		w, h := int(math.Round(float64(box.Dx())*scale)), int(math.Round(float64(box.Dy())*scale))
		x := int(math.Round(center - float64(w)/2))
		x = max(6, min(x, 414-w))
		y := 540 - h
		resized, err := compositor.Resize(patch.SubImage(box), w, h)
		if err != nil {
			panic(err)
		}
		out := image.NewNRGBA(image.Rect(0, 0, 420, 560))
		draw.Draw(out, image.Rect(x, y, x+w, y+h), resized, image.Point{}, draw.Src)
		eyePatches[i] = out
		report = append(report, record{Source: i, Eye: eye})
	}
	definitions := map[string][]int{}
	for i := 55; i < 150; i++ {
		definitions["surprised"] = append(definitions["surprised"], i)
	}
	for i := 0; i < 55; i += 4 {
		definitions["surprised_in"] = append(definitions["surprised_in"], i)
	}
	for i := 149; i >= 0; i -= 2 {
		definitions["surprised_out"] = append(definitions["surprised_out"], i)
	}
	changed, total := 0, 0
	for name, indices := range definitions {
		dir := filepath.Join(newRoot, "animations", name)
		for j, i := range indices {
			path := filepath.Join(dir, fmt.Sprintf("%04d.png", j))
			im := load(filepath.Join(old, "animations", name, fmt.Sprintf("%04d.png", j)))
			patch := eyePatches[i]
			if patch != nil {
				recovered := 0
				for p := 3; p < len(im.Pix); p += 4 {
					if im.Pix[p] < 100 && patch.Pix[p] > 220 {
						recovered++
					}
				}
				draw.Draw(im, im.Bounds(), patch, image.Point{}, draw.Over)
				report[i].RecoveredPixels += recovered
				changed++
			}
			// Preserve PNG's straight-alpha storage invariant.
			for p := 0; p < len(im.Pix); p += 4 {
				if im.Pix[p+3] == 0 {
					im.Pix[p], im.Pix[p+1], im.Pix[p+2] = 0, 0, 0
				}
			}
			f, err := os.Create(path)
			if err != nil {
				panic(err)
			}
			if err = png.Encode(f, im); err != nil {
				panic(err)
			}
			f.Close()
			total++
		}
		manifestPath := filepath.Join(dir, "manifest.json")
		data, _ := os.ReadFile(manifestPath)
		var m map[string]any
		json.Unmarshal(data, &m)
		frames := m["frames"].([]any)
		for _, item := range frames {
			v := item.(map[string]any)
			data, _ := os.ReadFile(filepath.Join(dir, v["file"].(string)))
			sum := sha256.Sum256(data)
			v["sha256"] = hex.EncodeToString(sum[:])
		}
		m["issues"] = []string{}
		delete(m, "repair") // Keep repair notes in audit, outside the strict runtime schema.
		data, _ = json.MarshalIndent(m, "", "  ")
		os.WriteFile(manifestPath, data, 0600)
	}
	os.MkdirAll(filepath.Join(newRoot, "audit"), 0700)
	data, _ := json.MarshalIndent(map[string]any{"runtime_frames": total, "frames_with_eye_patch": changed, "source_frames": report}, "", "  ")
	os.WriteFile(filepath.Join(newRoot, "audit", "eye_repair.json"), data, 0600)
	fmt.Printf("Restored source eye pixels in %d/%d runtime frames\n", changed, total)
}
