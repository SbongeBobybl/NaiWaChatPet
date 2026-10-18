package emotion

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func fixtureManifest(t *testing.T, root string) Manifest {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	im.SetNRGBA(1, 1, color.NRGBA{200, 150, 30, 128})
	var b bytes.Buffer
	_ = png.Encode(&b, im)
	_ = os.WriteFile(filepath.Join(root, "frame.png"), b.Bytes(), 0600)
	h := sha256.Sum256(b.Bytes())
	return Manifest{Name: "sample", Emotion: "happy", FPS: 30, FrameCount: 1, Priority: 20, Canvas: Canvas{4, 4}, GroundY: 3, MouthAnchor: Anchor{X: 1, Y: 1}, Reviewed: true, Frames: []Frame{{"frame.png", hex.EncodeToString(h[:])}}}
}
func writeManifest(t *testing.T, root string, m Manifest) {
	t.Helper()
	b, _ := json.Marshal(m)
	if e := os.WriteFile(filepath.Join(root, "manifest.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
}
func TestLibraryValidationAndImmutableFrameCache(t *testing.T) {
	root := t.TempDir()
	m := fixtureManifest(t, root)
	writeManifest(t, root, m)
	l, w := LoadLibrary(root, image.Rect(0, 0, 4, 4))
	if len(w) != 0 || len(l.Clips()) != 1 {
		t.Fatal(w)
	}
	f, e := l.Frame("sample", 0)
	if e != nil {
		t.Fatal(e)
	}
	f.Pix[0] = 255
	f2, _ := l.Frame("sample", 0)
	if f2.Pix[0] != 0 {
		t.Fatal("cache mutated by caller")
	}
}
func TestLibraryRejectsHashReviewPathAndTransitionErrors(t *testing.T) {
	for _, kind := range []string{"hash", "review", "escape", "transition"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			m := fixtureManifest(t, root)
			switch kind {
			case "hash":
				m.Frames[0].SHA256 = "00"
			case "review":
				m.Reviewed = false
			case "escape":
				m.Frames[0].File = "../frame.png"
			case "transition":
				m.TransitionIn = "missing"
			}
			writeManifest(t, root, m)
			l, w := LoadLibrary(root, image.Rect(0, 0, 4, 4))
			if len(w) == 0 || len(l.Clips()) != 0 {
				t.Fatal("bad resource accepted", kind)
			}
		})
	}
}

func TestLibraryRejectsTrailingJSONAndUnorderedAnchors(t *testing.T) {
	root := t.TempDir()
	m := fixtureManifest(t, root)
	writeManifest(t, root, m)
	f, _ := os.OpenFile(filepath.Join(root, "manifest.json"), os.O_APPEND|os.O_WRONLY, 0600)
	_, _ = f.WriteString(" {}")
	_ = f.Close()
	if _, e := loadClip(filepath.Join(root, "manifest.json"), image.Rect(0, 0, 4, 4)); e == nil {
		t.Fatal("trailing JSON accepted")
	}
	m.MouthKeyframes = []Anchor{{Frame: 0}, {Frame: 0}}
	writeManifest(t, root, m)
	if _, e := loadClip(filepath.Join(root, "manifest.json"), image.Rect(0, 0, 4, 4)); e == nil {
		t.Fatal("duplicate anchors accepted")
	}
}
