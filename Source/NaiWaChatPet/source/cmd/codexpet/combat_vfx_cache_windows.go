//go:build windows

package main

import (
	"image"
	"sync"

	"codexpet/reconstruction/compositor"
)

const combatEffectFrames = 24
const combatEffectRaster = 192
const combatScaledEffectBytes = 8 << 20

type combatEffectKey struct {
	kind                 string
	size, frame, quality int
}
type combatEffectPixels struct {
	pixels []byte
	used   uint64
}
type combatVFXCache struct {
	quality float64
	mu      sync.RWMutex
	frames  map[string][][]byte
	done    chan struct{}
	scaled  map[combatEffectKey]combatEffectPixels
	bytes   int
	clock   uint64
}

func (a *desktopHost) warmCombatVFX(u *combatPresentation) {
	cache := &combatVFXCache{frames: map[string][][]byte{}, scaled: map[combatEffectKey]combatEffectPixels{}, done: make(chan struct{}), quality: a.play.settings.Game.Effects}
	u.vfx = cache
	quality := a.play.settings.Game.Effects
	for _, kind := range []string{"explosion", "power", "rollhit", "dodge", "roll"} {
		cache.frames[kind] = make([][]byte, combatEffectFrames)
	}
	go func() {
		defer close(cache.done)
		for _, kind := range []string{"explosion", "power", "rollhit", "dodge", "roll"} {
			for i := 0; i < combatEffectFrames; i++ {
				select {
				case <-u.context.Done():
					return
				default:
				}
				cache.basePixels(kind, i, quality)
			}
		}
	}()
}

// Raster work is independent of desktop DPI. Cache immutable frames; only
// inexpensive resizing is needed for the requested on-screen effect size.
func (c *combatVFXCache) basePixels(kind string, frame int, quality float64) []byte {
	c.mu.RLock()
	frames := c.frames[kind]
	var pixels []byte
	if quality == c.quality && len(frames) > frame {
		pixels = frames[frame]
	}
	c.mu.RUnlock()
	if pixels != nil {
		return pixels
	}
	pixels = combatEffectSprite(kind, combatEffectRaster, float64(frame)/(combatEffectFrames-1), quality)
	if quality == c.quality && len(frames) > frame {
		c.mu.Lock()
		if c.frames[kind][frame] == nil {
			c.frames[kind][frame] = pixels
		}
		pixels = c.frames[kind][frame]
		c.mu.Unlock()
	}
	return pixels
}
func (u *combatPresentation) effectPixels(kind string, size int, t, quality float64) []byte {
	c := u.vfx
	if c == nil {
		return combatEffectSprite(kind, size, t, quality)
	}
	frame := min(combatEffectFrames-1, max(0, int(t*(combatEffectFrames-1)+.5)))
	key := combatEffectKey{kind, size, frame, int(quality*100 + .5)}
	// Scaled frames are used only on the UI thread; the warmer owns no entries.
	c.clock++
	if cached, ok := c.scaled[key]; ok {
		cached.used = c.clock
		c.scaled[key] = cached
		return cached.pixels
	}
	pixels := c.basePixels(kind, frame, quality)
	if size != combatEffectRaster {
		// The byte channels remain in BGRA order through the linear resampler.
		source := &image.RGBA{Pix: pixels, Stride: combatEffectRaster * 4, Rect: image.Rect(0, 0, combatEffectRaster, combatEffectRaster)}
		if resized, err := compositor.Resize(source, size, size); err == nil {
			pixels = resized.Pix
		}
	}
	if len(pixels) <= combatScaledEffectBytes {
		for c.bytes+len(pixels) > combatScaledEffectBytes && len(c.scaled) > 0 {
			var oldest combatEffectKey
			age := ^uint64(0)
			for k, entry := range c.scaled {
				if entry.used < age {
					oldest, age = k, entry.used
				}
			}
			c.bytes -= len(c.scaled[oldest].pixels)
			delete(c.scaled, oldest)
		}
		c.scaled[key] = combatEffectPixels{pixels, c.clock}
		c.bytes += len(pixels)
	}
	return pixels
}
