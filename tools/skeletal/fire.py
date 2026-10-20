"""Fire bones and slots for the power-up. Everything here draws behind the character."""

from __future__ import annotations

import math

import cv2
import numpy as np

import fx
from parts import H, W

FLAME_H = 170


def silhouette(b) -> np.ndarray:
    mask = np.zeros((H, W), np.uint8)
    for p in b.parts:
        if p.name != "shadow":
            mask[p.owned] = 255
    return mask


def contour_anchors(mask: np.ndarray, spacing: float = 26.0):
    contours, _ = cv2.findContours(mask, cv2.RETR_EXTERNAL, cv2.CHAIN_APPROX_NONE)
    c = max(contours, key=cv2.contourArea)[:, 0, :].astype(np.float64)
    # Smooth so normals follow the body, not pixel steps.
    k = 9
    pad = np.concatenate([c[-k:], c, c[:k]])
    kernel = np.ones(2 * k + 1) / (2 * k + 1)
    sm = np.stack([np.convolve(pad[:, i], kernel, mode="same")[k:-k] for i in range(2)], axis=1)
    seg = np.hypot(*np.diff(np.vstack([sm, sm[:1]]), axis=0).T)
    arc = np.concatenate([[0], np.cumsum(seg)])[:-1]
    total = arc[-1] + seg[-1]
    out = []
    cx, cy = sm[:, 0].mean(), sm[:, 1].mean()
    for s in np.arange(0, total, spacing):
        i = int(np.searchsorted(arc, s))
        i = min(i, len(sm) - 1)
        p0, p1 = sm[(i - 3) % len(sm)], sm[(i + 3) % len(sm)]
        tx, ty = p1 - p0
        n = np.array([ty, -tx])
        n /= np.linalg.norm(n) + 1e-9
        if np.dot(n, sm[i] - [cx, cy]) < 0:
            n = -n
        out.append((float(sm[i][0]), float(sm[i][1]), float(n[0]), float(n[1])))
    return out


def aura_frames(mask: np.ndarray, frames: int = fx.FLAME_FRAMES):
    """A flickering band of flame hugging the silhouette, taller above it."""
    inside = mask > 0
    best = np.full((H, W), 1e9, np.float32)
    for shift in (0, 6, 12, 18, 24):
        shifted = np.zeros_like(inside)
        shifted[: H - shift] = inside[shift:]
        d = cv2.distanceTransform((~shifted).astype(np.uint8), cv2.DIST_L2, 5)
        best = np.minimum(best, d + shift * 0.55)
    period = 96
    noise = fx.periodic_noise(period, W, 2.8, 900)
    noise2 = fx.periodic_noise(period, W, 2.2, 901)
    rows = np.arange(H)
    out = []
    for k in range(frames):
        s = int(round(k * period / frames))
        n = noise[(rows + s) % period]
        n2 = noise2[(rows + 2 * s) % period]
        reach = 9.0 + 9.0 * (0.5 + 0.5 * n) + 5.0 * np.clip(n2, 0, 1)
        t = np.clip(best / reach, 0, 1)
        alpha = (1 - fx.smoothstep(0.35, 1.0, t)) * 0.95
        heat = np.clip(1.05 - t * 1.1 + 0.15 * n2, 0, 1)
        rgb = fx.ramp_colors(heat, [(0.0, (214, 46, 8)), (0.4, (248, 110, 16)), (0.75, (255, 182, 46)), (1.0, (255, 230, 120))])
        out.append(fx.to_rgba(rgb, alpha))
    ys, xs = np.where(np.max([f[:, :, 3] for f in out], axis=0) > 0)
    box = (int(xs.min()), int(ys.min()), int(xs.max()) + 1, int(ys.max()) + 1)
    return [f[box[1] : box[3], box[0] : box[2]] for f in out], box


def layout_fire(b):
    rng = np.random.default_rng(7)
    mask = silhouette(b)
    rig = b.rig
    info = {"flames": [], "big": [], "licks": [], "embers": []}

    def flame_slot(slot: str, bone: str, family: int, alpha_hint: float = 1.0):
        b.slot(slot, bone)
        for k in range(fx.FLAME_FRAMES):
            region = f"flame_{family}_{k}"
            r = b.regions[region]
            b.attach(slot, f"f{k}", region, 0.0, r.h / 2 - 12)

    rig.add("fire_glow", "fire", (200.0, 300.0))
    b.slot("fire_glow", "fire_glow")
    b.attach("fire_glow", "fire_glow", "fire_glow", 0, 0)

    # Big, slow tongues behind the head and shoulders give the fire depth.
    big = [(205, 205, 0.0), (258, 250, -8.0), (160, 250, 9.0), (290, 330, -14.0), (120, 330, 15.0), (215, 300, 2.0)]
    for i, (x, y, rot) in enumerate(big):
        name = f"flame_big_{i}"
        rig.add(name, "fire", (float(x), float(y)))
        flame_slot(name, name, i % 3)
        info["big"].append({"bone": name, "slot": name, "rot": rot, "scale": 1.3 + 0.22 * ((i * 7) % 3) / 2, "offset": int(rng.integers(0, 6)), "phase": float(rng.random())})

    anchors = contour_anchors(mask)
    n = 0
    for x, y, nx, ny in anchors:
        if ny > 0.45 and y > 380:
            continue  # belly underside and soles: no fire pointing into the floor
        if y > 528:
            continue
        # Base sits inside the silhouette so the tongue emerges from behind the body.
        bx, by = x - nx * 9, y - ny * 9
        name = f"flame_{n}"
        rig.add(name, "fire", (bx, by))
        fam = n % 3
        flame_slot(name, name, fam)
        height = np.interp(y, [90, 200, 330, 450, 530], [1.05, 0.95, 0.8, 0.6, 0.5])
        lean = float(np.clip(-nx * 28.0, -30, 30))
        info["flames"].append({
            "bone": name, "slot": name, "rot": lean, "scale": float(height * (0.88 + 0.24 * rng.random())),
            "width": float(0.95 + 0.25 * rng.random()), "offset": int(rng.integers(0, 6)), "phase": float(rng.random()),
            "x": bx, "y": by,
        })
        n += 1

    frames, box = aura_frames(mask)
    b.slot("fire_aura", "fire")
    for k, f in enumerate(frames):
        b.region(f"fire_aura_{k}", f)
        b.place_canvas("fire_aura", f"a{k}", f"fire_aura_{k}", "fire", box)

    # Detached licks that peel off the top of the fire and vanish.
    top = sorted(info["flames"], key=lambda f: f["y"])[:8]
    for i, f in enumerate(top):
        name = f"lick_{i}"
        rig.add(name, "fire", (f["x"] + rng.uniform(-6, 6), f["y"] - 70 * f["scale"]))
        flame_slot(name, name, (i + 1) % 3)
        info["licks"].append({"bone": name, "slot": name, "rot": f["rot"], "phase": i / len(top), "rise": float(rng.uniform(45, 70))})

    # Embers start just outside the silhouette and drift up.
    pts = [a for a in anchors if a[1] < 470 and a[3] < 0.3]
    for i in range(26):
        x, y, nx, ny = pts[int(rng.integers(0, len(pts)))]
        sx, sy = x + nx * rng.uniform(8, 26), y + ny * rng.uniform(8, 26)
        name = f"ember_{i}"
        rig.add(name, "fire", (float(sx), float(sy)))
        b.slot(name, name)
        b.attach(name, "ember", "ember", 0, 0)
        info["embers"].append({
            "bone": name, "slot": name, "phase": float(rng.random()), "life": float(rng.uniform(0.55, 0.9)),
            "rise": float(rng.uniform(70, 150)), "sway": float(rng.uniform(5, 13)), "drift": float(np.clip(nx * 20, -14, 14)),
            "size": float(rng.uniform(0.6, 1.15)), "wob": float(rng.random() * math.tau),
        })

    rig.add("fire_pool", "fire", (192.0, 528.0))
    b.slot("fire_pool", "fire_pool")
    b.attach("fire_pool", "fire_pool", "fire_pool", 0, 0)
    b.extra["fire"] = info
