package emotion

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Anchor struct {
	Frame int     `json:"frame"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
}
type Frame struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
}
type Canvas struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}
type Manifest struct {
	Name           string   `json:"name"`
	Emotion        string   `json:"emotion"`
	FPS            float64  `json:"fps"`
	FrameCount     int      `json:"frameCount"`
	Loop           bool     `json:"loop"`
	Priority       int      `json:"priority"`
	MouthAnchor    Anchor   `json:"mouthAnchor"`
	MouthKeyframes []Anchor `json:"mouthKeyframes,omitempty"`
	TransitionIn   string   `json:"transitionIn"`
	TransitionOut  string   `json:"transitionOut"`
	Canvas         Canvas   `json:"canvas"`
	GroundY        float64  `json:"groundY"`
	// FeetX is where a filmed scene's character stands on its first frame,
	// when that is not where the idle pet stands. Zero means it is.
	FeetX        float64  `json:"feetX,omitempty"`
	Frames       []Frame  `json:"frames"`
	Reviewed     bool     `json:"reviewed"`
	SourceSHA256 string   `json:"sourceSha256"`
	Issues       []string `json:"issues,omitempty"`
}
type loadedClip struct {
	Manifest Manifest
	Encoded  [][]byte
}
type Library struct {
	clips map[string]loadedClip
	cache map[string]*image.NRGBA
	order []string
}

var safeName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

func LoadLibrary(root string, canvas image.Rectangle) (*Library, []error) {
	l := &Library{clips: map[string]loadedClip{}, cache: map[string]*image.NRGBA{}}
	var warnings []error
	var manifests []string
	if _, e := os.Stat(root); os.IsNotExist(e) {
		return l, nil
	}
	walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() && (d.Name() == "manifest.json" || strings.HasSuffix(d.Name(), ".animation.json")) {
			manifests = append(manifests, p)
		}
		if len(manifests) > 256 {
			return errors.New("too many manifests")
		}
		return nil
	})
	if walkErr != nil {
		warnings = append(warnings, walkErr)
	}
	sort.Strings(manifests)
	total := 0
	for _, p := range manifests {
		clip, e := loadClip(p, canvas)
		if e != nil {
			warnings = append(warnings, fmt.Errorf("%s: %w", filepath.Base(p), e))
			continue
		}
		if _, ok := l.clips[clip.Manifest.Name]; ok {
			warnings = append(warnings, errors.New("duplicate animation name"))
			continue
		}
		for _, b := range clip.Encoded {
			total += len(b)
		}
		if total > 128<<20 {
			warnings = append(warnings, errors.New("emotion resource byte budget exceeded"))
			break
		}
		l.clips[clip.Manifest.Name] = clip
	}
	for changed := true; changed; {
		changed = false
		for name, v := range l.clips {
			for _, bridge := range []string{v.Manifest.TransitionIn, v.Manifest.TransitionOut} {
				if bridge != "" {
					if _, ok := l.clips[bridge]; !ok {
						delete(l.clips, name)
						changed = true
						warnings = append(warnings, fmt.Errorf("%s: missing transition %s", name, bridge))
						break
					}
				}
			}
		}
	}
	return l, warnings
}
func loadClip(path string, canvas image.Rectangle) (loadedClip, error) {
	b, e := os.ReadFile(path)
	if e != nil || len(b) > 1<<20 {
		return loadedClip{}, errors.New("manifest unavailable or too large")
	}
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if dec.Decode(&m) != nil || dec.Decode(new(any)) != io.EOF {
		return loadedClip{}, errors.New("invalid manifest")
	}
	previous := -1
	for _, a := range m.MouthKeyframes {
		if a.Frame <= previous {
			return loadedClip{}, errors.New("mouth keyframes must be ordered and unique")
		}
		previous = a.Frame
	}
	if !m.Reviewed {
		return loadedClip{}, errors.New("animation has not completed frame review")
	}
	if !safeName.MatchString(m.Name) || !safeName.MatchString(m.Emotion) || m.FrameCount < 1 || m.FrameCount > 600 || len(m.Frames) != m.FrameCount || m.FPS < 1 || m.FPS > 60 || m.Priority < 1 || m.Priority > 50 || m.Canvas.Width != canvas.Dx() || m.Canvas.Height != canvas.Dy() || m.GroundY < 1 || m.GroundY > float64(canvas.Dy()) || m.FeetX < 0 || m.FeetX > float64(canvas.Dx()) {
		return loadedClip{}, errors.New("invalid animation geometry/timing")
	}
	for _, a := range append([]Anchor{m.MouthAnchor}, m.MouthKeyframes...) {
		if a.X < 0 || a.Y < 0 || a.X > float64(canvas.Dx()) || a.Y > float64(canvas.Dy()) || a.Frame < 0 || a.Frame >= m.FrameCount {
			return loadedClip{}, errors.New("invalid mouth anchor")
		}
	}
	out := loadedClip{Manifest: m}
	bytesTotal := 0
	for _, f := range m.Frames {
		if !fs.ValidPath(f.File) || f.File == "." || strings.Contains(f.File, "\\") {
			return out, errors.New("frame path escapes animation folder")
		}
		p := filepath.Join(filepath.Dir(path), filepath.FromSlash(f.File))
		for parent := filepath.Dir(p); parent != filepath.Dir(path); parent = filepath.Dir(parent) {
			info, err := os.Lstat(parent)
			if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return out, errors.New("frame parent must be an ordinary directory")
			}
		}
		info, e := os.Lstat(p)
		if e != nil || !info.Mode().IsRegular() || info.Size() > 8<<20 {
			return out, errors.New("frame missing or not a regular PNG")
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return out, e
		}
		bytesTotal += len(b)
		if bytesTotal > 64<<20 {
			return out, errors.New("clip byte budget exceeded")
		}
		sum := sha256.Sum256(b)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), f.SHA256) {
			return out, errors.New("frame SHA-256 mismatch")
		}
		im, e := png.Decode(bytes.NewReader(b))
		if e != nil || im.Bounds() != canvas {
			return out, errors.New("PNG canvas mismatch")
		}
		n, ok := im.(*image.NRGBA)
		if !ok {
			return out, errors.New("frame must be straight RGBA PNG")
		}
		transparent := false
		for i := 0; i < len(n.Pix); i += 4 {
			if n.Pix[i+3] == 0 {
				transparent = true
				if n.Pix[i] != 0 || n.Pix[i+1] != 0 || n.Pix[i+2] != 0 {
					return out, errors.New("alpha-zero RGB is not zero")
				}
			}
		}
		if !transparent {
			return out, errors.New("frame has no transparent canvas padding")
		}
		out.Encoded = append(out.Encoded, b)
	}
	return out, nil
}
func (l *Library) Clips() []Clip {
	var out []Clip
	for _, v := range l.clips {
		m := v.Manifest
		out = append(out, Clip{Name: m.Name, Emotion: m.Emotion, Frames: m.FrameCount, Step: time.Duration(float64(time.Second) / m.FPS), Loop: m.Loop, Priority: m.Priority, TransitionIn: m.TransitionIn, TransitionOut: m.TransitionOut})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func (l *Library) Manifest(name string) (Manifest, bool) {
	v, ok := l.clips[name]
	return v.Manifest, ok
}
func (l *Library) Frame(name string, index int) (*image.NRGBA, error) {
	v, ok := l.clips[name]
	if !ok || index < 0 || index >= len(v.Encoded) {
		return nil, errors.New("unknown emotion frame")
	}
	key := fmt.Sprintf("%s/%d", name, index)
	n := l.cache[key]
	if n == nil {
		im, e := png.Decode(bytes.NewReader(v.Encoded[index]))
		if e != nil {
			return nil, e
		}
		n = im.(*image.NRGBA)
		if len(l.order) == 8 {
			delete(l.cache, l.order[0])
			l.order = l.order[1:]
		}
		l.cache[key] = n
		l.order = append(l.order, key)
	}
	clone := image.NewNRGBA(n.Bounds())
	copy(clone.Pix, n.Pix)
	return clone, nil
}
