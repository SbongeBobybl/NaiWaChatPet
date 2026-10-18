package compositor

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"testing/fstest"
)

func TestPremultipliedTopDownBGRA(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 1, 3))
	src.SetNRGBA(0, 0, color.NRGBA{255, 64, 0, 128})
	src.SetNRGBA(0, 1, color.NRGBA{255, 0, 255, 0})
	src.SetNRGBA(0, 2, color.NRGBA{0, 0, 255, 255})
	want := []byte{0, 32, 128, 128, 0, 0, 0, 0, 255, 0, 0, 255}
	if got := BGRA(src); !bytes.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestTransparentInvalidRGBIsCleared(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 1, 1))
	src.Pix = []byte{255, 0, 255, 0}
	if got := BGRA(src); !bytes.Equal(got, []byte{0, 0, 0, 0}) {
		t.Fatal(got)
	}
}

func TestClearBuffersAndBodyOcclusion(t *testing.T) {
	b := image.Rect(0, 0, 2, 1)
	body := image.NewNRGBA(b)
	body.SetNRGBA(0, 0, color.NRGBA{0, 255, 0, 255})
	prop := image.NewNRGBA(b)
	prop.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	prop.SetNRGBA(1, 0, color.NRGBA{255, 0, 0, 255})
	p := Layer{prop, image.Point{}}
	first := Compose(b, Layer{body, image.Point{}}, &p, nil)
	if first.RGBAAt(0, 0) != (color.RGBA{0, 255, 0, 255}) {
		t.Fatal("body must occlude prop")
	}
	second := Compose(b, Layer{body, image.Point{}}, nil, nil)
	if second.RGBAAt(1, 0).A != 0 {
		t.Fatal("stale prop remained on canvas")
	}
}

func TestCacheCannotBeChangedViaReturnedFrame(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	src.SetNRGBA(0, 0, color.NRGBA{42, 84, 126, 255})
	var encoded bytes.Buffer
	png.Encode(&encoded, src)
	cache, err := Load(fstest.MapFS{"idle_000.png": &fstest.MapFile{Data: encoded.Bytes()}}, map[string]int{"idle": 1}, src.Bounds())
	if err != nil {
		t.Fatal(err)
	}
	a, _ := cache.BodyCopy("idle", 0)
	a.Pix[0] = 255
	b, _ := cache.BodyCopy("idle", 0)
	if b.Pix[0] != 42 {
		t.Fatal("caller changed shared cache")
	}
	if _, err := cache.BodyCopy("idle", 1); err == nil {
		t.Fatal("accepted out of range frame")
	}
}

func TestResizeAvoidsTransparentPixelColorFringe(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	src.SetNRGBA(0, 0, color.NRGBA{255, 255, 0, 255})
	src.SetNRGBA(1, 0, color.NRGBA{255, 0, 255, 0})
	out, err := Resize(src, 8, 4)
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 8; x++ {
			c := out.RGBAAt(x, y)
			if c.B != 0 || c.R != c.G || c.R > c.A {
				t.Fatal("transparent magenta leaked", c)
			}
		}
	}
}

// TestBicubicPremultiplicationInvariant reproduces the Windows failure.
// Catmull-Rom's negative interpolation lobes can make color exceed alpha
// around an opaque/transparent boundary even when source is valid.
func TestBicubicPremultiplicationInvariant(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 12, 12))
	for y := 1; y < 11; y++ {
		for x := 1; x < 11; x++ {
			if (x-6)*(x-6)+(y-6)*(y-6) <= 20 {
				src.SetNRGBA(x, y, color.NRGBA{255, 205, 75, 255})
			}
		}
	}
	for _, wh := range [][2]int{{4, 4}, {7, 7}, {9, 9}, {18, 18}, {153, 204}} {
		scaled, err := Resize(src, wh[0], wh[1])
		if err != nil {
			t.Fatal(err)
		}
		bgra := BGRA(scaled)
		for i := 0; i < len(bgra); i += 4 {
			if bgra[i] > bgra[i+3] || bgra[i+1] > bgra[i+3] || bgra[i+2] > bgra[i+3] {
				t.Fatalf("illegal BGRA at %dx%d pixel %d: %v", wh[0], wh[1], i/4, bgra[i:i+4])
			}
		}
	}
}

// plain hides an image's concrete type so Resize and BGRA take their general
// paths, which are the reference for the direct ones.
type plain struct{ image.Image }

func canvasFixture() *image.RGBA {
	frame := image.NewRGBA(image.Rect(0, 0, 420, 560))
	for y := 0; y < 560; y++ {
		for x := 0; x < 420; x++ {
			dx, dy := float64(x-235)/135, float64(y-290)/255
			if d := dx*dx + dy*dy; d < 1 {
				a := uint8(255)
				if d > .9 {
					a = uint8(2550 * (1 - d))
				}
				// Premultiplied, as Compose produces.
				frame.SetRGBA(x, y, color.RGBA{uint8((120 + x/4) * int(a) / 255), uint8((60 + y/4) * int(a) / 255), uint8((x + y) % 200 * int(a) / 255), a})
			}
		}
	}
	return frame
}

func TestDirectPathsMatchTheGeneralOnes(t *testing.T) {
	frame := canvasFixture()
	for _, size := range [][2]int{{153, 204}, {273, 364}, {420, 560}, {630, 840}, {31, 41}} {
		fast, err := Resize(frame, size[0], size[1])
		if err != nil {
			t.Fatal(err)
		}
		slow, err := Resize(plain{frame}, size[0], size[1])
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(fast.Pix, slow.Pix) {
			t.Fatalf("resize to %dx%d differs between the direct and general paths", size[0], size[1])
		}
		if !bytes.Equal(BGRA(fast), BGRA(plain{fast})) {
			t.Fatalf("BGRA at %dx%d differs between the direct and general paths", size[0], size[1])
		}
	}
}

func BenchmarkResizeDefaultScale(b *testing.B) {
	frame := canvasFixture()
	for b.Loop() {
		if _, err := Resize(frame, 153, 204); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkResizeGeneralPath(b *testing.B) {
	frame := plain{canvasFixture()}
	for b.Loop() {
		if _, err := Resize(frame, 153, 204); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkResizeLargest(b *testing.B) {
	frame := canvasFixture()
	for b.Loop() {
		if _, err := Resize(frame, 410, 546); err != nil {
			b.Fatal(err)
		}
	}
}
