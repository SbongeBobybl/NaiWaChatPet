#!/usr/bin/env python3
"""Render original NaiWa emotion performances.

Each new emotion is a soft-shaded 3D pear mascot posed on its own skeleton:
different arms, body arc, eyes, and mouth. Frames are painted here. They are
not windows, reverses, or renames of the legacy angry/sad/shy/surprised/confused
plates. Legacy directories are never written.

The character stays planted on groundY. Lean and squash are pose, not travel.
"""

from __future__ import annotations

import hashlib
import json
import math
import shutil
import sys
from pathlib import Path

import numpy as np
from PIL import Image, ImageDraw

ROOT = Path(__file__).resolve().parents[1]
ANIM = ROOT / "animations"
CATALOG_PATH = ROOT / "source" / "emotion" / "emotions.json"
CONTRACT_PATH = ROOT / "source" / "emotion" / "CONTRACT.md"

W, H = 420, 560
CX = 214.0
GROUND = 540.0
# Shrink slightly so raised arms stay inside the plate. Legacy bodies fill the
# frame; these performances need headroom above the pear.
GLOBAL_SCALE = 0.88
PIVOT_REST = 390.0
SAFE_BUDGET = 125 * 1024 * 1024

# Key light from the upper right, y axis points down.
LX, LY, LZ = 0.42, -0.58, 0.70
_LL = math.sqrt(LX * LX + LY * LY + LZ * LZ)
LX, LY, LZ = LX / _LL, LY / _LL, LZ / _LL

YELLOW = np.array([0.98, 0.78, 0.20], np.float32)
YELLOW_SH = np.array([0.62, 0.34, 0.06], np.float32)
BELLY = np.array([0.99, 0.90, 0.62], np.float32)
HAND = np.array([0.80, 0.55, 0.24], np.float32)
GREEN = np.array([0.42, 0.73, 0.38], np.float32)
GREEN_DK = np.array([0.16, 0.38, 0.18], np.float32)
WHITE = np.array([0.97, 0.98, 0.96], np.float32)
PINK = np.array([1.0, 0.42, 0.48], np.float32)
BROW = np.array([0.45, 0.24, 0.08], np.float32)
TEAR = np.array([0.75, 0.90, 1.0], np.float32)

# Rest-pose body ellipses: dx from CX, canvas y, rx, ry.
BODY = (
    (0.0, 98.0, 58.0, 52.0),
    (0.0, 188.0, 84.0, 64.0),
    (0.0, 278.0, 108.0, 90.0),
    (0.0, 368.0, 76.0, 64.0),
)
BELLY_E = (0.0, 292.0, 70.0, 58.0)
HEAD_Y = 98.0
SHOULDER_Y = 200.0
SHOULDER_X = 70.0
UPPER_LEN = 78.0
FORE_LEN = 70.0

N_IN, N_MAIN, N_OUT = 4, 8, 4
FPS = 12


def clamp(v, lo, hi):
    return lo if v < lo else hi if v > hi else v


def _scales(pose):
    squash = max(float(pose["squash"]), 0.35)
    sq = squash * GLOBAL_SCALE
    x_scale = GLOBAL_SCALE / math.sqrt(squash)
    return sq, x_scale


def forward_point(x, y, pose):
    sq, x_scale = _scales(pose)
    y1 = GROUND - (GROUND - y) * sq
    x1 = CX + (x - CX) * x_scale
    pivot = GROUND - (GROUND - PIVOT_REST) * sq
    a = math.radians(float(pose["lean"]))
    ca, sa = math.cos(a), math.sin(a)
    px, py = x1 - CX, y1 - pivot
    return ca * px - sa * py + CX + float(pose["shift"]), sa * px + ca * py + pivot


def inverse_grid(xs, ys, pose):
    sq, x_scale = _scales(pose)
    pivot = GROUND - (GROUND - PIVOT_REST) * sq
    a = -math.radians(float(pose["lean"]))
    ca, sa = math.cos(a), math.sin(a)
    px = xs - CX - float(pose["shift"])
    py = ys - pivot
    x1 = ca * px - sa * py + CX
    y1 = sa * px + ca * py + pivot
    x = CX + (x1 - CX) / x_scale
    y = GROUND - (GROUND - y1) / sq
    return x, y


def base_pose():
    return {
        "lean": 0.0,
        "squash": 1.0,
        "shift": 0.0,
        "face_dy": 0.0,
        "tilt": 0.0,
        "l_ik": 0.0,
        "l_raise": 16.0,
        "l_bend": 18.0,
        "l_tx": CX - 55.0,
        "l_ty": 310.0,
        "r_ik": 0.0,
        "r_raise": 16.0,
        "r_bend": 18.0,
        "r_tx": CX + 55.0,
        "r_ty": 310.0,
        "l_spread": 52.0,
        "r_spread": 52.0,
        "l_knee": 4.0,
        "r_knee": 4.0,
        "l_lift": 0.0,
        "r_lift": 0.0,
        "lid": 0.08,
        "eye_scale": 1.0,
        "look_x": 0.0,
        "look_y": 0.0,
        "eye": "green",
        "brow_l": 6.0,
        "brow_r": 6.0,
        "blush": 0.05,
        "tears": 0.0,
        "sweat": 0.0,
        "mouth": "smile",
        "mouth_open": 0.15,
        "l_hand": "open",
        "r_hand": "open",
        "l_back": 0.0,
        "r_back": 0.0,
        "hand_scale": 1.0,
        "zzz": 0.0,
        "hearts": 0.0,
        "spark": 0.0,
        "peek": 0.0,
        "t": 0.0,
    }


def pose(**kw):
    p = base_pose()
    p.update(kw)
    return p


def lerp_pose(a, b, t):
    out = {}
    for key, av in a.items():
        bv = b[key]
        if isinstance(av, (int, float)) and isinstance(bv, (int, float)):
            out[key] = av * (1.0 - t) + bv * t
        else:
            out[key] = av if t < 0.5 else bv
    return out


def smooth(t):
    t = clamp(t, 0.0, 1.0)
    return t * t * (3.0 - 2.0 * t)


def pulse(t):
    """0 at the loop ends, 1 at the mid frame."""
    return (1.0 - math.cos(2.0 * math.pi * t)) * 0.5


def fk_arm(side, raise_deg, bend_deg):
    sign = -1.0 if side == "l" else 1.0
    shoulder = (CX + sign * SHOULDER_X, SHOULDER_Y)
    a = math.radians(raise_deg)
    dx = sign * math.sin(a)
    dy = math.cos(a)
    elbow = (shoulder[0] + dx * UPPER_LEN, shoulder[1] + dy * UPPER_LEN)
    theta = -sign * math.radians(bend_deg)
    ca, sa = math.cos(theta), math.sin(theta)
    fdx = ca * dx - sa * dy
    fdy = sa * dx + ca * dy
    hand = (elbow[0] + fdx * FORE_LEN, elbow[1] + fdy * FORE_LEN)
    return shoulder, elbow, hand


def ik_arm(side, target):
    sign = -1.0 if side == "l" else 1.0
    shoulder = (CX + sign * SHOULDER_X, SHOULDER_Y)
    dx = target[0] - shoulder[0]
    dy = target[1] - shoulder[1]
    dist = math.hypot(dx, dy)
    reach = UPPER_LEN + FORE_LEN - 2.0
    dist = clamp(dist, 12.0, reach)
    if math.hypot(dx, dy) < 1e-3:
        ux, uy = sign, 1.0
    else:
        ux, uy = dx / math.hypot(dx, dy), dy / math.hypot(dx, dy)
    tx = shoulder[0] + ux * dist
    ty = shoulder[1] + uy * dist
    cos_a = (dist * dist + UPPER_LEN * UPPER_LEN - FORE_LEN * FORE_LEN) / (2.0 * dist * UPPER_LEN)
    cos_a = clamp(cos_a, -1.0, 1.0)
    turn = math.acos(cos_a)
    base = math.atan2(uy, ux)
    # Elbows bend away from the torso.
    elbow_ang = base - sign * turn
    elbow = (
        shoulder[0] + math.cos(elbow_ang) * UPPER_LEN,
        shoulder[1] + math.sin(elbow_ang) * UPPER_LEN,
    )
    return shoulder, elbow, (tx, ty)


def arm_rest(side, pose):
    ik = pose["l_ik"] if side == "l" else pose["r_ik"]
    if ik >= 0.5:
        tx = pose["l_tx"] if side == "l" else pose["r_tx"]
        ty = pose["l_ty"] if side == "l" else pose["r_ty"]
        return ik_arm(side, (tx, ty))
    raise_deg = pose["l_raise"] if side == "l" else pose["r_raise"]
    bend = pose["l_bend"] if side == "l" else pose["r_bend"]
    return fk_arm(side, raise_deg, bend)


def leg_rest(side, pose):
    sign = -1.0 if side == "l" else 1.0
    spread = pose["l_spread"] if side == "l" else pose["r_spread"]
    knee = pose["l_knee"] if side == "l" else pose["r_knee"]
    lift = pose["l_lift"] if side == "l" else pose["r_lift"]
    hip = (CX + sign * 32.0, 396.0)
    kn = (CX + sign * (36.0 + knee * 0.55), 468.0 - knee * 0.2)
    ankle = (CX + sign * spread, 518.0 - lift)
    foot = (ankle[0] + sign * 6.0, 528.0 - lift)
    return hip, kn, ankle, foot


def shade(nx, ny, nz, base):
    ndot = nx * LX + ny * LY + nz * LZ
    wrap = np.clip(ndot * 0.55 + 0.45, 0.0, 1.0)
    lit = wrap ** 0.9
    if base.ndim == 1:
        col = YELLOW_SH * (1.0 - lit)[..., None] + base * lit[..., None]
    else:
        shadow = np.empty_like(base)
        shadow[:] = YELLOW_SH
        col = shadow * (1.0 - lit)[..., None] + base * lit[..., None]
    spec = np.clip(ndot, 0.0, 1.0) ** 16 * 0.28
    rim = (1.0 - nz) ** 1.5 * 0.12
    col = col + (spec + rim)[..., None]
    return np.clip(col, 0.0, 1.0)


class Canvas:
    def __init__(self):
        self.rgb = np.zeros((H, W, 3), np.float32)
        self.a = np.zeros((H, W), np.float32)
        self.z = np.full((H, W), -1e9, np.float32)
        self.yy, self.xx = np.mgrid[0:H, 0:W].astype(np.float32)

    def _write(self, y0, y1, x0, x1, mask, zz, col, alpha):
        if y1 <= y0 or x1 <= x0:
            return
        zs = self.z[y0:y1, x0:x1]
        sel = mask & (zz > zs) & (alpha > 0.01)
        if not np.any(sel):
            return
        zs[sel] = zz[sel]
        self.rgb[y0:y1, x0:x1][sel] = col[sel]
        self.a[y0:y1, x0:x1][sel] = np.maximum(self.a[y0:y1, x0:x1][sel], alpha[sel])

    def stamp_ellipse(self, cx, cy, rx, ry, zbase, base, angle=0.0, clip_below=None):
        rx = max(rx, 0.8)
        ry = max(ry, 0.8)
        pad = max(rx, ry) + 3
        x0 = max(0, int(cx - pad))
        x1 = min(W, int(cx + pad) + 1)
        y0 = max(0, int(cy - pad))
        y1 = min(H, int(cy + pad) + 1)
        if x1 <= x0 or y1 <= y0:
            return
        xs = self.xx[y0:y1, x0:x1]
        ys = self.yy[y0:y1, x0:x1]
        dx = xs - cx
        dy = ys - cy
        if abs(angle) > 0.01:
            ca, sa = math.cos(angle), math.sin(angle)
            lx = ca * dx + sa * dy
            ly = -sa * dx + ca * dy
        else:
            lx, ly = dx, dy
        px = lx / rx
        py = ly / ry
        k0 = np.sqrt(px * px + py * py + 1e-8)
        sdf = (k0 - 1.0) * min(rx, ry)
        mask = sdf < 1.2
        if clip_below is not None:
            mask = mask & (ys > clip_below)
        e = np.clip(k0, 0.0, 1.0)
        nz = np.sqrt(np.clip(1.0 - e * e, 0.0, 1.0))
        gx = px / (k0 * rx)
        gy = py / (k0 * ry)
        if abs(angle) > 0.01:
            ca, sa = math.cos(angle), math.sin(angle)
            sgx = ca * gx - sa * gy
            sgy = sa * gx + ca * gy
            gx, gy = sgx, sgy
        nlen = np.sqrt(gx * gx + gy * gy) + 1e-8
        nx = gx / nlen * e
        ny = gy / nlen * e
        ln = np.sqrt(nx * nx + ny * ny + nz * nz) + 1e-8
        col = shade(nx / ln, ny / ln, nz / ln, base)
        alpha = np.clip(1.2 - sdf, 0.0, 1.0)
        self._write(y0, y1, x0, x1, mask, zbase + nz, col, alpha)

    def stamp_capsule(self, p0, p1, radius, zbase, base):
        radius = max(radius, 0.8)
        x0 = max(0, int(min(p0[0], p1[0]) - radius - 2))
        x1 = min(W, int(max(p0[0], p1[0]) + radius) + 3)
        y0 = max(0, int(min(p0[1], p1[1]) - radius - 2))
        y1 = min(H, int(max(p0[1], p1[1]) + radius) + 3)
        if x1 <= x0 or y1 <= y0:
            return
        xs = self.xx[y0:y1, x0:x1]
        ys = self.yy[y0:y1, x0:x1]
        vx = p1[0] - p0[0]
        vy = p1[1] - p0[1]
        l2 = vx * vx + vy * vy + 1e-6
        t = np.clip(((xs - p0[0]) * vx + (ys - p0[1]) * vy) / l2, 0.0, 1.0)
        cx = p0[0] + t * vx
        cy = p0[1] + t * vy
        dx = xs - cx
        dy = ys - cy
        d = np.sqrt(dx * dx + dy * dy) + 1e-6
        e = d / radius
        mask = e <= 1.15
        ee = np.clip(e, 0.0, 1.0)
        nz = np.sqrt(np.clip(1.0 - ee * ee, 0.0, 1.0))
        nx = dx / radius
        ny = dy / radius
        ln = np.sqrt(nx * nx + ny * ny + nz * nz) + 1e-8
        col = shade(nx / ln, ny / ln, nz / ln, base)
        alpha = np.clip((1.15 - e) * radius, 0.0, 1.0)
        self._write(y0, y1, x0, x1, mask, zbase + nz, col, alpha)

    def paint_body(self, pose):
        xs, ys = inverse_grid(self.xx, self.yy, pose)
        x0, x1 = 20, W - 20
        y0, y1 = 10, H - 4
        xr = xs[y0:y1, x0:x1]
        yr = ys[y0:y1, x0:x1]
        k = 46.0
        sdf = None
        gx = gy = None
        for dx, cy, rx, ry in BODY:
            px = (xr - (CX + dx)) / rx
            py = (yr - cy) / ry
            k0 = np.sqrt(px * px + py * py + 1e-8)
            s = (k0 - 1.0) * min(rx, ry)
            ggx = px / (k0 * rx)
            ggy = py / (k0 * ry)
            if sdf is None:
                sdf, gx, gy = s, ggx, ggy
                continue
            h = np.clip(0.5 + 0.5 * (s - sdf) / k, 0.0, 1.0)
            sdf = s * (1.0 - h) + sdf * h - k * h * (1.0 - h)
            gx = ggx * (1.0 - h) + gx * h
            gy = ggy * (1.0 - h) + gy * h
        mask = sdf < 1.3
        # Map the rest-space normal through the pose scale so light stays stable.
        e = np.clip(sdf / 72.0 + 1.0, 0.0, 1.0)
        nz = np.sqrt(np.clip(1.0 - np.minimum(e, 1.0) ** 2, 0.0, 1.0))
        nlen = np.sqrt(gx * gx + gy * gy) + 1e-8
        nx = gx / nlen * np.minimum(e, 1.0)
        ny = gy / nlen * np.minimum(e, 1.0)
        ln = np.sqrt(nx * nx + ny * ny + nz * nz) + 1e-8
        nx, ny, nz = nx / ln, ny / ln, nz / ln
        bdx, bcy, brx, bry = BELLY_E
        bel = ((xr - (CX + bdx)) / brx) ** 2 + ((yr - bcy) / bry) ** 2
        belly_w = np.clip(1.15 - bel, 0.0, 1.0) ** 0.65
        base = YELLOW * (1.0 - belly_w)[..., None] + BELLY * belly_w[..., None]
        # Cheek blush in rest space.
        blush = float(pose["blush"])
        if blush > 0.01:
            for s in (-1.0, 1.0):
                cx = CX + s * 46.0
                cy = 124.0 + float(pose["face_dy"])
                be = ((xr - cx) / 22.0) ** 2 + ((yr - cy) / 14.0) ** 2
                w = np.clip(1.0 - be, 0.0, 1.0) * blush * 0.85
                base = base * (1.0 - w)[..., None] + PINK * w[..., None]
        col = shade(nx, ny, nz, base)
        alpha = np.clip(1.3 - sdf, 0.0, 1.0)
        self._write(y0, y1, x0, x1, mask, 1.0 + nz, col, alpha)

    def paint_limbs(self, pose):
        for side in ("l", "r"):
            hip, knee, ankle, foot = leg_rest(side, pose)
            pts = [forward_point(*p, pose) for p in (hip, knee, ankle, foot)]
            self.stamp_capsule(pts[0], pts[1], 20.0, 0.3, YELLOW)
            self.stamp_capsule(pts[1], pts[2], 16.0, 0.35, YELLOW)
            self.stamp_ellipse(pts[3][0], pts[3][1], 28.0, 13.0, 0.5, YELLOW)
            sign = -1.0 if side == "l" else 1.0
            for ox in (-12.0, 0.0, 12.0):
                self.stamp_ellipse(pts[3][0] + sign * ox, pts[3][1] + 6.0, 7.0, 6.0, 0.6, YELLOW)
            shoulder, elbow, hand = arm_rest(side, pose)
            pts = [forward_point(*p, pose) for p in (shoulder, elbow, hand)]
            back = pose["l_back"] if side == "l" else pose["r_back"]
            z_arm = 0.15 if back > 0.5 else 2.2
            self.stamp_capsule(pts[0], pts[1], 18.0, z_arm, YELLOW)
            self.stamp_capsule(pts[1], pts[2], 15.0, z_arm + 0.2, YELLOW)
            self.stamp_ellipse(pts[1][0], pts[1][1], 16.0, 16.0, z_arm + 0.15, YELLOW)
            self.paint_hand(pts[1], pts[2], pose["l_hand"] if side == "l" else pose["r_hand"], side, pose)

    def paint_hand(self, elbow, hand, style, side, pose):
        hs = max(float(pose.get("hand_scale", 1.0)), 0.4)
        dx = hand[0] - elbow[0]
        dy = hand[1] - elbow[1]
        length = math.hypot(dx, dy) + 1e-6
        ux, uy = dx / length, dy / length
        ang = math.atan2(uy, ux)
        palm = (hand[0] + ux * 10.0 * hs, hand[1] + uy * 10.0 * hs)
        self.stamp_ellipse(palm[0], palm[1], 16.0 * hs, 13.0 * hs, 3.2, HAND, angle=ang)
        sign = -1.0 if side == "l" else 1.0
        if style == "fist":
            spreads = (-0.25, 0.0, 0.25)
            flen = 7.0
        elif style == "point":
            spreads = (-0.45, -0.15, 0.15, 0.7)
            flen = 8.0
        elif style == "spread":
            spreads = (-0.85, -0.28, 0.28, 0.85)
            flen = 18.0
        elif style == "together":
            spreads = (-0.12, -0.04, 0.04, 0.12)
            flen = 16.0
        elif style == "thumb":
            spreads = (-0.2, 0.05, 0.25)
            flen = 6.0
        else:
            spreads = (-0.55, -0.18, 0.18, 0.55)
            flen = 16.0
        for i, sp in enumerate(spreads):
            fa = ang + sp
            this = 24.0 if style == "point" and i == len(spreads) - 1 else flen
            this *= hs
            tip = (palm[0] + math.cos(fa) * (12.0 * hs + this), palm[1] + math.sin(fa) * (12.0 * hs + this))
            root = (palm[0] + math.cos(fa) * 8.0 * hs, palm[1] + math.sin(fa) * 8.0 * hs)
            self.stamp_capsule(root, tip, 4.2 * hs, 3.5, HAND)
        if style == "thumb":
            ta = -math.pi / 2
            tlen = 30.0 * hs
            thick = 7.0 * hs
        else:
            ta = ang + sign * 1.15
            tlen = 18.0 * hs
            thick = 5.0 * hs
        t_root = (palm[0] + math.cos(ta) * 4.0, palm[1] + math.sin(ta) * 4.0)
        t_tip = (palm[0] + math.cos(ta) * tlen, palm[1] + math.sin(ta) * tlen)
        self.stamp_capsule(t_root, t_tip, thick, 3.8, HAND)

    def paint_eyes(self, pose, centers):
        style = pose["eye"]
        scale = float(pose["eye_scale"])
        lid = float(pose["lid"])
        for i, (cx, cy) in enumerate(centers):
            this_lid = lid
            if pose["peek"] > 0.5 and i == 1:
                this_lid = min(lid, 0.35)
            if style == "wide":
                scl_r, iris_r, pup_r = 22.0 * scale, 9.0 * scale, 4.2 * scale
                sclera = True
            elif style in ("heart", "spiral"):
                scl_r, iris_r, pup_r = 18.0 * scale, 0.0, 0.0
                sclera = True
            elif style in ("shut_happy", "shut_sad"):
                continue
            else:
                scl_r, iris_r, pup_r = 15.0 * scale, 13.0 * scale, 5.5 * scale
                sclera = style == "half" or lid > 0.2
            ry = scl_r if sclera else iris_r
            cut = None
            if this_lid > 0.02 and style not in ("shut_happy", "shut_sad"):
                cut = cy - ry + 2.0 * ry * this_lid
            if sclera:
                self.stamp_ellipse(cx, cy, scl_r, scl_r * 1.05, 4.2, WHITE, clip_below=cut)
            if iris_r > 0.5:
                ix = cx + float(pose["look_x"]) * 6.0
                iy = cy + float(pose["look_y"]) * 6.0
                self.stamp_ellipse(ix, iy, iris_r, iris_r, 4.5, GREEN, clip_below=cut)
                self.stamp_ellipse(ix, iy, pup_r, pup_r, 4.7, np.array([0.05, 0.05, 0.06], np.float32), clip_below=cut)
                self.stamp_ellipse(ix - iris_r * 0.28, iy - iris_r * 0.32, iris_r * 0.28, iris_r * 0.22, 4.9, WHITE, clip_below=cut)


def feature_points(pose):
    hx, hy = forward_point(CX, HEAD_Y, pose)
    origin = (hx, hy)

    def feat(dx, dy):
        x, y = forward_point(CX + dx, HEAD_Y + dy + float(pose["face_dy"]), pose)
        a = math.radians(float(pose["tilt"]))
        ca, sa = math.cos(a), math.sin(a)
        px, py = x - origin[0], y - origin[1]
        return ca * px - sa * py + origin[0], sa * px + ca * py + origin[1]

    left = feat(-34.0, -16.0)
    right = feat(34.0, -16.0)
    mouth = feat(0.0, 22.0)
    return (hx, hy), left, right, mouth


def draw_poly_mouth(draw, cx, cy, pose):
    kind = pose["mouth"]
    open_amt = float(pose["mouth_open"])
    w = 36.0
    tilt = math.radians(float(pose["tilt"]))
    ca, sa = math.cos(tilt), math.sin(tilt)

    def rot(dx, dy):
        return cx + ca * dx - sa * dy, cy + sa * dx + ca * dy

    lip = (120, 48, 42, 255)
    inner = (54, 22, 28, 255)
    teeth = (248, 244, 236, 255)
    if kind == "smile":
        pts = [rot(-w, 2), rot(-w * 0.4, 10 + 8 * open_amt), rot(w * 0.4, 10 + 8 * open_amt), rot(w, 2)]
        draw.line(pts, fill=lip, width=5)
        if open_amt > 0.35:
            draw.polygon([rot(-w * 0.55, 4), rot(0, 8 + 18 * open_amt), rot(w * 0.55, 4)], fill=inner)
    elif kind == "grin":
        draw.polygon(
            [rot(-w, 0), rot(-w * 0.2, 16 + 14 * open_amt), rot(w * 0.2, 16 + 14 * open_amt), rot(w, 0)],
            fill=inner,
        )
        draw.polygon([rot(-w * 0.7, 1), rot(0, 8), rot(w * 0.7, 1)], fill=teeth)
        draw.line([rot(-w, -2), rot(0, 12), rot(w, -2)], fill=lip, width=4)
    elif kind == "frown":
        pts = [rot(-w, 8), rot(-w * 0.3, -2 - 6 * open_amt), rot(w * 0.3, -2 - 6 * open_amt), rot(w, 8)]
        draw.line(pts, fill=lip, width=5)
        if open_amt > 0.4:
            draw.polygon([rot(-w * 0.4, 4), rot(0, -4 - 14 * open_amt), rot(w * 0.4, 4)], fill=inner)
    elif kind == "o":
        rw = 12 + 10 * open_amt
        rh = 14 + 16 * open_amt
        box = [cx - rw, cy - rh * 0.2, cx + rw, cy + rh]
        draw.ellipse(box, fill=inner, outline=lip, width=4)
    elif kind == "flat":
        draw.line([rot(-w * 0.7, 2), rot(w * 0.7, 2)], fill=lip, width=5)
    elif kind == "smirk":
        draw.line([rot(-w * 0.2, 4), rot(w * 0.15, 2), rot(w * 0.85, -8)], fill=lip, width=5)
    elif kind == "pout":
        draw.ellipse([cx - 14, cy - 4, cx + 14, cy + 16], fill=(196, 92, 96, 255), outline=lip, width=3)
    elif kind == "yawn":
        draw.ellipse([cx - 18, cy - 6, cx + 18, cy + 28 + 10 * open_amt], fill=inner, outline=lip, width=4)
    elif kind == "wave":
        pts = []
        for i in range(9):
            u = i / 8.0
            pts.append(rot(-w + u * 2 * w, math.sin(u * 6.0 + pose["t"] * 6.0) * 6.0))
        draw.line(pts, fill=lip, width=5)
    else:
        draw.line([rot(-w * 0.6, 4), rot(w * 0.6, 4)], fill=lip, width=4)


def draw_brows(draw, left, right, pose):
    for (cx, cy), ang, side in (
        (left, pose["brow_l"], -1.0),
        (right, pose["brow_r"], 1.0),
    ):
        a = math.radians(ang)
        dx, dy = math.cos(a) * 16.0, math.sin(a) * 10.0
        x0, y0 = cx - side * 8 - dx, cy - 22 - dy
        x1, y1 = cx + side * 14 + dx * 0.2, cy - 20 + dy * 0.3
        draw.line([(x0, y0), (x1, y1)], fill=(110, 58, 18, 255), width=5)


def draw_arc_eyes(draw, left, right, pose):
    style = pose["eye"]
    if style not in ("shut_happy", "shut_sad", "heart", "spiral"):
        return
    for cx, cy in (left, right):
        if style == "shut_happy":
            draw.arc([cx - 16, cy - 14, cx + 16, cy + 10], 200, 340, fill=(70, 40, 20, 255), width=5)
        elif style == "shut_sad":
            draw.arc([cx - 16, cy - 6, cx + 16, cy + 16], 20, 160, fill=(70, 40, 20, 255), width=5)
        elif style == "heart":
            s = 12.0 * float(pose["eye_scale"])
            pts = []
            for i in range(24):
                t = i / 24.0 * math.tau
                x = s * 0.9 * math.sin(t) ** 3
                y = -s * (0.75 * math.cos(t) - 0.3 * math.cos(2 * t) - 0.15 * math.cos(3 * t) - 0.05 * math.cos(4 * t))
                pts.append((cx + x, cy + y))
            draw.polygon(pts, fill=(214, 46, 78, 255))
            draw.ellipse([cx - s * 0.45, cy - s * 0.55, cx - s * 0.05, cy - s * 0.15], fill=(255, 220, 230, 255))
        elif style == "spiral":
            draw.ellipse([cx - 16, cy - 16, cx + 16, cy + 16], fill=(245, 248, 250, 255))
            pts = []
            for i in range(28):
                t = i / 27.0
                r = 2 + 12 * t
                a = t * 4.2 * math.pi + float(pose["t"]) * math.tau
                pts.append((cx + math.cos(a) * r, cy + math.sin(a) * r))
            draw.line(pts, fill=(40, 40, 50, 255), width=3)


def draw_fx(draw, head, pose):
    hx, hy = head
    t = float(pose["t"])
    if pose["tears"] > 0.2:
        drop = (t * 36.0) % 28.0
        for s in (-1.0, 1.0):
            x = hx + s * 38
            y = hy + 8 + drop
            draw.ellipse([x - 5, y - 8, x + 5, y + 10], fill=(186, 220, 245, 230))
            y2 = hy + 8 + (drop + 16) % 28.0
            draw.ellipse([x - 3.5, y2, x + 3.5, y2 + 12], fill=(170, 210, 240, 180))
    if pose["sweat"] > 0.2:
        x, y = hx + 62, hy - 10 + 6 * math.sin(t * math.tau)
        draw.polygon([(x, y - 10), (x - 6, y + 6), (x + 6, y + 6)], fill=(190, 225, 245, 230))
    if pose["zzz"] > 0.5:
        for i, (ox, oy, sc) in enumerate(((18, -58, 10), (36, -78, 14), (56, -100, 18))):
            x = hx + ox
            y = hy + oy + math.sin(t * math.tau + i) * 3
            draw.line([(x, y), (x + sc, y), (x, y + sc), (x + sc, y + sc)], fill=(90, 110, 160, 230), width=3)
    if pose["hearts"] > 0.5:
        for i, ox in enumerate((-28, 24)):
            s = 8 + (i * 2)
            cy = hy - 70 - ((t * 20 + i * 12) % 18)
            cx = hx + ox
            pts = []
            for k in range(20):
                u = k / 20.0 * math.tau
                x = s * math.sin(u) ** 3
                y = -s * (0.75 * math.cos(u) - 0.3 * math.cos(2 * u) - 0.12 * math.cos(3 * u))
                pts.append((cx + x, cy + y))
            draw.polygon(pts, fill=(220, 60, 90, 230))
    if pose["spark"] > 0.5:
        for i, (ox, oy) in enumerate(((-80, -120), (86, -130), (10, -150), (70, -70))):
            tw = 0.45 + 0.55 * (0.5 + 0.5 * math.sin(t * math.tau * 2 + i))
            cx, cy = hx + ox, hy + oy
            r = 7 * tw
            draw.line([(cx - r, cy), (cx + r, cy), (cx, cy - r * 1.4), (cx, cy + r * 1.4)], fill=(255, 214, 80, 230), width=3)


def render_pose(pose) -> tuple[Image.Image, tuple[float, float]]:
    canvas = Canvas()
    canvas.paint_body(pose)
    canvas.paint_limbs(pose)
    head, left, right, mouth = feature_points(pose)
    if pose["eye"] not in ("shut_happy", "shut_sad"):
        canvas.paint_eyes(pose, (left, right))
    rgb = np.clip(canvas.rgb * 255.0, 0, 255).astype(np.uint8)
    alpha = np.clip(canvas.a * 255.0, 0, 255).astype(np.uint8)
    im = Image.fromarray(np.dstack([rgb, alpha]), "RGBA")
    draw = ImageDraw.Draw(im)
    draw_brows(draw, left, right, pose)
    draw_arc_eyes(draw, left, right, pose)
    draw_poly_mouth(draw, mouth[0], mouth[1], pose)
    draw_fx(draw, head, pose)
    return im, mouth


def perf_happy(t):
    p = pulse(t)
    return pose(
        squash=1.03 + 0.04 * p,
        l_raise=50 + 8 * p,
        l_bend=18,
        r_raise=50 + 8 * p,
        r_bend=18,
        eye="shut_happy",
        mouth="grin",
        mouth_open=0.55 + 0.25 * p,
        blush=0.25,
        l_hand="open",
        r_hand="open",
        t=t,
    )


def perf_proud(t):
    p = pulse(t)
    return pose(
        squash=1.05 + 0.03 * p,
        face_dy=-10,
        l_ik=1, r_ik=1,
        l_tx=CX - 128, l_ty=305,
        r_tx=CX + 128, r_ty=305,
        lid=0.45,
        look_y=0.75,
        brow_l=20, brow_r=-10,
        mouth="smirk",
        blush=0.12,
        l_hand="fist", r_hand="fist",
        t=t,
    )


def perf_sleepy(t):
    sway = math.sin(2 * math.pi * t)
    return pose(
        lean=7 * sway,
        squash=0.9,
        face_dy=10,
        l_raise=8, l_bend=8,
        r_raise=6, r_bend=10,
        lid=0.82,
        eye="shut_sad",
        mouth="yawn",
        mouth_open=0.7 + 0.3 * pulse(t),
        zzz=1,
        t=t,
    )


def perf_love(t):
    return pose(
        lean=6 * math.sin(2 * math.pi * t),
        l_ik=1, r_ik=1,
        l_tx=CX + 20, l_ty=188,
        r_tx=CX - 20, r_ty=196,
        eye="heart",
        eye_scale=1.15,
        mouth="smile",
        mouth_open=0.45,
        blush=0.95,
        hearts=1,
        l_hand="open", r_hand="open",
        t=t,
    )


def perf_scared(t):
    wob = math.sin(4 * math.pi * t)
    return pose(
        lean=4 * wob,
        squash=0.78 + 0.03 * wob,
        l_raise=158, l_bend=18,
        r_raise=162, r_bend=14,
        l_spread=70, r_spread=70,
        l_knee=28, r_knee=26,
        eye="wide",
        eye_scale=1.25,
        look_x=0.4 * wob,
        brow_l=-20, brow_r=-22,
        mouth="o",
        mouth_open=0.9,
        sweat=0.8,
        l_hand="spread", r_hand="spread",
        t=t,
    )


def perf_excited(t):
    p = pulse(t)
    alt = math.sin(2 * math.pi * t)
    return pose(
        squash=1.04 + 0.05 * p,
        l_raise=72 + 18 * alt,
        l_bend=108,
        r_raise=72 - 18 * alt,
        r_bend=108,
        eye="wide",
        eye_scale=1.08,
        mouth="grin",
        mouth_open=0.9,
        blush=0.4,
        l_hand="fist", r_hand="fist",
        t=t,
    )


def perf_bored(t):
    return pose(
        lean=-4,
        squash=0.94,
        tilt=-12,
        l_ik=1, l_tx=CX - 30, l_ty=108,
        r_raise=4, r_bend=6,
        lid=0.62,
        look_x=-0.55,
        brow_l=0, brow_r=2,
        mouth="flat",
        l_hand="open", r_hand="open",
        t=t,
    )


def perf_embarrassed(t):
    p = pulse(t)
    return pose(
        squash=0.93,
        l_ik=1, r_ik=1,
        l_tx=CX - 26, l_ty=86,
        r_tx=CX + 22, r_ty=90,
        lid=0.82,
        peek=1,
        blush=1.0,
        sweat=1.0,
        mouth="smile",
        mouth_open=0.1,
        l_hand="open", r_hand="open",
        t=t,
    )


def perf_curious(t):
    return pose(
        lean=6,
        tilt=18,
        face_dy=-4,
        l_raise=128, l_bend=6,
        r_ik=1, r_tx=CX + 120, r_ty=310,
        eye="wide",
        eye_scale=1.12,
        look_x=0.85, look_y=-0.55,
        brow_l=-22, brow_r=18,
        peek=1,
        mouth="o",
        mouth_open=0.55,
        l_hand="point", r_hand="open",
        t=t,
    )


def perf_relieved(t):
    p = pulse(t)
    return pose(
        squash=0.92 + 0.1 * (1 - p),
        face_dy=4,
        l_ik=1, l_tx=CX - 10, l_ty=230,
        r_raise=20, r_bend=16,
        eye="shut_happy",
        mouth="smile",
        mouth_open=0.35,
        blush=0.2,
        l_hand="open", r_hand="open",
        t=t,
    )


def perf_disappointed(t):
    return pose(
        squash=0.84,
        face_dy=16,
        lean=3 * math.sin(2 * math.pi * t),
        l_raise=6, l_bend=6,
        r_raise=8, r_bend=8,
        l_spread=36, r_spread=36,
        l_knee=18, r_knee=18,
        lid=0.5,
        look_y=0.8,
        brow_l=22, brow_r=24,
        eye="half",
        mouth="frown",
        mouth_open=0.35,
        l_hand="open", r_hand="open",
        t=t,
    )


def perf_grateful(t):
    p = pulse(t)
    return pose(
        lean=0,
        squash=0.9,
        face_dy=6 + 4 * p,
        shift=0,
        l_ik=1, r_ik=1,
        l_tx=CX - 8, l_ty=214,
        r_tx=CX + 8, r_ty=214,
        eye="shut_happy",
        mouth="smile",
        mouth_open=0.4,
        blush=0.4,
        l_hand="together", r_hand="together",
        t=t,
    )


def perf_worried(t):
    look = math.sin(2 * math.pi * t)
    return pose(
        squash=0.95,
        l_ik=1, r_ik=1,
        l_tx=CX - 14, l_ty=250,
        r_tx=CX + 14, r_ty=254,
        lid=0.15,
        look_x=0.9 * look,
        brow_l=-14, brow_r=-12,
        eye="wide",
        eye_scale=0.95,
        mouth="frown",
        mouth_open=0.25,
        sweat=0.45,
        l_hand="together", r_hand="together",
        t=t,
    )


def perf_smug(t):
    return pose(
        lean=-8,
        squash=1.04,
        face_dy=-8,
        l_ik=1, l_tx=CX - 20, l_ty=78,
        r_ik=1, r_tx=CX + 130, r_ty=325,
        lid=0.5,
        look_x=0.7, look_y=0.35,
        brow_l=-8, brow_r=22,
        mouth="smirk",
        l_hand="open", r_hand="fist",
        t=t,
    )


def perf_crying(t):
    bob = math.sin(4 * math.pi * t)
    return pose(
        squash=0.9 + 0.03 * bob,
        face_dy=6,
        l_ik=1, r_ik=1,
        l_tx=CX - 42, l_ty=140,
        r_tx=CX + 42, r_ty=146,
        eye="shut_sad",
        brow_l=26, brow_r=26,
        mouth="frown",
        mouth_open=0.85,
        tears=1,
        l_hand="open", r_hand="open",
        t=t,
    )


def perf_wronged(t):
    return pose(
        shift=16,
        lean=-8,
        tilt=-12,
        squash=0.96,
        l_ik=1, r_ik=1,
        l_tx=CX + 36, l_ty=268,
        r_tx=CX - 10, r_ty=250,
        lid=0.22,
        look_x=-1.0,
        brow_l=18, brow_r=8,
        mouth="pout",
        mouth_open=0.2,
        tears=0.55,
        blush=0.3,
        l_hand="fist", r_hand="fist",
        t=t,
    )


def perf_cheer(t):
    p = pulse(t)
    return pose(
        squash=1.02 + 0.04 * p,
        l_raise=148, l_bend=6,
        r_raise=148, r_bend=6,
        eye="shut_happy",
        mouth="o",
        mouth_open=0.8,
        blush=0.35,
        l_hand="open", r_hand="open",
        t=t,
    )


def perf_dizzy(t):
    return pose(
        lean=10 * math.cos(2 * math.pi * t),
        l_raise=36, l_bend=16,
        r_raise=36, r_bend=16,
        l_spread=64, r_spread=40,
        eye="spiral",
        eye_scale=1.1,
        mouth="wave",
        l_hand="open", r_hand="open",
        t=t,
    )


def perf_focused(t):
    return pose(
        face_dy=2,
        l_ik=1, r_ik=1,
        l_tx=CX - 46, l_ty=92,
        r_tx=CX + 46, r_ty=92,
        lid=0.28,
        look_y=0.15,
        brow_l=-14, brow_r=-14,
        eye="green",
        eye_scale=0.85,
        mouth="flat",
        l_hand="open", r_hand="together",
        t=t,
    )


def perf_greet(t):
    return pose(
        l_raise=150 - 28 * math.cos(2 * math.pi * t),
        l_bend=16,
        r_ik=1, r_tx=CX + 48, r_ty=300,
        eye="green",
        eye_scale=1.05,
        mouth="grin",
        mouth_open=0.5,
        blush=0.2,
        l_hand="open", r_hand="open",
        t=t,
    )


def perf_agree(t):
    return pose(
        face_dy=16 * pulse(t),
        l_ik=1, l_tx=CX - 55, l_ty=230,
        r_raise=22, r_bend=18,
        eye="green",
        mouth="smile",
        mouth_open=0.45,
        l_hand="thumb", r_hand="open",
        t=t,
    )


def perf_refuse(t):
    shake = math.sin(4 * math.pi * t)
    return pose(
        tilt=12 * shake,
        l_ik=1, r_ik=1,
        l_tx=CX - 78, l_ty=210,
        r_tx=CX + 78, r_ty=210,
        lid=0.12,
        brow_l=-18, brow_r=-18,
        mouth="frown",
        mouth_open=0.45,
        l_hand="spread", r_hand="spread",
        t=t,
    )


def perf_ponder(t):
    return pose(
        tilt=6,
        l_ik=1, l_tx=CX - 6, l_ty=62,
        r_back=1, r_raise=12, r_bend=8,
        lid=0.22,
        look_y=-1.0,
        brow_l=12, brow_r=-6,
        mouth="flat",
        l_hand="open", r_hand="open",
        t=t,
    )


def perf_annoyed(t):
    return pose(
        l_ik=1, r_ik=1,
        l_tx=CX + 30, l_ty=276,
        r_tx=CX - 24, r_ty=262,
        l_lift=40 * pulse(t),
        lid=0.35,
        look_y=-0.95,
        brow_l=-18, brow_r=-20,
        eye="half",
        mouth="flat",
        l_hand="fist", r_hand="fist",
        t=t,
    )


def perf_celebrate(t):
    return pose(
        squash=1.03,
        l_ik=1, r_ik=1,
        l_tx=CX - 12, l_ty=36,
        r_tx=CX + 12, r_ty=36,
        eye="shut_happy",
        mouth="grin",
        mouth_open=0.75,
        blush=0.5,
        spark=1,
        l_hand="open", r_hand="open",
        t=t,
    )


def perf_hug(t):
    p = pulse(t)
    return pose(
        squash=1.02 + 0.03 * p,
        l_ik=1, r_ik=1,
        l_tx=CX - 28, l_ty=268,
        r_tx=CX + 28, r_ty=268,
        eye="shut_happy",
        mouth="smile",
        mouth_open=0.55,
        blush=0.85,
        hand_scale=1.35,
        l_hand="open", r_hand="open",
        t=t,
    )


def perf_shocked(t):
    wob = math.sin(6 * math.pi * t)
    return pose(
        lean=-4,
        squash=1.04,
        l_raise=78, l_bend=18,
        r_raise=78, r_bend=18,
        eye="wide",
        eye_scale=1.5,
        brow_l=-26, brow_r=-26,
        mouth="o",
        mouth_open=1.0,
        look_x=0.1 * wob,
        l_hand="spread", r_hand="spread",
        t=t,
    )


PERFORMANCES = {
    "happy": (perf_happy, "Arms lifted on a diagonal, shut smile-eyes, toothy grin, gentle bounce."),
    "proud": (perf_proud, "Both hands on hips, chest tall, chin up, one brow raised, smirk."),
    "sleepy": (perf_sleepy, "Arms hanging, slouch and sway, heavy lids, yawn, floating z's."),
    "love": (perf_love, "Self-hug across the chest, heart-shaped eyes, heavy blush, rising hearts."),
    "scared": (perf_scared, "Crouch with both arms thrown up, huge white eyes, round mouth, tremble."),
    "excited": (perf_excited, "Fists pump near the shoulders, body stretches, wide eyes, big grin."),
    "bored": (perf_bored, "Lean onto one hand on the cheek, other arm hanging, half-lidded flat mouth."),
    "embarrassed": (perf_embarrassed, "Both hands hide the face, full blush, one eye peeks, sweat drop."),
    "curious": (perf_curious, "Head tilt, one arm points up, mismatched brows, one wide eye, small o."),
    "relieved": (perf_relieved, "Hand on the chest, exhale squash, eyes closed in a soft smile."),
    "disappointed": (perf_disappointed, "Head bowed, arms limp, knees in, frown and sad brows."),
    "grateful": (perf_grateful, "Small bow, palms pressed together, eyes closed, warm smile."),
    "worried": (perf_worried, "Hands wrung at the stomach, inner brows up, eyes dart, small frown."),
    "smug": (perf_smug, "Lean back, one hand behind the head, one on the hip, asymmetric smirk."),
    "crying": (perf_crying, "Hands on the cheeks, squeezed eyes, open sob, falling tears."),
    "wronged": (perf_wronged, "Hip cocked, arms folded tight, side-eye, pout, a single tear line."),
    "cheer": (perf_cheer, "Both arms straight up in a V, bounce, happy eyes, open cheer mouth."),
    "dizzy": (perf_dizzy, "Body rocks side to side, limp arms, spiral eyes, wavy mouth."),
    "focused": (perf_focused, "Both hands frame the eyes like a visor, brows down, small steady eyes, flat mouth."),
    "greet": (perf_greet, "One arm waves overhead, the other rests in front, open grin."),
    "agree": (perf_agree, "Nod, one hand gives a thumbs-up, smile."),
    "refuse": (perf_refuse, "Both palms pushed forward, head shakes, brows down, frown."),
    "ponder": (perf_ponder, "One hand rests on top of the head, eyes look up, the other arm is behind the back."),
    "annoyed": (perf_annoyed, "Arms crossed, one foot taps, eyes rolled up, tight mouth."),
    "celebrate": (perf_celebrate, "Hands meet overhead in a clap, happy eyes, sparkles around the head."),
    "hug": (perf_hug, "Both arms come forward with open hands gathered at the belly, blush, closed smile."),
    "shocked": (perf_shocked, "Rigid arms straight out, body leans back, giant eyes, round mouth."),
}


def loop_pose(emotion_id, t):
    fn, _desc = PERFORMANCES[emotion_id]
    pose_out = fn(t)
    pose_out["t"] = t
    return pose_out


def main_t(index, count):
    """Build into the iconic pose (t=0.5) and hold it, so a non-looping player
    freezes on the readable emotion instead of a passing in-between."""
    peak = max(count // 2, 1)
    if index >= peak:
        return 0.5
    return 0.5 * index / peak


def frame_pose(emotion_id, phase, index, count):
    neutral = base_pose()
    if phase == "main":
        return loop_pose(emotion_id, main_t(index, count))
    if phase == "in":
        start = loop_pose(emotion_id, 0.0)
        t = (index + 1) / (count + 1)
        return lerp_pose(neutral, start, smooth(t))
    last = loop_pose(emotion_id, 0.5)
    t = (index + 1) / (count + 1)
    return lerp_pose(last, neutral, smooth(t))


def save_frame(im: Image.Image, path: Path):
    path.parent.mkdir(parents=True, exist_ok=True)
    im.save(path, format="PNG", optimize=True, compress_level=9)


def write_manifest(directory: Path, name: str, loop: bool, mouths: list[tuple[int, float, float]]):
    frames = []
    blob = bytearray()
    files = sorted(directory.glob("*.png"))
    for path in files:
        data = path.read_bytes()
        blob += data
        frames.append({"file": path.name, "sha256": hashlib.sha256(data).hexdigest()})
    keys = []
    for frame, x, y in mouths:
        if keys and keys[-1]["frame"] == frame:
            continue
        keys.append({"frame": frame, "x": round(x, 2), "y": round(y, 2)})
    if len(keys) < 2 and frames:
        keys = [
            {"frame": 0, "x": keys[0]["x"], "y": keys[0]["y"]},
            {"frame": len(frames) - 1, "x": keys[0]["x"], "y": keys[0]["y"]},
        ]
    anchor = keys[0]
    manifest = {
        "name": name,
        "emotion": name,
        "fps": FPS,
        "frameCount": len(frames),
        "loop": loop,
        "priority": 25,
        "mouthAnchor": {"frame": anchor["frame"], "x": anchor["x"], "y": anchor["y"]},
        "mouthKeyframes": keys,
        "transitionIn": "",
        "transitionOut": "",
        "canvas": {"width": W, "height": H},
        "groundY": int(GROUND),
        "frames": frames,
        "reviewed": True,
        "sourceSha256": hashlib.sha256(blob).hexdigest(),
        "issues": [],
    }
    (directory / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def render_clip(emotion_id, phase, count, loop):
    name = emotion_id if phase == "main" else f"{emotion_id}_{phase}"
    dest = ANIM / name
    if dest.exists():
        shutil.rmtree(dest)
    dest.mkdir(parents=True)
    mouths = []
    sample_at = {0, count // 2, count - 1}
    for i in range(count):
        posed = frame_pose(emotion_id, "main" if phase == "main" else phase, i, count)
        im, mouth = render_pose(posed)
        save_frame(im, dest / f"{i:04d}.png")
        if i in sample_at:
            mouths.append((i, mouth[0], mouth[1]))
    # Main clips point at their own transitions. In/out clips leave those blank,
    # matching angry_in / angry_out.
    write_manifest(dest, name, loop, mouths)
    if phase == "main":
        manifest_path = dest / "manifest.json"
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
        manifest["transitionIn"] = f"{emotion_id}_in"
        manifest["transitionOut"] = f"{emotion_id}_out"
        manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return sum(p.stat().st_size for p in dest.glob("*.png"))


def render_all():
    total_new = 0
    for emotion_id in PERFORMANCES:
        print(f"render {emotion_id}", flush=True)
        total_new += render_clip(emotion_id, "in", N_IN, False)
        total_new += render_clip(emotion_id, "main", N_MAIN, False)
        total_new += render_clip(emotion_id, "out", N_OUT, False)
    total = sum(p.stat().st_size for p in ANIM.rglob("*.png"))
    print(f"new png bytes {total_new} ({total_new/1024/1024:.2f} MiB)")
    print(f"library png bytes {total} ({total/1024/1024:.2f} MiB)")
    if total > SAFE_BUDGET:
        raise SystemExit(f"frame bytes {total} exceed safe budget {SAFE_BUDGET}")
    return total


def render_peaks(out_dir: Path):
    out_dir.mkdir(parents=True, exist_ok=True)
    images = []
    neutral, _ = render_pose(base_pose())
    neutral.save(out_dir / "_neutral.png")
    for emotion_id in PERFORMANCES:
        im, _ = render_pose(loop_pose(emotion_id, 0.5))
        im.save(out_dir / f"{emotion_id}.png")
        images.append((emotion_id, im))
    return images


def contact_sheet(images, path: Path, label_prefix=""):
    cols = 6
    cell_w, cell_h = 150, 214
    rows = (len(images) + cols - 1) // cols
    sheet = Image.new("RGBA", (cols * cell_w, rows * cell_h), (28, 28, 32, 255))
    draw = ImageDraw.Draw(sheet)
    for i, (emotion_id, im) in enumerate(images):
        thumb = im.copy()
        thumb.thumbnail((cell_w - 10, cell_h - 28))
        bg = Image.new("RGBA", thumb.size, (48, 48, 54, 255))
        bg.alpha_composite(thumb)
        r, c = divmod(i, cols)
        x = c * cell_w + (cell_w - bg.size[0]) // 2
        y = r * cell_h + 4
        sheet.alpha_composite(bg, (x, y))
        draw.text((c * cell_w + 6, r * cell_h + cell_h - 18), label_prefix + emotion_id, fill=(245, 245, 245, 255))
    path.parent.mkdir(parents=True, exist_ok=True)
    sheet.convert("RGB").save(path, quality=92)
    print("wrote", path)


def motion_sheet(path: Path):
    """Four frames per new emotion: start, middle of the rise, peak, held peak."""
    cols = 4
    frames = (0, 2, 4, 7)
    cell_w, cell_h = 120, 168
    ids = list(PERFORMANCES)
    sheet = Image.new("RGBA", (cols * cell_w + 72, len(ids) * cell_h), (28, 28, 32, 255))
    draw = ImageDraw.Draw(sheet)
    for r, emotion_id in enumerate(ids):
        draw.text((6, r * cell_h + 8), emotion_id, fill=(245, 245, 245, 255))
        for c, index in enumerate(frames):
            im = Image.open(ANIM / emotion_id / f"{index:04d}.png")
            thumb = im.copy()
            thumb.thumbnail((cell_w - 8, cell_h - 8))
            bg = Image.new("RGBA", thumb.size, (48, 48, 54, 255))
            bg.alpha_composite(thumb)
            x = 72 + c * cell_w + (cell_w - bg.size[0]) // 2
            y = r * cell_h + (cell_h - bg.size[1]) // 2
            sheet.alpha_composite(bg, (x, y))
    path.parent.mkdir(parents=True, exist_ok=True)
    sheet.convert("RGB").save(path, quality=90)
    print("wrote", path)


def sync_catalog():
    data = json.loads(CATALOG_PATH.read_text(encoding="utf-8"))
    ids = [e["id"] for e in data["emotions"]]
    if set(PERFORMANCES) - set(ids):
        raise SystemExit(f"catalog missing {set(PERFORMANCES) - set(ids)}")
    for emotion in data["emotions"]:
        if emotion.get("legacy"):
            continue
        if emotion["id"] not in PERFORMANCES:
            raise SystemExit(f"no performance for {emotion['id']}")
        _fn, desc = PERFORMANCES[emotion["id"]]
        for key in ("source", "start", "step", "count", "reverse"):
            emotion.pop(key, None)
        emotion["clip"] = f"animations/{emotion['id']}"
        emotion["transition_in"] = f"{emotion['id']}_in"
        emotion["transition_out"] = f"{emotion['id']}_out"
        emotion["fps"] = FPS
        emotion["production"] = (
            "Original soft-3D skeleton performance from tools/render_emotions.py. "
            + desc
            + " Not a crop, reverse, or rename of a legacy plate."
        )
        if emotion["id"] == "happy":
            emotion["note"] = (
                "Low intensity returns to idle. Intensity >= 0.6 still plays the embedded full laugh "
                "(toLaugh/video/fromLaugh). This plate is the distinct low-intensity happy performance."
            )
    CATALOG_PATH.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    text = CONTRACT_PATH.read_text(encoding="utf-8")
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
    return data


def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else "all"
    if mode == "peaks":
        images = render_peaks(Path("/tmp/emotion-preview/peaks"))
        contact_sheet(images, Path("/tmp/emotion-preview/peaks-sheet.png"))
        return
    if mode == "all":
        render_all()
        sync_catalog()
        images = []
        for emotion_id in PERFORMANCES:
            images.append((emotion_id, Image.open(ANIM / emotion_id / "0004.png")))
        contact_sheet(images, ROOT / "source" / "emotion" / "contact-sheet.png")
        contact_sheet(images, Path("/opt/cursor/artifacts/emotion-contact-sheet.png"))
        motion_sheet(ROOT / "source" / "emotion" / "motion-sheet.png")
        motion_sheet(Path("/opt/cursor/artifacts/emotion-motion-sheet.png"))
        return
    raise SystemExit(f"unknown mode {mode}")


if __name__ == "__main__":
    main()
