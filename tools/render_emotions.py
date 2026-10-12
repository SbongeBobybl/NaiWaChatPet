#!/usr/bin/env python3
"""Build new emotion plates from the attached NaiWa reference photo.

Frame 0 of every new clip is the canonical plate: that photo, uniformly
scaled onto the 420x560 animation canvas. Later frames keep those pixels.
The body squashes, leans, and nods, and the eye, mouth, blush, and tears
are painted with colors taken from the photo. Nothing here is a new shaded
character, and nothing is a crop of the legacy plates.

Legacy directories are never written.
"""

from __future__ import annotations

import hashlib
import json
import math
import shutil
import sys
from pathlib import Path

import numpy as np
from PIL import Image, ImageDraw, ImageFilter

ROOT = Path(__file__).resolve().parents[1]
ANIM = ROOT / "animations"
CATALOG_PATH = ROOT / "source" / "emotion" / "emotions.json"
CONTRACT_PATH = ROOT / "source" / "emotion" / "CONTRACT.md"
REF_JPG = ROOT / "source" / "emotion" / "reference-neutral.jpg"
CANON_PNG = ROOT / "source" / "emotion" / "neutral-reference.png"

W, H = 420, 560
GROUND = 540
SAFE_BUDGET = 125 * 1024 * 1024
N_IN, N_MAIN, N_OUT = 3, 6, 3
FPS = 12
CROP = (4, 2, 818, 1192)


def build_canonical():
    src = Image.open(REF_JPG).convert("RGB").crop(CROP)
    arr = np.asarray(src)
    white = (arr[:, :, 0] > 250) & (arr[:, :, 1] > 250) & (arr[:, :, 2] > 250)
    ys, xs = np.where(~white)
    pad = 8
    x0 = max(0, int(xs.min()) - pad)
    y0 = max(0, int(ys.min()) - pad)
    x1 = min(arr.shape[1], int(xs.max()) + pad + 1)
    y1 = min(arr.shape[0], int(ys.max()) + pad + 1)
    cropped = src.crop((x0, y0, x1, y1))
    target_h = 512
    scale = target_h / cropped.height
    target_w = max(1, int(round(cropped.width * scale)))
    if target_w > 400:
        target_w = 400
        scale = target_w / cropped.width
        target_h = max(1, int(round(cropped.height * scale)))
    resized = cropped.resize((target_w, target_h), Image.Resampling.LANCZOS)
    canvas = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    ox = (W - target_w) // 2
    oy = GROUND - target_h
    canvas.paste(resized, (ox, oy))
    rgba = np.array(canvas).astype(np.float32)
    rgb = rgba[:, :, :3]
    dist = 255.0 - rgb.min(axis=2)
    backdrop = (rgba[:, :, 3] > 0) & (dist < 14.0)
    rgba[:, :, 3] = np.where(backdrop, 0.0, rgba[:, :, 3])
    soft = (rgba[:, :, 3] > 0) & (dist < 28.0)
    rgba[:, :, 3] = np.where(soft, np.clip((dist - 14.0) * 18.0, 0.0, 255.0), rgba[:, :, 3])
    out = Image.fromarray(rgba.astype(np.uint8), "RGBA")
    CANON_PNG.parent.mkdir(parents=True, exist_ok=True)
    out.save(CANON_PNG, format="PNG", optimize=True, compress_level=9)
    return out


def find_anchors(rgba):
    rgb = rgba[:, :, :3].astype(np.int16)
    a = rgba[:, :, 3] > 40
    r, g, b = rgb[:, :, 0], rgb[:, :, 1], rgb[:, :, 2]
    h, w = a.shape
    yy, xx = np.mgrid[0:h, 0:w]
    ys, xs = np.where(a)
    feet = int(ys.max())
    top = int(ys.min())
    green = a & (g > r + 12) & (g > 90) & (b > 40) & (g > b - 10) & (yy < top + 180)
    gys, gxs = np.where(green)
    if len(gxs) < 20:
        raise SystemExit("reference eye was not found")
    eye = (float(gxs.mean()), float(gys.mean()))
    # The slit is the dark horizontal mark on the snout, below and in front of the eye.
    slit = (
        a
        & (r < 90)
        & (g < 60)
        & (b < 50)
        & (yy > eye[1] + 16)
        & (yy < eye[1] + 36)
        & (xx > eye[0] - 48)
        & (xx < eye[0] + 8)
    )
    sys_, sxs = np.where(slit)
    if len(sxs) < 8:
        mouth = (eye[0] - 22.0, eye[1] + 24.0)
    else:
        mouth = (float(sxs.mean()), float(sys_.mean()))
    body_cx = float(np.median(xs))
    return {
        "eye": eye,
        "mouth": mouth,
        "feet": float(feet),
        "top": float(top),
        "body_cx": body_cx,
        "neck": float(eye[1] + 78),
        "head": (body_cx, float(eye[1] + 40)),
    }


def dilate(mask, rad):
    out = mask.copy()
    ys, xs = np.where(mask)
    h, w = mask.shape
    for dy in range(-rad, rad + 1):
        for dx in range(-rad, rad + 1):
            if dx * dx + dy * dy > rad * rad:
                continue
            out[np.clip(ys + dy, 0, h - 1), np.clip(xs + dx, 0, w - 1)] = True
    return out


def hand_hole(rgba):
    rgb = rgba[:, :, :3].astype(np.float32)
    s = rgb.sum(2)
    a = rgba[:, :, 3]
    yy, xx = np.mgrid[0:H, 0:W]
    ref = np.full(H, 700.0)
    for y in range(220, 450):
        vals = s[y, 215:305][a[y, 215:305] > 200]
        if len(vals):
            ref[y] = float(np.median(vals))
    roi = (xx >= 90) & (xx <= 200) & (yy >= 258) & (yy <= 338) & (a > 150)
    roi &= ~((xx > 182) & (yy > 314))
    core = (
        roi
        & (s < ref[yy] - 70)
        & (rgb[:, :, 2] + 6 < rgb[:, :, 1])
        & (rgb[:, :, 1] < rgb[:, :, 0] + 14)
        & (rgb[:, :, 0] < 220)
    )
    return dilate(core, 3) & (a > 40), core


def fill_belly(rgba, hole):
    """Continue each belly row across the hole so the shading stays in the photo."""
    body = rgba.copy()
    for y in range(H):
        xs = np.where(hole[y])[0]
        if len(xs) == 0:
            continue
        runs = []
        start = prev = int(xs[0])
        for x in map(int, xs[1:]):
            if x > prev + 1:
                runs.append((start, prev))
                start = x
            prev = x
        runs.append((start, prev))
        for a, b in runs:
            left = a - 1
            while left > 0 and (hole[y, left] or rgba[y, left, 3] < 80):
                left -= 1
            right = b + 1
            while right < W - 1 and (hole[y, right] or rgba[y, right, 3] < 80):
                right += 1
            if rgba[y, left, 3] < 80 or rgba[y, right, 3] < 80 or right <= left:
                continue
            c0 = rgba[y, left, :3].astype(np.float32)
            c1 = rgba[y, right, :3].astype(np.float32)
            span = max(right - left, 1)
            ts = np.arange(a, b + 1)
            t = ((ts - left) / span)[:, None]
            body[y, ts, :3] = np.clip(c0 * (1 - t) + c1 * t, 0, 255)
            body[y, ts, 3] = 255
    blur = np.array(Image.fromarray(body).filter(ImageFilter.GaussianBlur(0.7)))
    body[hole] = blur[hole]
    return body


def sprite_from_mask(rgba, mask):
    ys, xs = np.where(mask)
    if len(xs) < 30:
        raise SystemExit("hand sprite is empty")
    xa, ya = int(xs.min()), int(ys.min())
    xb, yb = int(xs.max()) + 1, int(ys.max()) + 1
    sp = rgba[ya:yb, xa:xb].copy()
    sm = mask[ya:yb, xa:xb]
    af = np.array(Image.fromarray((sm.astype(np.uint8) * 255)).filter(ImageFilter.GaussianBlur(1.3)))
    sp[:, :, 3] = np.where(af < 24, 0, np.minimum(sp[:, :, 3], af))
    return sp, float(xs.mean()), float(ys.mean())


def components(mask):
    from collections import deque

    vis = np.zeros(mask.shape, np.int32)
    found = []
    cid = 0
    ys, xs = np.where(mask)
    for y, x in zip(ys.tolist(), xs.tolist()):
        if vis[y, x]:
            continue
        cid += 1
        q = deque([(y, x)])
        vis[y, x] = cid
        n = 0
        while q:
            cy, cx = q.popleft()
            n += 1
            for dy, dx in ((1, 0), (-1, 0), (0, 1), (0, -1)):
                ny, nx = cy + dy, cx + dx
                if 0 <= ny < H and 0 <= nx < W and mask[ny, nx] and vis[ny, nx] == 0:
                    vis[ny, nx] = cid
                    q.append((ny, nx))
        if n > 40:
            found.append((n, cid))
    found.sort(reverse=True)
    return vis, found


def build_parts(canon_u8):
    anchors = find_anchors(canon_u8)
    hole, core = hand_hole(canon_u8)
    body = fill_belly(canon_u8, hole)
    # Dark mitten pixels only, so a moved hand does not carry a belly rectangle.
    mask = core & (canon_u8[:, :, 3] > 80)
    vis, found = components(mask)
    if len(found) < 2:
        raise SystemExit(f"expected two hands, found {len(found)}")
    sprites = []
    for _n, cid in found[:2]:
        sprites.append(sprite_from_mask(canon_u8, vis == cid))
    # Larger component is the mitten on the belly. Smaller is the side hand.
    anchors["hands"] = [(sprites[0][1], sprites[0][2]), (sprites[1][1], sprites[1][2])]
    eye = anchors["eye"]
    patch = canon_u8[int(eye[1]) - 3 : int(eye[1]) + 4, int(eye[0]) + 16 : int(eye[0]) + 26, :3]
    anchors["skin"] = np.median(patch.reshape(-1, 3), axis=0).astype(np.float32)
    slit = canon_u8[int(anchors["mouth"][1]) - 1 : int(anchors["mouth"][1]) + 2, int(anchors["mouth"][0]) - 4 : int(anchors["mouth"][0]) + 5, :3]
    dark = slit.reshape(-1, 3)
    anchors["slit"] = np.median(dark, axis=0).astype(np.float32)
    return {"canon": canon_u8, "body": body, "sprites": sprites, "anchors": anchors}


def sample_rgba(img, sx, sy):
    h, w = img.shape[:2]
    sx = np.clip(sx, 0, w - 1.001)
    sy = np.clip(sy, 0, h - 1.001)
    x0 = np.floor(sx).astype(np.int32)
    y0 = np.floor(sy).astype(np.int32)
    x1 = np.clip(x0 + 1, 0, w - 1)
    y1 = np.clip(y0 + 1, 0, h - 1)
    fx = (sx - x0)[..., None]
    fy = (sy - y0)[..., None]
    ia = img[y0, x0].astype(np.float32)
    ib = img[y0, x1].astype(np.float32)
    ic = img[y1, x0].astype(np.float32)
    id_ = img[y1, x1].astype(np.float32)
    return ia * (1 - fx) * (1 - fy) + ib * fx * (1 - fy) + ic * (1 - fx) * fy + id_ * fx * fy


def smooth_weight(y, neck):
    band = 26.0
    t = np.clip((neck + band - y) / band, 0.0, 1.0)
    return t * t * (3.0 - 2.0 * t)


def inverse_xy(xx, yy, p, anchors):
    feet = anchors["feet"]
    cx = anchors["body_cx"]
    squash = max(float(p["squash"]), 0.45)
    widen = max(float(p["widen"]), 0.45)
    sx = xx - float(p["shift"])
    sy = yy
    lean = math.radians(float(p["lean"]))
    ca, sa = math.cos(-lean), math.sin(-lean)
    dx, dy = sx - cx, sy - feet
    sx = ca * dx - sa * dy + cx
    sy = sa * dx + ca * dy + feet
    sx = cx + (sx - cx) / widen
    sy = feet - (feet - sy) / squash
    neck = anchors["neck"]
    hx, hy = anchors["head"]
    w = smooth_weight(sy, neck)
    sy = sy - float(p["head_dy"]) * w
    tilt = math.radians(-float(p["tilt"]))
    if abs(float(p["tilt"])) > 0.05:
        ca, sa = math.cos(tilt), math.sin(tilt)
        dx, dy = sx - hx, sy - hy
        rx = ca * dx - sa * dy + hx
        ry = sa * dx + ca * dy + hy
        sx = sx * (1 - w) + rx * w
        sy = sy * (1 - w) + ry * w
    return sx, sy


def forward_point(x, y, p, anchors):
    feet = anchors["feet"]
    cx = anchors["body_cx"]
    hx, hy = anchors["head"]
    neck = anchors["neck"]
    w = float(smooth_weight(np.array([y]), neck)[0])
    y = y + float(p["head_dy"]) * w
    tilt = math.radians(float(p["tilt"]))
    if abs(tilt) > 1e-4:
        ca, sa = math.cos(tilt), math.sin(tilt)
        dx, dy = x - hx, y - hy
        rx = ca * dx - sa * dy + hx
        ry = sa * dx + ca * dy + hy
        x = x * (1 - w) + rx * w
        y = y * (1 - w) + ry * w
    squash = float(p["squash"])
    widen = float(p["widen"])
    y = feet - (feet - y) * squash
    x = cx + (x - cx) * widen
    lean = math.radians(float(p["lean"]))
    ca, sa = math.cos(lean), math.sin(lean)
    dx, dy = x - cx, y - feet
    x = ca * dx - sa * dy + cx
    y = sa * dx + ca * dy + feet
    x += float(p["shift"])
    return x, y


def blend_disc(img, cx, cy, rx, ry, color, amount, y_gate=None):
    if amount <= 0.01:
        return
    x0 = max(0, int(cx - rx * 3))
    x1 = min(W, int(cx + rx * 3) + 1)
    y0 = max(0, int(cy - ry * 3))
    y1 = min(H, int(cy + ry * 3) + 1)
    if x1 <= x0 or y1 <= y0:
        return
    yy, xx = np.mgrid[y0:y1, x0:x1]
    fall = np.exp(-((xx - cx) ** 2) / (2 * rx * rx) - ((yy - cy) ** 2) / (2 * ry * ry))
    if y_gate is not None:
        fall *= y_gate(yy)
    a = img[y0:y1, x0:x1, 3:4].astype(np.float32) / 255.0
    cover = (fall * amount * (a[:, :, 0] > 0.45))[..., None]
    rgb = img[y0:y1, x0:x1, :3].astype(np.float32)
    img[y0:y1, x0:x1, :3] = np.clip(rgb * (1 - cover) + color * cover, 0, 255).astype(np.uint8)


def cover_eye(out, cx, cy, radius, skin, lid_frac):
    """Paint skin down over the eye. lid_frac 0 leaves it, 1 shuts it."""
    if lid_frac <= 0.02:
        return
    rad = int(math.ceil(radius)) + 2
    x0, x1 = max(0, int(cx) - rad), min(W, int(cx) + rad + 1)
    y0, y1 = max(0, int(cy) - rad), min(H, int(cy) + rad + 1)
    yy, xx = np.mgrid[y0:y1, x0:x1]
    r = np.hypot(xx - cx, yy - cy)
    lid_y = cy - radius + lid_frac * (radius * 2.15)
    inside = (r < radius + 1.2) & (out[y0:y1, x0:x1, 3] > 160)
    soft = np.clip((lid_y - yy) / 2.2, 0.0, 1.0) * np.clip((radius + 1.2 - r) / 2.0, 0.0, 1.0)
    cover = (inside * soft)[..., None]
    rgb = out[y0:y1, x0:x1, :3].astype(np.float32)
    out[y0:y1, x0:x1, :3] = np.clip(rgb * (1 - cover) + skin * cover, 0, 255).astype(np.uint8)


def paint_face(img, anchors, p):
    out = img.copy()
    ex, ey = anchors["eye"]
    mx, my = anchors["mouth"]
    skin = anchors["skin"]
    slit = np.clip(anchors["slit"], 0, 255)
    yy, xx = np.mgrid[0:H, 0:W]
    eye_dx = float(p["eye_dx"])
    eye_dy = float(p["eye_dy"])
    eye_scale = float(p["eye_scale"])
    lid = float(p["lid"])
    big = abs(eye_scale - 1.0) > 0.12 or abs(eye_dx) > 2.0 or abs(eye_dy) > 2.0
    if big:
        r = np.hypot(xx - ex, yy - ey)
        cover = np.clip((16.0 - r) / 2.4, 0, 1) * (out[:, :, 3] > 180)
        rgb = out[:, :, :3].astype(np.float32)
        out[:, :, :3] = np.clip(rgb * (1 - cover[..., None]) + skin * cover[..., None], 0, 255).astype(np.uint8)
        if lid < 0.9:
            rad = 16
            x0, y0 = int(ex) - rad, int(ey) - rad
            disc = img[y0 : y0 + rad * 2 + 1, x0 : x0 + rad * 2 + 1].copy()
            local = np.hypot(np.arange(-rad, rad + 1)[None, :], np.arange(-rad, rad + 1)[:, None])
            edge = np.clip((rad + 0.6 - local) * 90.0, 0, 255)
            disc[:, :, 3] = np.minimum(disc[:, :, 3], edge.astype(np.uint8))
            nw = max(3, int(round(disc.shape[1] * eye_scale)))
            nh = max(3, int(round(disc.shape[0] * eye_scale)))
            scaled = np.array(Image.fromarray(disc).resize((nw, nh), Image.Resampling.BICUBIC))
            paste_rgba(out, scaled, ex + eye_dx, ey + eye_dy)
        cover_eye(out, ex + eye_dx, ey + eye_dy, 15 * max(eye_scale, 0.5), skin, lid)
    elif lid > 0.02:
        cover_eye(out, ex, ey, 15.5, skin, lid)
    if lid > 0.82:
        blend_disc(out, ex + eye_dx, ey + eye_dy + 1.5, 8.5, 1.15, slit * 0.4, 0.95)

    smile = float(p["smile"])
    open_m = float(p["mouth_open"])
    if abs(smile) > 0.04 or open_m > 0.04:
        blend_disc(out, mx, my, 9.5, 2.4, skin, 1.0)
        width = 16 + open_m * 4
        # Positive smile lifts the corners. Image y grows downward.
        arch = smile * 4.2
        ink = np.minimum(slit * 0.75, 70)
        for t in np.linspace(-1, 1, 9):
            x = mx + t * width * 0.5
            y = my + arch * (1 - t * t)
            blend_disc(out, x, y, 1.35, 1.15, ink, 1.0)
        if open_m > 0.15:
            blend_disc(out, mx, my + 2.2, 3.2 + open_m * 2.0, 1.4 + open_m * 3.4, ink * 0.55, min(1.0, open_m))
    blush = float(p["blush"])
    if blush > 0.02:
        cheek = np.exp(-((xx - (ex - 2)) ** 2) / (2 * 24 * 24) - ((yy - (ey + 26)) ** 2) / (2 * 16 * 16))
        cheek += 0.65 * np.exp(-((xx - (ex + 14)) ** 2) / (2 * 16 * 16) - ((yy - (ey + 18)) ** 2) / (2 * 12 * 12))
        yellow = (out[:, :, 0] > 150) & (out[:, :, 1] > 80) & (out[:, :, 2] + 15 < out[:, :, 0]) & (out[:, :, 3] > 180) & (yy < ey + 70) & (yy > ey - 10)
        cover = (np.clip(cheek, 0, 1) * blush * yellow)[..., None]
        pink = np.array([255.0, 150.0, 150.0], np.float32)
        rgb = out[:, :, :3].astype(np.float32)
        out[:, :, :3] = np.clip(rgb * (1 - 0.55 * cover) + pink * (0.55 * cover), 0, 255).astype(np.uint8)
    tear = float(p["tear"])
    if tear > 0.05:
        drop = np.array([210.0, 228.0, 245.0], np.float32)
        blend_disc(out, ex + 6, ey + 22, 3.2, 6.5, drop, tear)
        if tear > 0.6:
            blend_disc(out, ex + 2, ey + 36, 2.6, 5.5, drop, tear * 0.85)
    brow = float(p["brow"])
    if brow > 0.05:
        blend_disc(out, ex + 2, ey - 16, 10, 1.6, slit * 0.45 + skin * 0.2, brow * 0.7)
    return out


def paste_rgba(dst, sprite, cx, cy):
    h, w = sprite.shape[:2]
    x = int(round(cx - w / 2.0))
    y = int(round(cy - h / 2.0))
    x0, y0 = max(0, x), max(0, y)
    x1, y1 = min(W, x + w), min(H, y + h)
    if x1 <= x0 or y1 <= y0:
        return
    src = sprite[y0 - y : y0 - y + (y1 - y0), x0 - x : x0 - x + (x1 - x0)].astype(np.float32)
    d = dst[y0:y1, x0:x1].astype(np.float32)
    sa = src[:, :, 3:4] / 255.0
    da = d[:, :, 3:4] / 255.0
    oa = sa + da * (1 - sa)
    d[:, :, :3] = (src[:, :, :3] * sa + d[:, :, :3] * da * (1 - sa)) / np.maximum(oa, 1e-4)
    d[:, :, 3:4] = oa * 255.0
    dst[y0:y1, x0:x1] = np.clip(d, 0, 255).astype(np.uint8)


def transform_sprite(sprite, scale, rot):
    im = Image.fromarray(sprite)
    if abs(rot) > 0.2:
        im = im.rotate(rot, resample=Image.Resampling.BICUBIC, expand=True)
    if abs(scale - 1.0) > 0.02:
        nw = max(1, int(round(im.size[0] * scale)))
        nh = max(1, int(round(im.size[1] * scale)))
        im = im.resize((nw, nh), Image.Resampling.BICUBIC)
    return np.array(im)


def needs_hands(p):
    # Lifting the mittens out of this photo leaves a flat patch in the belly
    # shading, so the pose stays on the original pixels. Expression, squash,
    # lean, and nod carry the emotion.
    return False


def render_pose(parts, p):
    anchors = parts["anchors"]
    base = parts["body"] if needs_hands(p) else parts["canon"]
    painted = paint_face(base, anchors, p)
    yy, xx = np.mgrid[0:H, 0:W].astype(np.float32)
    sx, sy = inverse_xy(xx, yy, p, anchors)
    warped = np.clip(sample_rgba(painted, sx, sy), 0, 255).astype(np.uint8)
    if needs_hands(p):
        for sprite, (dx_k, dy_k, rot_k, sc_k), (rx, ry) in (
            (parts["sprites"][0][0], ("h0_dx", "h0_dy", "h0_rot", "h0_sc"), anchors["hands"][0]),
            (parts["sprites"][1][0], ("h1_dx", "h1_dy", "h1_rot", "h1_sc"), anchors["hands"][1]),
        ):
            fx, fy = forward_point(rx, ry, p, anchors)
            fx += float(p[dx_k])
            fy += float(p[dy_k])
            moved = abs(p[dx_k]) + abs(p[dy_k]) > 16
            stamp = transform_sprite(sprite, float(p[sc_k]), float(p[rot_k]))
            if moved:
                shadow = stamp.copy()
                shadow[:, :, :3] = 80
                shadow[:, :, 3] = (shadow[:, :, 3].astype(np.float32) * 0.28).astype(np.uint8)
                paste_rgba(warped, shadow, fx + 2, fy + 5)
            paste_rgba(warped, stamp, fx, fy)
    return warped


def base_pose():
    return {
        "squash": 1.0,
        "widen": 1.0,
        "lean": 0.0,
        "shift": 0.0,
        "head_dy": 0.0,
        "tilt": 0.0,
        "h0_dx": 0.0,
        "h0_dy": 0.0,
        "h0_rot": 0.0,
        "h0_sc": 1.0,
        "h1_dx": 0.0,
        "h1_dy": 0.0,
        "h1_rot": 0.0,
        "h1_sc": 1.0,
        "eye_scale": 1.0,
        "eye_dx": 0.0,
        "eye_dy": 0.0,
        "smile": 0.0,
        "mouth_open": 0.0,
        "lid": 0.0,
        "blush": 0.0,
        "tear": 0.0,
        "brow": 0.0,
    }


def pose(**kw):
    p = base_pose()
    p.update(kw)
    return p


def scale_pose(peak, t):
    out = {}
    origin = base_pose()
    for k, v in peak.items():
        out[k] = origin[k] + (v - origin[k]) * t
    return out


PEAKS = {
    "happy": (
        pose(squash=0.97, smile=1.0, lid=0.58, blush=0.75),
        "Squint, smile, and blush on the reference face, with a small bounce.",
    ),
    "proud": (
        pose(squash=1.07, widen=1.06, head_dy=-14, lid=0.42, smile=0.5, eye_dy=2),
        "Taller chest, chin up, lidded smirk on the reference face.",
    ),
    "sleepy": (
        pose(squash=0.9, head_dy=14, lean=5, lid=0.96, mouth_open=0.85, smile=-0.05, eye_scale=0.7),
        "Slouch, eye shut to a crease, mouth open in a yawn.",
    ),
    "love": (
        pose(widen=1.08, blush=1.0, smile=0.95, lid=0.12, eye_scale=1.2),
        "Heavy blush, soft smile, eye a little wider.",
    ),
    "scared": (
        pose(squash=0.86, widen=1.05, eye_scale=1.5, mouth_open=1.0, smile=-0.3, h0_dx=12, h0_dy=-72, h0_rot=-8, head_dy=4),
        "Crouch, eye widened, mouth open.",
    ),
    "excited": (
        pose(squash=1.1, widen=1.04, eye_scale=1.4, smile=0.85, mouth_open=0.7, blush=0.45),
        "Stretched tall, wide eye, open grin.",
    ),
    "bored": (
        pose(tilt=-11, lid=0.58, smile=-0.35, eye_dx=-3, head_dy=8, h0_dx=28, h0_dy=-92, h0_rot=22),
        "Head tilt, half-shut eye, flat mouth.",
    ),
    "embarrassed": (
        pose(squash=0.88, blush=1.0, lid=0.48, eye_dx=-4, smile=0.4, h0_dx=6, h0_dy=-112, shift=-6),
        "Shrink, strong blush, glance away, small smile.",
    ),
    "curious": (
        pose(tilt=14, squash=1.05, lean=5, eye_scale=1.4, eye_dy=-3, mouth_open=0.55, head_dy=-8),
        "Head cocked the other way, eye wide, small open mouth.",
    ),
    "relieved": (
        pose(squash=0.95, lid=0.92, smile=0.75, blush=0.2, head_dy=6, lean=-3),
        "Exhale squash, eye closed, soft smile.",
    ),
    "disappointed": (
        pose(squash=0.86, head_dy=18, smile=-1.0, lid=0.28, eye_dy=4, eye_scale=0.88),
        "Body and head sink, eye down, mouth pulled into a frown.",
    ),
    "grateful": (
        pose(squash=0.9, head_dy=20, smile=0.85, lid=0.82, blush=0.55),
        "Small bow, eyes closed, smile, blush.",
    ),
    "worried": (
        pose(squash=0.95, eye_scale=1.28, smile=-0.7, eye_dx=4, brow=0.8, blush=0.12, h0_dx=-8, h0_dy=-10, h1_dx=6, h1_dy=-6),
        "Eye wide and glancing, brow pinched, frown.",
    ),
    "smug": (
        pose(lean=-9, squash=1.04, lid=0.58, smile=0.6, eye_dx=5, head_dy=-8, widen=1.05),
        "Lean back, lidded eye, one-sided smile, chest out.",
    ),
    "crying": (
        pose(squash=0.86, lean=8, lid=0.88, smile=-1.0, mouth_open=0.7, tear=1.0, head_dy=14, eye_scale=0.75),
        "Eye squeezed, frown open, tears on the cheek under the eye.",
    ),
    "wronged": (
        pose(lean=-9, shift=8, smile=-0.55, mouth_open=0.15, tear=0.55, eye_dx=-5, lid=0.12, head_dy=4),
        "Lean aside, side glance, pout, a single tear.",
    ),
    "cheer": (
        pose(squash=1.06, h0_dx=18, h0_dy=-96, h0_rot=-12, smile=0.6, mouth_open=1.0, lid=0.28, blush=0.3),
        "Stretched up, eye open, mouth opened wide.",
    ),
    "dizzy": (
        pose(lean=10, tilt=-14, eye_dx=6, eye_dy=-4, smile=-0.15, mouth_open=0.3),
        "Whole photo rocked sideways, eye slid off center.",
    ),
    "focused": (
        pose(squash=1.02, lean=-6, lid=0.55, eye_scale=0.68, eye_dy=4, smile=-0.05, head_dy=2, brow=0.55),
        "Narrow steady eye, level mouth.",
    ),
    "greet": (
        pose(head_dy=-12, lean=-6, smile=1.0, eye_scale=1.08, blush=0.25, lid=0.05),
        "Chin up, slight lean back, open eye, big smile.",
    ),
    "agree": (
        pose(head_dy=20, smile=0.9, lid=0.15, squash=0.96),
        "Nod down, smile.",
    ),
    "refuse": (
        pose(tilt=10, smile=-0.9, eye_scale=1.02, brow=0.7, lean=4, shift=6),
        "Head shake, frown, pinched brow.",
    ),
    "ponder": (
        pose(tilt=-10, squash=0.92, lean=-6, eye_dy=-8, eye_scale=0.95, lid=0.2, head_dy=6, smile=-0.1),
        "Eye rolled up, head tilt, small thoughtful mouth.",
    ),
    "annoyed": (
        pose(lid=0.5, eye_dy=-8, smile=-0.4, lean=5, brow=0.9, h0_dx=-8, h1_dx=4),
        "Eye rolled up under the lid, tight mouth, a pinched brow.",
    ),
    "celebrate": (
        pose(squash=0.96, lean=6, lid=0.75, smile=1.0, blush=0.85, mouth_open=0.1),
        "Tall happy squint, big smile, blush.",
    ),
    "hug": (
        pose(widen=1.08, squash=1.03, smile=0.85, blush=0.9, lid=0.2),
        "Wider body, blush, soft smile.",
    ),
    "shocked": (
        pose(lean=-8, squash=1.05, eye_scale=1.7, mouth_open=1.2, smile=-0.05, head_dy=-6),
        "Eye very wide, mouth round, slight lean back.",
    ),
}


def save_png(arr, path: Path):
    Image.fromarray(arr, "RGBA").save(path, format="PNG", optimize=True, compress_level=9)


def write_manifest(directory: Path, name: str, mouths):
    frames = []
    blob = bytearray()
    for path in sorted(directory.glob("*.png")):
        data = path.read_bytes()
        blob += data
        frames.append({"file": path.name, "sha256": hashlib.sha256(data).hexdigest()})
    keys = []
    for frame, x, y in mouths:
        if keys and keys[-1]["frame"] == frame:
            continue
        keys.append({"frame": frame, "x": round(float(x), 2), "y": round(float(y), 2)})
    if len(keys) == 1 and frames:
        keys.append({"frame": len(frames) - 1, "x": keys[0]["x"], "y": keys[0]["y"]})
    anchor = keys[0]
    manifest = {
        "name": name,
        "emotion": name,
        "fps": FPS,
        "frameCount": len(frames),
        "loop": False,
        "priority": 25,
        "mouthAnchor": {"frame": anchor["frame"], "x": anchor["x"], "y": anchor["y"]},
        "mouthKeyframes": keys,
        "transitionIn": "",
        "transitionOut": "",
        "canvas": {"width": W, "height": H},
        "groundY": GROUND,
        "frames": frames,
        "reviewed": True,
        "sourceSha256": hashlib.sha256(blob).hexdigest(),
        "issues": [],
    }
    (directory / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def clip_times(phase, index, count):
    if index == 0:
        return 0.0
    if phase == "in":
        return 0.9 * index / max(count - 1, 1)
    if phase == "main":
        peak_at = max(count // 2, 1)
        if index >= peak_at:
            return 1.0
        return index / peak_at
    return max(0.0, 1.0 - index / max(count - 1, 1))


def render_clip(parts, canon_bytes, emotion_id, peak, phase, count):
    name = emotion_id if phase == "main" else f"{emotion_id}_{phase}"
    dest = ANIM / name
    if dest.exists():
        shutil.rmtree(dest)
    dest.mkdir(parents=True)
    anchors = parts["anchors"]
    mouth = anchors["mouth"]
    mouths = []
    for i in range(count):
        t = clip_times(phase, i, count)
        path = dest / f"{i:04d}.png"
        if i == 0:
            path.write_bytes(canon_bytes)
            mouths.append((0, mouth[0], mouth[1]))
            continue
        arr = render_pose(parts, scale_pose(peak, t))
        save_png(arr, path)
        mx, my = forward_point(mouth[0], mouth[1], scale_pose(peak, t), anchors)
        mouths.append((i, mx, my))
    write_manifest(dest, name, mouths)
    if phase == "main":
        path = dest / "manifest.json"
        manifest = json.loads(path.read_text(encoding="utf-8"))
        manifest["transitionIn"] = f"{emotion_id}_in"
        manifest["transitionOut"] = f"{emotion_id}_out"
        path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return sum(p.stat().st_size for p in dest.glob("*.png"))


def render_all(parts):
    canon_bytes = CANON_PNG.read_bytes()
    total_new = 0
    for emotion_id, (peak, _desc) in PEAKS.items():
        print(f"render {emotion_id}", flush=True)
        total_new += render_clip(parts, canon_bytes, emotion_id, peak, "in", N_IN)
        total_new += render_clip(parts, canon_bytes, emotion_id, peak, "main", N_MAIN)
        total_new += render_clip(parts, canon_bytes, emotion_id, peak, "out", N_OUT)
    total = sum(p.stat().st_size for p in ANIM.rglob("*.png"))
    print(f"new png bytes {total_new} ({total_new / 1024 / 1024:.2f} MiB)")
    print(f"library png bytes {total} ({total / 1024 / 1024:.2f} MiB)")
    if total > SAFE_BUDGET:
        raise SystemExit(f"frame bytes {total} exceed safe budget {SAFE_BUDGET}")


def on_white(im):
    bg = Image.new("RGBA", im.size, (255, 255, 255, 255))
    bg.alpha_composite(im)
    return bg.convert("RGB")


def paste_cell(sheet, draw, im, index, cell_w, cell_h, label):
    thumb = on_white(im)
    thumb.thumbnail((cell_w - 8, cell_h - 22))
    cols = sheet.size[0] // cell_w
    r, c = divmod(index, cols)
    x = c * cell_w + (cell_w - thumb.size[0]) // 2
    y = r * cell_h + 2
    sheet.paste(thumb, (x, y))
    draw.text((c * cell_w + 4, r * cell_h + cell_h - 16), label, fill=(20, 20, 20))


def contact_sheet(parts):
    cols = 6
    cell_w, cell_h = 150, 214
    ids = list(PEAKS)
    rows = 1 + (len(ids) + cols - 1) // cols
    sheet = Image.new("RGB", (cols * cell_w, rows * cell_h), (244, 244, 246))
    draw = ImageDraw.Draw(sheet)
    canon = Image.fromarray(parts["canon"], "RGBA")
    proof = [canon, Image.open(ANIM / "happy" / "0000.png"), Image.open(ANIM / "scared" / "0000.png"), Image.open(ANIM / "hug" / "0000.png")]
    labels = ["reference", "happy/0000", "scared/0000", "hug/0000"]
    for i, (label, im) in enumerate(zip(labels, proof)):
        paste_cell(sheet, draw, im.convert("RGBA"), i, cell_w, cell_h, label)
    for i, emotion_id in enumerate(ids):
        im = Image.open(ANIM / emotion_id / "0004.png")
        paste_cell(sheet, draw, im.convert("RGBA"), cols + i, cell_w, cell_h, emotion_id)
    for path in (
        ROOT / "source" / "emotion" / "contact-sheet.png",
        Path("/opt/cursor/artifacts/emotion-contact-sheet.png"),
    ):
        path.parent.mkdir(parents=True, exist_ok=True)
        sheet.save(path, quality=92)
        print("wrote", path)


def motion_sheet():
    ids = list(PEAKS)
    frames = (0, 2, 4, 5)
    cell_w, cell_h = 110, 150
    sheet = Image.new("RGB", (len(frames) * cell_w + 78, len(ids) * cell_h), (244, 244, 246))
    draw = ImageDraw.Draw(sheet)
    for r, emotion_id in enumerate(ids):
        draw.text((4, r * cell_h + 8), emotion_id, fill=(20, 20, 20))
        for c, index in enumerate(frames):
            im = Image.open(ANIM / emotion_id / f"{index:04d}.png").convert("RGBA")
            thumb = on_white(im)
            thumb.thumbnail((cell_w - 8, cell_h - 8))
            x = 78 + c * cell_w + (cell_w - thumb.size[0]) // 2
            y = r * cell_h + (cell_h - thumb.size[1]) // 2
            sheet.paste(thumb, (x, y))
    for path in (
        ROOT / "source" / "emotion" / "motion-sheet.png",
        Path("/opt/cursor/artifacts/emotion-motion-sheet.png"),
    ):
        path.parent.mkdir(parents=True, exist_ok=True)
        sheet.save(path, quality=90)
        print("wrote", path)


def sync_catalog():
    data = json.loads(CATALOG_PATH.read_text(encoding="utf-8"))
    ids = [e["id"] for e in data["emotions"]]
    missing = set(PEAKS) - set(ids)
    if missing:
        raise SystemExit(f"catalog missing {missing}")
    for emotion in data["emotions"]:
        if emotion.get("legacy"):
            continue
        _peak, desc = PEAKS[emotion["id"]]
        for key in ("source", "start", "step", "count", "reverse"):
            emotion.pop(key, None)
        emotion["clip"] = f"animations/{emotion['id']}"
        emotion["transition_in"] = f"{emotion['id']}_in"
        emotion["transition_out"] = f"{emotion['id']}_out"
        emotion["fps"] = FPS
        emotion["production"] = (
            "Warp of source/emotion/reference-neutral.jpg via tools/render_emotions.py. "
            "Frame 0 of this clip and of its in/out clips is source/emotion/neutral-reference.png "
            "(uniform scale of that photo onto 420x560, backdrop removed). Later frames move that photo. "
            + desc
        )
        if emotion["id"] == "happy":
            emotion["note"] = (
                "Low intensity returns to idle. Intensity >= 0.6 still plays the embedded full laugh "
                "(toLaugh/video/fromLaugh). This plate opens on the reference artwork and warps into a smile."
            )
    CATALOG_PATH.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    text = CONTRACT_PATH.read_text(encoding="utf-8")
    old = (
        "Every other emotion is an original performance from `tools/render_emotions.py`: "
        "a soft-shaded pear mascot on its own skeleton, with its own eyes, mouth, and motion arc. "
        "Those frames are not crops, reverses, or renames of the legacy plates."
    )
    new = (
        "Every other emotion is a 2D warp of `source/emotion/reference-neutral.jpg` "
        "by `tools/render_emotions.py`. Frame 0 of each new clip (`animations/<id>`, `<id>_in`, `<id>_out`) "
        "is `source/emotion/neutral-reference.png`, that photo scaled onto the 420×560 plate. "
        "Later frames keep those pixels: the body squashes, leans, and nods, and the eye, mouth, blush, and tears "
        "are painted with colors taken from the photo. "
        "They are not crops, reverses, or renames of the legacy plates, and they are not a newly shaded character."
    )
    if old in text:
        text = text.replace(old, new, 1)
    elif new not in text:
        raise SystemExit("contract performance paragraph was not found")
    start = text.find("<!-- EMOTION_TABLE_START -->")
    end = text.find("<!-- EMOTION_TABLE_END -->")
    if start < 0 or end < 0:
        raise SystemExit("contract table markers missing")
    rows = [
        "| id | 中文 | priority | clip | transition in | transition out |",
        "| --- | --- | --- | --- | --- | --- |",
        "| neutral | 待机 | low | （无片段，回到待机） |  |  |",
    ]
    for emotion in data["emotions"]:
        rows.append(
            "| {id} | {zh} | {priority} | `{clip}` | `{transition_in}` | `{transition_out}` |".format(**emotion)
        )
    start = text.find("\n", start) + 1
    CONTRACT_PATH.write_text(text[:start] + "\n".join(rows) + "\n" + text[end:], encoding="utf-8")


def render_peaks(parts):
    out = Path("/tmp/emotion-preview/ref-peaks")
    out.mkdir(parents=True, exist_ok=True)
    canon = Image.fromarray(parts["canon"], "RGBA")
    canon.save(out / "_frame0.png")
    images = [("_frame0", canon)]
    for emotion_id, (peak, _desc) in PEAKS.items():
        arr = render_pose(parts, peak)
        im = Image.fromarray(arr, "RGBA")
        im.save(out / f"{emotion_id}.png")
        images.append((emotion_id, im))
    cols = 6
    cell_w, cell_h = 150, 214
    sheet = Image.new("RGB", (cols * cell_w, ((len(images) + cols - 1) // cols) * cell_h), (244, 244, 246))
    draw = ImageDraw.Draw(sheet)
    for i, (label, im) in enumerate(images):
        paste_cell(sheet, draw, im, i, cell_w, cell_h, label)
    sheet.save("/tmp/emotion-preview/ref-peaks-sheet.png", quality=92)
    print("wrote /tmp/emotion-preview/ref-peaks-sheet.png")
    print("anchors", {k: parts["anchors"][k] for k in ("eye", "mouth", "hands", "feet", "top", "body_cx", "neck")})


def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else "all"
    canon = build_canonical()
    parts = build_parts(np.array(canon))
    print("anchors", parts["anchors"]["eye"], parts["anchors"]["mouth"], parts["anchors"]["hands"])
    if mode == "peaks":
        render_peaks(parts)
        return
    if mode == "all":
        render_all(parts)
        sync_catalog()
        contact_sheet(parts)
        motion_sheet()
        return
    raise SystemExit(f"unknown mode {mode}")


if __name__ == "__main__":
    main()
