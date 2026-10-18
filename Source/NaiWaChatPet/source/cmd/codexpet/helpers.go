package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"math"
	"path/filepath"
	"strings"
	"time"

	"codexpet/reconstruction/animation"
)

type rectJSON struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

func (r rectJSON) valid() bool {
	return r.W > 0 && r.H > 0 && r.W <= 8192 && r.H <= 8192 && r.X >= math.MinInt32 && r.X <= math.MaxInt32-r.W && r.Y >= math.MinInt32 && r.Y <= math.MaxInt32-r.H
}
func overlaps(a, b rectJSON) bool {
	if !a.valid() || !b.valid() {
		return false
	}
	w := min(a.X+a.W, b.X+b.W) - max(a.X, b.X)
	h := min(a.Y+a.H, b.Y+b.H) - max(a.Y, b.Y)
	return w > max(18, min(a.W, b.W)/4) && h > max(25, min(a.H, b.H)/3)
}

type contactLatch struct{ touch bool }

func (l *contactLatch) update(current rectJSON, peers []rectJSON) bool {
	hit := false
	for _, p := range peers {
		if overlaps(current, p) {
			hit = true
			break
		}
	}
	edge := hit && !l.touch
	l.touch = hit
	return edge
}

type hoverHold struct {
	inside  bool
	lastHit time.Duration
}

func (h *hoverHold) update(now time.Duration, hit bool) bool {
	if hit {
		h.lastHit = now
		h.inside = true
	} else if h.inside && now-h.lastHit > 250*time.Millisecond {
		h.inside = false
	}
	return h.inside
}
func scaledCanvas(c image.Rectangle, scale float64) (int, int, error) {
	if !finite(scale) || scale <= 0 {
		return 0, 0, errors.New("invalid scale")
	}
	w, h := int(math.Round(float64(c.Dx())*scale)), int(math.Round(float64(c.Dy())*scale))
	if w < 1 || h < 1 || w > 8192 || h > 8192 {
		return 0, 0, errors.New("scaled canvas outside render range")
	}
	return w, h, nil
}
func scaleBottom(y, oldHeight, newHeight int) int { return y + oldHeight - newHeight }
func scaleAnchor(y int, anchor, oldFactor, newFactor float64) int {
	return y + int(math.Round(anchor*oldFactor)) - int(math.Round(anchor*newFactor))
}
func maskHit(mask *image.NRGBA, x, y, w, h int) bool {
	if mask == nil || x < 0 || y < 0 || x >= w || y >= h || w <= 0 || h <= 0 {
		return false
	}
	sx := x * mask.Bounds().Dx() / w
	sy := y * mask.Bounds().Dy() / h
	return mask.NRGBAAt(sx, sy).A > 40
}
func uniqueTarget(dir, source string, now time.Time) (string, error) {
	name := filepath.Base(source)
	if name == "." || name == ".." || name == "" || strings.ContainsAny(name, "/\\:") {
		return "", errors.New("invalid source file name")
	}
	var entropy [12]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("%s_%s_%s", now.UTC().Format("20060102T150405.000000000"), hex.EncodeToString(entropy[:]), name)), nil
}

func transferProblemsText(problems []transferProblem) string {
	if len(problems) == 0 {
		return ""
	}
	const limit = 12
	var b strings.Builder
	for i, p := range problems {
		if i == limit {
			fmt.Fprintf(&b, "\n\u53e6\u6709 %d \u4e2a\u6587\u4ef6\u9700\u8981\u5904\u7406\u3002", len(problems)-limit)
			break
		}
		if i > 0 {
			b.WriteString("\n\n")
		}
		switch {
		case p.TargetVerified && p.SourceRemovalUncertain:
			b.WriteString("\u76ee\u6807\u526f\u672c\u5df2\u901a\u8fc7\u6821\u9a8c\uff1b\u539f\u6587\u4ef6\u5220\u9664\u72b6\u6001\u65e0\u6cd5\u786e\u8ba4\u3002\u8bf7\u6838\u5bf9\u4ee5\u4e0b\u4e24\u4e2a\u4f4d\u7f6e\u540e\u518d\u6e05\u7406\u91cd\u590d\u6587\u4ef6\u3002")
		case p.TargetVerified && !p.SourceRemoved:
			b.WriteString("\u76ee\u6807\u526f\u672c\u5df2\u901a\u8fc7\u6821\u9a8c\uff1b\u539f\u6587\u4ef6\u4ecd\u4fdd\u7559\uff0c\u4e24\u4efd\u6587\u4ef6\u90fd\u5728\u3002")
		case p.TargetVerified && p.SourceRemoved:
			b.WriteString("\u76ee\u6807\u526f\u672c\u5df2\u901a\u8fc7\u6821\u9a8c\uff0c\u539f\u6587\u4ef6\u5df2\u79fb\u9664\u3002")
		default:
			b.WriteString("\u8f6c\u79fb\u672a\u5b8c\u6210\uff0c\u539f\u6587\u4ef6\u5df2\u4fdd\u7559\uff1b\u76ee\u6807\u6587\u4ef6\u672a\u901a\u8fc7\u6821\u9a8c\u3002")
		}
		b.WriteString("\n\u539f\u6587\u4ef6\uff1a")
		b.WriteString(p.Source)
		if p.Target != "" {
			b.WriteString("\n\u76ee\u6807\u4f4d\u7f6e\uff1a")
			b.WriteString(p.Target)
		}
	}
	return b.String()
}

type actionQueue struct {
	storedBatches int
	laugh         bool
}

func (q *actionQueue) stored() { q.storedBatches++ }

// service removes a successful-storage notification only after Engine accepts it.
// Fight exits through its bridge before the next requested action starts.
func (q *actionQueue) service(e *animation.Engine, now time.Duration) (bool, error) {
	s := e.Snapshot()
	if s.DigestPending {
		return false, nil
	}
	if q.storedBatches == 0 && !q.laugh {
		return false, nil
	}
	if s.State == animation.Fight || s.State == animation.ToFight {
		_, err := e.Fire(now, animation.HoverOff)
		return false, err
	}
	if s.State != animation.Idle {
		return false, nil
	}
	input := animation.DoubleClick
	if q.storedBatches > 0 {
		input = animation.FilesStored
	}
	accepted, err := e.Fire(now, input)
	if err != nil {
		return false, err
	}
	if accepted {
		if input == animation.FilesStored {
			q.storedBatches--
		} else {
			q.laugh = false
		}
	}
	return accepted, nil
}
func (q *actionQueue) requestLaugh(e *animation.Engine, now time.Duration) (bool, error) {
	s := e.Snapshot()
	if q.storedBatches > 0 || s.DigestPending {
		return false, nil
	}
	switch s.State {
	case animation.ToEat, animation.Eat, animation.FromEat, animation.ToBack, animation.Poop, animation.FromBack, animation.AfterPoop, animation.ToLaugh, animation.Laugh, animation.FromLaugh:
		return false, nil
	}
	q.laugh = true
	return q.service(e, now)
}

// Legacy hover masks are opaque black/white PNGs. Auto uses their luminance,
// while true alpha masks use alpha. No opaque-black rectangle becomes a hit.
func normalizedMask(mask *image.NRGBA, mode string) *image.NRGBA {
	opaque := true
	for i := 3; i < len(mask.Pix); i += 4 {
		if mask.Pix[i] != 255 {
			opaque = false
			break
		}
	}
	luminance := mode == "luminance" || ((mode == "" || mode == "auto") && opaque)
	if !luminance {
		return mask
	}
	result := image.NewNRGBA(mask.Bounds())
	for y := mask.Bounds().Min.Y; y < mask.Bounds().Max.Y; y++ {
		for x := mask.Bounds().Min.X; x < mask.Bounds().Max.X; x++ {
			i := mask.PixOffset(x, y)
			j := result.PixOffset(x, y)
			a := byte((uint32(mask.Pix[i])*54 + uint32(mask.Pix[i+1])*183 + uint32(mask.Pix[i+2])*19 + 128) >> 8)
			result.Pix[j+3] = a
		}
	}
	return result
}
