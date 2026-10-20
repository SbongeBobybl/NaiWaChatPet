"""Effect sprites. None of these touch the character art.

Flames and smoke are built from periodic noise, so a flipbook of N frames that
scrolls through one noise period loops without a seam.
"""

from __future__ import annotations

import numpy as np
from PIL import Image, ImageDraw, ImageFilter

RNG = np.random.default_rng(20261020)


def periodic_noise(h: int, w: int, beta: float = 2.2, seed: int = 0) -> np.ndarray:
    """Smooth noise that tiles in both directions, normalised to [-1, 1]."""
    rng = np.random.default_rng(seed)
    white = rng.standard_normal((h, w))
    fy = np.fft.fftfreq(h)[:, None]
    fx = np.fft.fftfreq(w)[None, :]
    f = np.sqrt(fx * fx + fy * fy)
    f[0, 0] = 1.0
    spec = np.fft.fft2(white) / f ** (beta / 2.0)
    spec[0, 0] = 0
    n = np.real(np.fft.ifft2(spec))
    n -= n.mean()
    return n / (np.abs(n).max() + 1e-9)


def smoothstep(e0, e1, x):
    t = np.clip((x - e0) / (e1 - e0), 0.0, 1.0)
    return t * t * (3 - 2 * t)


def ramp_colors(t: np.ndarray, stops: list[tuple[float, tuple[int, int, int]]]) -> np.ndarray:
    out = np.zeros(t.shape + (3,), np.float32)
    xs = [s[0] for s in stops]
    for c in range(3):
        out[..., c] = np.interp(t, xs, [s[1][c] for s in stops])
    return out


def to_rgba(rgb: np.ndarray, alpha: np.ndarray) -> np.ndarray:
    a = np.clip(alpha, 0, 1)
    out = np.zeros(alpha.shape + (4,), np.uint8)
    out[..., :3] = np.clip(np.round(rgb), 0, 255).astype(np.uint8)
    out[..., 3] = np.clip(np.round(a * 255), 0, 255).astype(np.uint8)
    out[out[..., 3] == 0, :3] = 0
    return out


def supersample(draw_fn, w: int, h: int, ss: int = 4) -> np.ndarray:
    img = Image.new("RGBA", (w * ss, h * ss), (0, 0, 0, 0))
    draw_fn(ImageDraw.Draw(img), ss)
    return premul_resize(np.array(img), (w, h))


def premul_resize(arr: np.ndarray, size: tuple[int, int]) -> np.ndarray:
    a = arr[..., 3:4].astype(np.float32) / 255.0
    pm = np.concatenate([arr[..., :3] * a, a * 255], axis=-1).astype(np.float32)
    out = np.stack(
        [np.array(Image.fromarray(pm[..., c]).resize(size, Image.Resampling.BOX)) for c in range(4)], axis=-1
    )
    alpha = out[..., 3] / 255.0
    rgb = np.divide(out[..., :3], np.maximum(alpha[..., None], 1e-6))
    return to_rgba(rgb, alpha)


# ---------------------------------------------------------------- fan

FAN_W, FAN_H = 88, 64
FAN_RIVET = (44.0, 58.0)  # pixel in the sprite where the guard is pinned


def fan_sprite() -> np.ndarray:
    """Open folding fan: cream and gold pleats, dark ribs, lacquered guards."""

    def draw(d: ImageDraw.ImageDraw, ss: int):
        px, py = FAN_RIVET[0] * ss, FAN_RIVET[1] * ss
        r_out, r_in = 54 * ss, 14 * ss
        start, sweep, pleats = -160.0, 140.0, 12
        cream = (250, 238, 210, 255)
        gold = (236, 186, 92, 255)
        shade = (222, 168, 80, 255)
        for i in range(pleats):
            a0 = start + sweep * i / pleats
            a1 = start + sweep * (i + 1) / pleats
            col = cream if i % 2 == 0 else gold
            d.pieslice([px - r_out, py - r_out, px + r_out, py + r_out], a0, a1, fill=col)
        # Painted band near the edge reads as a fan, not a slice of pie.
        for i in range(pleats):
            a0 = start + sweep * i / pleats
            a1 = start + sweep * (i + 1) / pleats
            d.arc([px - r_out + 5 * ss, py - r_out + 5 * ss, px + r_out - 5 * ss, py + r_out - 5 * ss], a0, a1,
                  fill=(205, 70, 48, 255) if i % 2 == 0 else (190, 58, 40, 255), width=4 * ss)
        d.pieslice([px - r_in, py - r_in, px + r_in, py + r_in], start - 2, start + sweep + 2, fill=(0, 0, 0, 0))
        for i in range(pleats + 1):
            ang = np.deg2rad(start + sweep * i / pleats)
            x0, y0 = px + np.cos(ang) * 6 * ss, py + np.sin(ang) * 6 * ss
            x1, y1 = px + np.cos(ang) * r_out, py + np.sin(ang) * r_out
            w = 3 * ss if i in (0, pleats) else max(1, ss)
            col = (96, 46, 30, 255) if i in (0, pleats) else (150, 104, 58, 255)
            d.line([(x0, y0), (x1, y1)], fill=col, width=w)
        d.arc([px - r_out, py - r_out, px + r_out, py + r_out], start, start + sweep, fill=shade, width=max(1, ss))
        d.ellipse([px - 4 * ss, py - 4 * ss, px + 4 * ss, py + 4 * ss], fill=(70, 40, 26, 255))
        d.ellipse([px - 1.5 * ss, py - 1.5 * ss, px + 1.5 * ss, py + 1.5 * ss], fill=(230, 196, 120, 255))

    return supersample(draw, FAN_W, FAN_H)


def fan_centroid(fan: np.ndarray) -> tuple[float, float]:
    a = fan[..., 3].astype(np.float64)
    ys, xs = np.mgrid[0 : fan.shape[0], 0 : fan.shape[1]]
    return float((xs * a).sum() / a.sum()), float((ys * a).sum() / a.sum())


def spin_blur(fan: np.ndarray, spread: float = 40.0, taps: int = 15) -> np.ndarray:
    """Rotational motion blur of the fan about its centroid, padded square."""
    cx, cy = fan_centroid(fan)
    size = 96
    canvas = np.zeros((size, size, 4), np.float32)
    ox, oy = size / 2 - cx, size / 2 - cy
    base = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    base.alpha_composite(Image.fromarray(fan), (int(round(ox)), int(round(oy))))
    arr = np.array(base).astype(np.float32)
    a = arr[..., 3:4] / 255.0
    pm = np.concatenate([arr[..., :3] * a, a], axis=-1)
    for k in range(taps):
        ang = -spread / 2 + spread * k / (taps - 1)
        chans = [np.array(Image.fromarray(pm[..., c]).rotate(ang, resample=Image.Resampling.BILINEAR)) for c in range(4)]
        canvas += np.stack(chans, axis=-1)
    canvas /= taps
    alpha = canvas[..., 3]
    rgb = np.divide(canvas[..., :3], np.maximum(alpha[..., None], 1e-6))
    return to_rgba(rgb, alpha * 1.05)


def trail_sprite(w: int = 190, h: int = 26) -> np.ndarray:
    """Air streak behind the fan. The right end is where the fan was."""
    xs = np.linspace(0, 1, w)[None, :]
    ys = np.linspace(-1, 1, h)[:, None]
    taper = 0.25 + 0.75 * xs ** 0.7
    core = np.exp(-((ys / (0.42 * taper)) ** 2))
    along = smoothstep(0.0, 0.85, xs) * (1 - smoothstep(0.93, 1.0, xs))
    noise = 0.85 + 0.15 * periodic_noise(h, w, 1.6, 7)
    alpha = core * along * noise * 0.85
    rgb = ramp_colors(core * 0.6 + xs * 0.4, [(0.0, (226, 150, 60)), (0.7, (246, 200, 120)), (1.0, (255, 240, 200))])
    return to_rgba(rgb, alpha)


def speed_lines(w: int = 120, h: int = 60) -> np.ndarray:
    def draw(d: ImageDraw.ImageDraw, ss: int):
        for y, x0, x1, wd in ((12, 30, 118, 2.0), (28, 8, 104, 2.6), (44, 40, 116, 1.8), (54, 64, 112, 1.4)):
            d.line([(x0 * ss, y * ss), (x1 * ss, y * ss)], fill=(214, 150, 70, 255), width=int(wd * ss))

    img = supersample(draw, w, h)
    fade = smoothstep(0.0, 0.6, np.linspace(0, 1, w))[None, :]
    img[..., 3] = (img[..., 3] * fade).astype(np.uint8)
    return img


def swoosh_sprite(w: int = 150, h: int = 120) -> np.ndarray:
    """Crescent smear that trails the hand on the thrust."""
    yy, xx = np.mgrid[0:h, 0:w].astype(np.float32)
    cx, cy = w * 0.92, h * 1.02
    r = np.hypot(xx - cx, yy - cy)
    ang = np.arctan2(yy - cy, xx - cx)
    band = np.exp(-(((r - h * 0.82) / (h * 0.11)) ** 2))
    # Thickest at the leading (front) end, fading toward where the hand came from.
    t = np.clip((ang - (-np.pi * 0.92)) / (np.pi * 0.42), 0, 1)
    sweep = (1 - t) ** 1.4 * smoothstep(0.0, 0.08, 1 - t) * smoothstep(-0.02, 0.1, t)
    alpha = band * sweep * 0.8
    rgb = ramp_colors(band, [(0.0, (236, 160, 70)), (1.0, (255, 226, 150))])
    return to_rgba(rgb, alpha)


# ---------------------------------------------------------------- explosion


def radial(size: int, stops, alpha_stops, squash: float = 1.0) -> np.ndarray:
    yy, xx = np.mgrid[0:size, 0:size].astype(np.float32)
    c = (size - 1) / 2
    r = np.hypot((xx - c), (yy - c) / squash) / c
    rgb = ramp_colors(np.clip(r, 0, 1), stops)
    alpha = np.interp(np.clip(r, 0, 1.0001), [s[0] for s in alpha_stops], [s[1] for s in alpha_stops])
    return to_rgba(rgb, alpha)


def flash_sprite(size: int = 256) -> np.ndarray:
    return radial(
        size,
        [(0.0, (255, 255, 255)), (0.25, (255, 250, 220)), (0.55, (255, 214, 120)), (1.0, (255, 170, 60))],
        [(0.0, 1.0), (0.2, 0.98), (0.5, 0.6), (0.8, 0.18), (1.0, 0.0)],
    )


def fireball_sprite(stage: int, size: int = 200) -> np.ndarray:
    """Billowing fireball. Stage 0 is white hot, stage 3 is a dark, broken shell."""
    yy, xx = np.mgrid[0:size, 0:size].astype(np.float32)
    c = (size - 1) / 2
    n1 = periodic_noise(size, size, 3.0, 100 + stage)
    n2 = periodic_noise(size, size, 3.2, 200 + stage)
    ang = np.arctan2(yy - c, xx - c)
    r = np.hypot(xx - c, yy - c) / c
    lobes = 0.08 * np.sin(ang * 5 + stage) + 0.05 * np.sin(ang * 9 - stage * 2)
    edge = 0.78 + lobes + 0.16 * n1
    body = 1 - smoothstep(edge - 0.2, edge, r)
    heat = np.clip((1 - r / np.maximum(edge, 0.3)) * 1.25 + 0.25 * n2, 0, 1)
    cool = [0.0, 0.18, 0.3, 0.45][stage]
    heat = np.clip(heat - cool, 0, 1)
    holes = smoothstep(0.2, 0.7, n2 + (stage - 2) * 0.3) if stage >= 2 else 0.0
    alpha = body * (1 - 0.6 * holes)
    stops = [(0.0, (150, 30, 10)), (0.3, (232, 82, 18)), (0.6, (255, 160, 40)), (0.85, (255, 228, 130)), (1.0, (255, 252, 236))]
    if stage >= 2:
        # Burnt rim: soot creeping into the flame.
        stops = [(0.0, (84, 64, 58)), (0.2, (112, 70, 52)), (0.45, (190, 70, 24)), (0.7, (244, 130, 34)), (1.0, (255, 210, 110))]
    return to_rgba(ramp_colors(heat, stops), alpha)


def ring_sprite(size: int = 256) -> np.ndarray:
    yy, xx = np.mgrid[0:size, 0:size].astype(np.float32)
    c = (size - 1) / 2
    r = np.hypot(xx - c, yy - c) / c
    band = np.exp(-(((r - 0.9) / 0.045) ** 2)) + 0.35 * np.exp(-(((r - 0.82) / 0.09) ** 2))
    alpha = np.clip(band, 0, 1) * 0.9
    rgb = ramp_colors(np.clip(band, 0, 1), [(0.0, (246, 150, 60)), (1.0, (255, 214, 130))])
    return to_rgba(rgb, alpha)


def spark_sprite(w: int = 44, h: int = 9) -> np.ndarray:
    xs = np.linspace(0, 1, w)[None, :]
    ys = np.linspace(-1, 1, h)[:, None]
    core = np.exp(-((ys / (0.25 + 0.55 * xs)) ** 2))
    along = xs ** 1.2 * (1 - smoothstep(0.9, 1.0, xs))
    rgb = ramp_colors(core * xs, [(0.0, (255, 120, 30)), (0.6, (255, 210, 90)), (1.0, (255, 252, 230))])
    return to_rgba(rgb, core * along)


def debris_sprite(kind: int) -> np.ndarray:
    """Torn pleat of the fan, cream or gold with a scorched edge."""
    shapes = [
        [(2, 6), (16, 1), (19, 11), (6, 15)],
        [(1, 3), (12, 0), (14, 9), (9, 13), (3, 10)],
        [(3, 1), (18, 4), (15, 12), (1, 9)],
    ]
    cols = [(250, 238, 210, 255), (236, 186, 92, 255), (244, 222, 176, 255)]

    def draw(d: ImageDraw.ImageDraw, ss: int):
        pts = [(x * ss, y * ss) for x, y in shapes[kind]]
        d.polygon(pts, fill=cols[kind])
        d.line(pts + [pts[0]], fill=(120, 70, 40, 255), width=ss)

    return supersample(draw, 20, 16)


def smoke_sprite(kind: int, size: int = 128) -> np.ndarray:
    yy, xx = np.mgrid[0:size, 0:size].astype(np.float32)
    c = (size - 1) / 2
    n = periodic_noise(size, size, 3.2, 300 + kind)
    n2 = periodic_noise(size, size, 3.6, 400 + kind)
    r = np.hypot(xx - c, (yy - c) * 1.08) / c
    edge = 0.72 + 0.2 * n
    alpha = (1 - smoothstep(edge - 0.35, edge, r)) * (0.75 + 0.25 * n2)
    light = np.clip(0.55 + 0.45 * (c - yy) / c * 0.6 + 0.2 * n2, 0, 1)
    rgb = ramp_colors(light, [(0.0, (96, 88, 84)), (0.5, (146, 138, 130)), (1.0, (206, 200, 192))])
    return to_rgba(rgb, alpha * 0.92)


# ---------------------------------------------------------------- fire

FLAME_FRAMES = 6


def flame_flipbook(family: int, w: int = 70, h: int = 170, frames: int = FLAME_FRAMES) -> list[np.ndarray]:
    """One looping flame tongue. Base at the bottom centre of the sprite."""
    period = h * 2
    n_shape = periodic_noise(period, 64, 3.0, 500 + family)
    n_turb = periodic_noise(period, w, 2.6, 600 + family)
    yy, xx = np.mgrid[0:h, 0:w].astype(np.float32)
    v = 1 - yy / (h - 1)  # 0 at the base, 1 at the tip
    u = (xx - (w - 1) / 2) / ((w - 1) / 2)
    out = []
    lean = [0.0, 0.14, -0.12][family % 3]
    for k in range(frames):
        s = int(round(k * period / frames))
        rows = (np.arange(h) + s) % period
        sway = n_shape[rows, 8][:, None] * 0.45 + n_shape[rows, 40][:, None] * 0.2
        uu = u - (sway + lean) * v ** 1.25
        width = 0.9 * (1 - v) ** 0.75 * (0.6 + 0.4 * np.sin(np.pi * np.clip(v * 1.5 + 0.2, 0, 1)))
        shape = 1 - smoothstep(0.45, 1.0, np.abs(uu) / np.maximum(width, 1e-3))
        turb = n_turb[rows]
        eat = np.clip(0.45 + 0.75 * turb, 0, 1) * v ** 1.1
        core = np.clip(1 - np.abs(uu) / np.maximum(width * 0.42, 1e-3), 0, 1) * (1 - v) ** 1.4
        heat = np.clip(shape * (0.78 - 0.45 * v) + 0.5 * core - 0.6 * eat, 0, 1)
        alpha = smoothstep(0.05, 0.28, heat) * smoothstep(0.0, 0.14, v)
        rgb = ramp_colors(
            heat,
            [(0.0, (168, 20, 6)), (0.2, (222, 52, 8)), (0.42, (248, 104, 14)), (0.62, (255, 160, 32)),
             (0.8, (255, 214, 84)), (1.0, (255, 244, 196))],
        )
        out.append(to_rgba(rgb, alpha))
    return out


def glow_sprite(w: int = 360, h: int = 500) -> np.ndarray:
    yy, xx = np.mgrid[0:h, 0:w].astype(np.float32)
    nx = (xx - (w - 1) / 2) / (w / 2)
    ny = (yy - h * 0.52) / (h / 2)
    r = np.sqrt(nx * nx + ny * ny)
    alpha = (1 - smoothstep(0.35, 1.0, r)) ** 1.6 * 0.55
    rgb = ramp_colors(np.clip(1 - r, 0, 1), [(0.0, (255, 120, 30)), (0.6, (255, 170, 60)), (1.0, (255, 220, 120))])
    return to_rgba(rgb, alpha)


def pool_sprite(w: int = 360, h: int = 64) -> np.ndarray:
    yy, xx = np.mgrid[0:h, 0:w].astype(np.float32)
    nx = (xx - (w - 1) / 2) / (w / 2)
    ny = (yy - (h - 1) / 2) / (h / 2)
    r = np.sqrt(nx * nx + ny * ny)
    alpha = (1 - smoothstep(0.2, 1.0, r)) ** 1.4 * 0.7
    rgb = ramp_colors(np.clip(1 - r, 0, 1), [(0.0, (240, 70, 14)), (0.7, (255, 150, 40)), (1.0, (255, 214, 120))])
    return to_rgba(rgb, alpha)


def ember_sprite(size: int = 18) -> np.ndarray:
    return radial(
        size,
        [(0.0, (255, 252, 230)), (0.35, (255, 214, 90)), (1.0, (255, 110, 20))],
        [(0.0, 1.0), (0.35, 0.95), (0.7, 0.35), (1.0, 0.0)],
    )


def build_all() -> dict[str, np.ndarray]:
    fx: dict[str, np.ndarray] = {}
    fan = fan_sprite()
    fx["fan"] = fan
    fx["fan_spin"] = spin_blur(fan)
    fx["fan_trail"] = trail_sprite()
    fx["swoosh"] = swoosh_sprite()
    fx["boom_flash"] = flash_sprite()
    for s in range(4):
        fx[f"boom_fire_{s}"] = fireball_sprite(s)
    fx["boom_ring"] = ring_sprite()
    fx["spark"] = spark_sprite()
    for k in range(3):
        fx[f"debris_{k}"] = debris_sprite(k)
        fx[f"smoke_{k}"] = smoke_sprite(k)
    for fam in range(3):
        for k, frame in enumerate(flame_flipbook(fam)):
            fx[f"flame_{fam}_{k}"] = frame
    fx["fire_glow"] = glow_sprite()
    fx["fire_pool"] = pool_sprite()
    fx["ember"] = ember_sprite()
    return fx
