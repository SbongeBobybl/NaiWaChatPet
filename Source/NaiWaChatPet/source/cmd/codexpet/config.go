package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"io/fs"
	"math"
	"path"
	"strings"
	"time"

	"codexpet/reconstruction/animation"
	"codexpet/reconstruction/compositor"
)

type clipJSON struct {
	Frames     int   `json:"frames"`
	DurationNS int64 `json:"duration_ns"`
}
type canvasJSON struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}
type gateJSON struct {
	Passed bool   `json:"passed"`
	Reason string `json:"reason"`
}
type scaleJSON struct {
	Default float64   `json:"default"`
	Min     float64   `json:"min"`
	Max     float64   `json:"max"`
	Step    float64   `json:"step"`
	Menu    []float64 `json:"menu"`
}
type dropJSON struct {
	X               float64 `json:"x"`
	StartY          float64 `json:"start_y"`
	GroundY         float64 `json:"ground_y"`
	InitialVelocity float64 `json:"initial_velocity"`
	Gravity         float64 `json:"gravity"`
	Restitution     float64 `json:"restitution"`
	SettleSpeed     float64 `json:"settle_speed"`
	RevealNS        int64   `json:"reveal_ns"`
	SquashNS        int64   `json:"squash_ns"`
}

func (d dropJSON) config() animation.DropConfig {
	return animation.DropConfig{X: d.X, StartY: d.StartY, GroundY: d.GroundY, InitialVelocity: d.InitialVelocity, Gravity: d.Gravity, Restitution: d.Restitution, SettleSpeed: d.SettleSpeed, RevealTime: time.Duration(d.RevealNS), SquashTime: time.Duration(d.SquashNS)}
}

type runtimeJSON struct {
	Version        int                 `json:"version"`
	Canvas         canvasJSON          `json:"canvas"`
	Clips          map[string]clipJSON `json:"clips"`
	DigestDelayNS  int64               `json:"digest_delay_ns"`
	BeginDropFrame int                 `json:"begin_drop_frame"`
	BodyDir        string              `json:"body_dir"`
	HoverMask      string              `json:"hover_mask"`
	HoverMaskMode  string              `json:"hover_mask_mode"`
	Prop           string              `json:"prop"`
	Scale          scaleJSON           `json:"scale"`
	Drop           dropJSON            `json:"drop_geometry"`
	Gate           gateJSON            `json:"release_gate"`
	Hashes         map[string]string   `json:"frames_sha256"`
}
type loadedAssets struct {
	Config runtimeJSON
	Engine *animation.Engine
	Bodies *compositor.Cache
	Mask   *image.NRGBA
	Prop   *image.NRGBA
	Canvas image.Rectangle
	Counts map[string]int
}

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }
func assetPath(p string) error {
	if !fs.ValidPath(p) || p == "." {
		return fmt.Errorf("invalid asset path %q", p)
	}
	return nil
}
func (c runtimeJSON) validate() (animation.Config, error) {
	if c.Version != 1 {
		return animation.Config{}, errors.New("runtime.json version must be 1")
	}
	if !c.Gate.Passed {
		return animation.Config{}, fmt.Errorf("asset release gate has not passed: %s", c.Gate.Reason)
	}
	if c.Canvas.Width < 1 || c.Canvas.Height < 1 || c.Canvas.Width > 4096 || c.Canvas.Height > 4096 {
		return animation.Config{}, errors.New("invalid stable canvas")
	}
	if c.BodyDir == "" {
		c.BodyDir = "."
	}
	if !fs.ValidPath(c.BodyDir) {
		return animation.Config{}, errors.New("invalid body_dir")
	}
	if c.HoverMaskMode != "" && c.HoverMaskMode != "auto" && c.HoverMaskMode != "alpha" && c.HoverMaskMode != "luminance" && c.HoverMaskMode != "body" {
		return animation.Config{}, errors.New("invalid hover_mask_mode")
	}
	if err := assetPath(c.HoverMask); err != nil {
		return animation.Config{}, err
	}
	if err := assetPath(c.Prop); err != nil {
		return animation.Config{}, err
	}
	s := c.Scale
	if !finite(s.Default) || !finite(s.Min) || !finite(s.Max) || !finite(s.Step) || s.Min <= 0 || s.Max < s.Min || s.Max > 8 || s.Step <= 0 || s.Default < s.Min || s.Default > s.Max {
		return animation.Config{}, errors.New("invalid global scale configuration")
	}
	for _, v := range s.Menu {
		if !finite(v) || v < s.Min || v > s.Max {
			return animation.Config{}, errors.New("menu scale outside global range")
		}
	}
	if err := c.Drop.config().Validate(); err != nil {
		return animation.Config{}, err
	}
	if c.Drop.X < 0 || c.Drop.X >= float64(c.Canvas.Width) || c.Drop.StartY < 0 || c.Drop.GroundY > float64(c.Canvas.Height) {
		return animation.Config{}, errors.New("drop geometry lies outside stable canvas")
	}
	ac := animation.Config{Clips: map[animation.State]animation.Clip{}, DigestDelay: time.Duration(c.DigestDelayNS), BeginDropFrame: c.BeginDropFrame}
	var total int64
	for _, s := range animation.States {
		row, ok := c.Clips[string(s)]
		if !ok || row.Frames < 1 || row.Frames > 2000 || row.DurationNS <= 0 {
			return ac, fmt.Errorf("missing or invalid clip %s", s)
		}
		ac.Clips[s] = animation.Clip{Frames: row.Frames, Step: time.Duration(row.DurationNS)}
		total += int64(row.Frames)
	}
	if total*int64(c.Canvas.Width)*int64(c.Canvas.Height) > 256*1024*1024 {
		return ac, errors.New("decoded asset cache exceeds pixel limit")
	}
	if _, err := animation.New(ac); err != nil {
		return ac, err
	}
	return ac, nil
}
func loadAssets(files fs.FS, configPath string) (*loadedAssets, error) {
	if err := assetPath(configPath); err != nil {
		return nil, err
	}
	file, err := files.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("asset configuration unavailable: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(file, 4*1024*1024+1))
	closeErr := file.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(data) > 4*1024*1024 {
		return nil, errors.New("runtime.json too large")
	}
	var cfg runtimeJSON
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("runtime.json: %w", err)
	}
	var extra any
	if err = dec.Decode(&extra); err != io.EOF {
		return nil, errors.New("runtime.json must contain exactly one object")
	}
	if cfg.BodyDir == "" {
		cfg.BodyDir = "."
	}
	ac, err := cfg.validate()
	if err != nil {
		return nil, err
	}
	engine, err := animation.New(ac)
	if err != nil {
		return nil, err
	}
	canvas := image.Rect(0, 0, cfg.Canvas.Width, cfg.Canvas.Height)
	counts := map[string]int{}
	memory := assetMemory{}
	var compressed int64
	readVerified := func(name string) ([]byte, error) {
		if err := assetPath(name); err != nil {
			return nil, err
		}
		want, ok := cfg.Hashes[name]
		if !ok || len(want) != 64 {
			return nil, fmt.Errorf("missing release SHA-256 for %s", name)
		}
		if _, err := hex.DecodeString(want); err != nil {
			return nil, fmt.Errorf("invalid release SHA-256 for %s", name)
		}
		f, err := files.Open(name)
		if err != nil {
			return nil, fmt.Errorf("missing released asset %s: %w", name, err)
		}
		b, err := io.ReadAll(io.LimitReader(f, 16*1024*1024+1))
		ce := f.Close()
		if err != nil {
			return nil, err
		}
		if ce != nil {
			return nil, ce
		}
		if len(b) > 16*1024*1024 {
			return nil, fmt.Errorf("asset %s too large", name)
		}
		compressed += int64(len(b))
		if compressed > 256*1024*1024 {
			return nil, errors.New("compressed asset bundle exceeds limit")
		}
		sum := sha256.Sum256(b)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), want) {
			return nil, fmt.Errorf("release SHA-256 mismatch for %s", name)
		}
		return b, nil
	}
	decode := func(name string, b []byte, expected image.Rectangle) (*image.NRGBA, error) {
		header, err := png.DecodeConfig(bytes.NewReader(b))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if header.Width < 1 || header.Height < 1 || header.Width > 4096 || header.Height > 4096 {
			return nil, fmt.Errorf("%s: invalid image dimensions", name)
		}
		if !expected.Empty() && (header.Width != expected.Dx() || header.Height != expected.Dy()) {
			return nil, fmt.Errorf("%s: stable canvas mismatch", name)
		}
		im, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		n := image.NewNRGBA(image.Rect(0, 0, header.Width, header.Height))
		draw.Draw(n, n.Bounds(), im, im.Bounds().Min, draw.Src)
		for i := 0; i < len(n.Pix); i += 4 {
			if n.Pix[i+3] == 0 && (n.Pix[i] != 0 || n.Pix[i+1] != 0 || n.Pix[i+2] != 0) {
				return nil, fmt.Errorf("%s: alpha-zero RGB pixels violate release contract", name)
			}
		}
		return n, nil
	}
	for _, s := range animation.States {
		count := cfg.Clips[string(s)].Frames
		counts[string(s)] = count
		for i := 0; i < count; i++ {
			leaf := fmt.Sprintf("%s_%03d.png", s, i)
			full := path.Join(cfg.BodyDir, leaf)
			b, err := readVerified(full)
			if err != nil {
				return nil, err
			}
			if _, err = decode(full, b, canvas); err != nil {
				return nil, err
			}
			memory[leaf] = b
		}
	}
	cache, err := compositor.Load(memory, counts, canvas)
	if err != nil {
		return nil, err
	}
	mb, err := readVerified(cfg.HoverMask)
	if err != nil {
		return nil, err
	}
	mask, err := decode(cfg.HoverMask, mb, canvas)
	if err != nil {
		return nil, err
	}
	mask = normalizedMask(mask, cfg.HoverMaskMode)
	pb, err := readVerified(cfg.Prop)
	if err != nil {
		return nil, err
	}
	prop, err := decode(cfg.Prop, pb, image.Rectangle{})
	if err != nil {
		return nil, err
	}
	return &loadedAssets{cfg, engine, cache, mask, prop, canvas, counts}, nil
}

type assetMemory map[string][]byte

func (m assetMemory) Open(name string) (fs.File, error) {
	b, ok := m[name]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &assetFile{Reader: bytes.NewReader(b), name: name, size: int64(len(b))}, nil
}

type assetFile struct {
	*bytes.Reader
	name string
	size int64
}

func (f *assetFile) Close() error               { return nil }
func (f *assetFile) Stat() (fs.FileInfo, error) { return assetInfo{f.name, f.size}, nil }

type assetInfo struct {
	name string
	size int64
}

func (f assetInfo) Name() string       { return path.Base(f.name) }
func (f assetInfo) Size() int64        { return f.size }
func (f assetInfo) Mode() fs.FileMode  { return 0444 }
func (f assetInfo) ModTime() time.Time { return time.Time{} }
func (f assetInfo) IsDir() bool        { return false }
func (f assetInfo) Sys() any           { return nil }
