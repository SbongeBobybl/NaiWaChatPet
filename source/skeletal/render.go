package skeletal

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Atlas is a single-page Spine atlas with unrotated, untrimmed regions.
type Atlas struct {
	Page    *image.NRGBA
	Regions map[string]image.Rectangle
}

// LoadAtlas reads a .atlas file and its page image from the same directory.
func LoadAtlas(path string) (*Atlas, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pageName, regions, err := parseAtlas(raw)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(filepath.Dir(path), pageName))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	im, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", pageName, err)
	}
	page, ok := im.(*image.NRGBA)
	if !ok {
		page = image.NewNRGBA(im.Bounds())
		draw.Draw(page, page.Bounds(), im, im.Bounds().Min, draw.Src)
	}
	for name, r := range regions {
		if !r.In(page.Bounds()) {
			return nil, fmt.Errorf("region %s %v is outside the page", name, r)
		}
	}
	return &Atlas{Page: page, Regions: regions}, nil
}

func parseAtlas(raw []byte) (string, map[string]image.Rectangle, error) {
	sc := bufio.NewScanner(bytes.NewReader(raw))
	page := ""
	regions := map[string]image.Rectangle{}
	current := ""
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		key, val, isProp := strings.Cut(line, ":")
		if !isProp {
			if page == "" {
				page = line
			} else if strings.HasSuffix(line, ".png") {
				return "", nil, fmt.Errorf("multi-page atlases are not supported")
			} else {
				current = line
			}
			continue
		}
		val = strings.TrimSpace(val)
		switch strings.TrimSpace(key) {
		case "bounds":
			if current == "" {
				return "", nil, fmt.Errorf("bounds before a region name")
			}
			var n [4]int
			parts := strings.Split(val, ",")
			if len(parts) != 4 {
				return "", nil, fmt.Errorf("region %s: bad bounds %q", current, val)
			}
			for i, p := range parts {
				v, err := strconv.Atoi(strings.TrimSpace(p))
				if err != nil {
					return "", nil, fmt.Errorf("region %s: bad bounds %q", current, val)
				}
				n[i] = v
			}
			regions[current] = image.Rect(n[0], n[1], n[0]+n[2], n[1]+n[3])
		case "rotate", "offsets":
			if current != "" && val != "false" && val != "0" {
				return "", nil, fmt.Errorf("region %s: %s is not supported", current, key)
			}
		}
	}
	if page == "" {
		return "", nil, fmt.Errorf("empty atlas")
	}
	return page, regions, sc.Err()
}

// Render draws a pose onto a transparent canvas of the skeleton's size with
// bilinear sampling and premultiplied source-over, matching the preview
// renderer in tools/skeletal. Quads that land on whole pixels unscaled are
// copied exactly, so the setup pose reproduces the reference plate.
func (s *Skeleton) Render(p *Pose, atlas *Atlas) (*image.NRGBA, error) {
	w, h := int(s.Width), int(s.Height)
	acc := make([]float64, w*h*4)
	for _, sp := range p.Slots {
		if sp.Point || sp.Color[3] <= 0.0005 {
			continue
		}
		r, ok := atlas.Regions[sp.Region]
		if !ok {
			return nil, fmt.Errorf("atlas has no region %q", sp.Region)
		}
		rw, rh := float64(r.Dx()), float64(r.Dy())
		blx, bly := s.ToCanvas(sp.Verts[0], sp.Verts[1])
		ulx, uly := s.ToCanvas(sp.Verts[2], sp.Verts[3])
		urx, ury := s.ToCanvas(sp.Verts[4], sp.Verts[5])
		// dst = M * src + T, src in region pixels from its top-left.
		m00, m10 := (urx-ulx)/rw, (ury-uly)/rw
		m01, m11 := (blx-ulx)/rh, (bly-uly)/rh
		tint := [4]float64{sp.Color[0] * sp.Color[3], sp.Color[1] * sp.Color[3], sp.Color[2] * sp.Color[3], sp.Color[3]}
		identity := math.Abs(m00-1) < 1e-4 && math.Abs(m11-1) < 1e-4 && math.Abs(m01) < 1e-4 && math.Abs(m10) < 1e-4 &&
			math.Abs(ulx-math.Round(ulx)) < 1e-3 && math.Abs(uly-math.Round(uly)) < 1e-3
		if identity {
			tx, ty := int(math.Round(ulx)), int(math.Round(uly))
			for y := max(0, ty); y < min(h, ty+r.Dy()); y++ {
				for x := max(0, tx); x < min(w, tx+r.Dx()); x++ {
					c := atlas.Page.NRGBAAt(r.Min.X+x-tx, r.Min.Y+y-ty)
					over(acc[(y*w+x)*4:], premul(c), tint)
				}
			}
			continue
		}
		det := m00*m11 - m01*m10
		if math.Abs(det) < 1e-9 {
			continue
		}
		i00, i01, i10, i11 := m11/det, -m01/det, -m10/det, m00/det
		minX, maxX := math.Inf(1), math.Inf(-1)
		minY, maxY := math.Inf(1), math.Inf(-1)
		for k := 0; k < 4; k++ {
			cx, cy := s.ToCanvas(sp.Verts[k*2], sp.Verts[k*2+1])
			minX, maxX = math.Min(minX, cx), math.Max(maxX, cx)
			minY, maxY = math.Min(minY, cy), math.Max(maxY, cy)
		}
		x0, x1 := max(0, int(math.Floor(minX))-1), min(w-1, int(math.Ceil(maxX))+1)
		y0, y1 := max(0, int(math.Floor(minY))-1), min(h-1, int(math.Ceil(maxY))+1)
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				// Pixel centres: dst + .5 = M (src + .5) + T.
				dx, dy := float64(x)+0.5-ulx, float64(y)+0.5-uly
				u, v := i00*dx+i01*dy-0.5, i10*dx+i11*dy-0.5
				if u <= -1 || v <= -1 || u >= rw || v >= rh {
					continue
				}
				c := bilinear(atlas.Page, r, u, v)
				if c[3] == 0 {
					continue
				}
				over(acc[(y*w+x)*4:], c, tint)
			}
		}
	}
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		a := acc[i*4+3]
		if a <= 0 {
			continue
		}
		o := out.Pix[i*4 : i*4+4]
		for ch := 0; ch < 3; ch++ {
			o[ch] = to8(acc[i*4+ch] / a)
		}
		o[3] = to8(a)
		if o[3] == 0 {
			o[0], o[1], o[2] = 0, 0, 0
		}
	}
	return out, nil
}

func premul(c color.NRGBA) [4]float64 {
	a := float64(c.A) / 255
	return [4]float64{float64(c.R) / 255 * a, float64(c.G) / 255 * a, float64(c.B) / 255 * a, a}
}

func bilinear(page *image.NRGBA, r image.Rectangle, u, v float64) [4]float64 {
	fx, fy := math.Floor(u), math.Floor(v)
	ax, ay := u-fx, v-fy
	x0, y0 := int(fx), int(fy)
	wx := [2]float64{1 - ax, ax}
	wy := [2]float64{1 - ay, ay}
	var out [4]float64
	for j := 0; j < 2; j++ {
		for i := 0; i < 2; i++ {
			x, y := x0+i, y0+j
			if x < 0 || y < 0 || x >= r.Dx() || y >= r.Dy() {
				continue
			}
			wgt := wx[i] * wy[j]
			p := premul(page.NRGBAAt(r.Min.X+x, r.Min.Y+y))
			for ch := range out {
				out[ch] += wgt * p[ch]
			}
		}
	}
	return out
}

func over(dst []float64, src, tint [4]float64) {
	a := src[3] * tint[3]
	for ch := 0; ch < 3; ch++ {
		dst[ch] = src[ch]*tint[ch] + dst[ch]*(1-a)
	}
	dst[3] = a + dst[3]*(1-a)
}

func to8(v float64) uint8 {
	return uint8(math.Max(0, math.Min(255, math.Round(v*255))))
}
