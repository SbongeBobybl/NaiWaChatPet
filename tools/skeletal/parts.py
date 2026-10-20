"""Cut the reference plate into rig parts.

Every visible pixel of neutral-reference.png has exactly one owner part that
carries the original RGBA. A part may also carry a feathered copy of opaque
pixels owned by a part below it (soft seam), and hidden underfill where a part
above it is fully opaque. Underfill and feathered copies are invisible in the
setup pose, so the setup pose composites back to the reference exactly.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from pathlib import Path

import cv2
import numpy as np
import scipy.sparse as sp
import scipy.sparse.linalg as spla
from PIL import Image, ImageDraw

ROOT = Path(__file__).resolve().parents[2]
REFERENCE = ROOT / "source" / "emotion" / "neutral-reference.png"
W, H = 420, 560

# Skeleton origin on the canvas: ground line under the feet.
ORIGIN = (195.0, 540.0)


def to_world(x: float, y: float) -> tuple[float, float]:
    return x - ORIGIN[0], ORIGIN[1] - y


def poly_mask(points, feather: float = 0.0, ss: int = 4) -> np.ndarray:
    """Antialiased polygon coverage, optionally blurred into a soft edge."""
    img = Image.new("L", (W * ss, H * ss), 0)
    ImageDraw.Draw(img).polygon([(x * ss, y * ss) for x, y in points], fill=255)
    m = np.array(img.resize((W, H), Image.Resampling.BOX)).astype(np.float32) / 255.0
    if feather > 0:
        m = cv2.GaussianBlur(m, (0, 0), feather)
    return np.clip(m, 0.0, 1.0)


def ramp(values: np.ndarray, full: float, zero: float) -> np.ndarray:
    """1 at `full`, 0 at `zero`, smooth in between."""
    t = np.clip((values - zero) / (full - zero), 0.0, 1.0)
    return t * t * (3 - 2 * t)


def disc(cx: float, cy: float, r: float, soft: float = 2.0) -> np.ndarray:
    yy, xx = np.mgrid[0:H, 0:W].astype(np.float32)
    d = np.hypot(xx - cx, yy - cy)
    return np.clip((r - d) / soft + 0.5, 0.0, 1.0)


def harmonic_fill(rgb: np.ndarray, known: np.ndarray, hole: np.ndarray) -> np.ndarray:
    """Laplace fill of `hole` from `known` neighbours. Unknown, non-hole
    neighbours are treated as no-flux, so the fill only draws on the part."""
    out = rgb.astype(np.float64).copy()
    ys, xs = np.where(hole)
    if len(xs) == 0:
        return rgb
    idx = -np.ones((H, W), np.int64)
    idx[ys, xs] = np.arange(len(xs))
    rows, cols, vals = [], [], []
    b = np.zeros((len(xs), 3))
    anchor = out[known].mean(axis=0) if known.any() else np.full(3, 128.0)
    for k, (y, x) in enumerate(zip(ys, xs)):
        deg = 0
        for dy, dx in ((-1, 0), (1, 0), (0, -1), (0, 1)):
            ny, nx = y + dy, x + dx
            if not (0 <= ny < H and 0 <= nx < W):
                continue
            if hole[ny, nx]:
                rows.append(k)
                cols.append(idx[ny, nx])
                vals.append(-1.0)
                deg += 1
            elif known[ny, nx]:
                b[k] += out[ny, nx]
                deg += 1
        rows.append(k)
        cols.append(k)
        # A faint pull to the part's mean keeps islands without known neighbours solvable.
        vals.append(float(deg) + 1e-4)
        b[k] += 1e-4 * anchor
    A = sp.csr_matrix((vals, (rows, cols)), shape=(len(xs), len(xs)))
    solve = spla.factorized(A.tocsc())
    for c in range(3):
        out[ys, xs, c] = solve(b[:, c])
    return np.clip(np.round(out), 0, 255).astype(np.uint8)


@dataclass
class PartSpec:
    name: str
    soft: np.ndarray  # coverage 0..1 over the canvas
    underfill: np.ndarray | None = None  # where hidden fill may live
    fill: str = "synth"  # "synth" harmonic fill or "dup" original pixels
    pivot: tuple[float, float] = (0.0, 0.0)  # canvas coords of the owning bone
    bone: str = ""


@dataclass
class Part:
    name: str
    bone: str
    pivot: tuple[float, float]
    rgba: np.ndarray  # full canvas layer
    bbox: tuple[int, int, int, int] = (0, 0, 0, 0)  # x0, y0, x1, y1 exclusive
    owned: np.ndarray = field(default_factory=lambda: np.zeros((H, W), bool))

    def crop(self) -> np.ndarray:
        x0, y0, x1, y1 = self.bbox
        return self.rgba[y0:y1, x0:x1]


def load_reference() -> np.ndarray:
    return np.array(Image.open(REFERENCE).convert("RGBA"))


def build_specs(ref: np.ndarray) -> list[PartSpec]:
    """Front to back. The last spec takes every pixel nobody else owns."""
    A = ref[:, :, 3]
    S = A > 0
    rgb = ref[:, :, :3].astype(np.int16)
    luma = rgb.mean(axis=2)
    chroma = rgb.max(axis=2) - rgb.min(axis=2)
    yy, xx = np.mgrid[0:H, 0:W].astype(np.float32)

    # Mitten olive: dark, warm, low blue.
    olive = S & (luma < 150) & ((rgb[:, :, 0] - rgb[:, :, 2]) < 110)

    # Near hand: the mitten up to the wrist, without the beige belly between the fingers.
    hand_poly = [(160, 276), (148, 275), (136, 276), (124, 281), (118, 293), (118, 306), (121, 320),
                 (130, 329), (146, 331), (162, 328), (172, 322), (171, 300), (168, 283)]
    hand_area = poly_mask(hand_poly) > 0.5
    beige_gap = (luma > 150) & (xx < 152)
    mitten = cv2.dilate((olive & hand_area).astype(np.uint8), np.ones((3, 3), np.uint8)) > 0
    hand_near = ((hand_area & ~beige_gap) | (mitten & hand_area)).astype(np.float32)

    # One outline for the whole near arm, split at the elbow by a single line,
    # so the two pieces share their seam without a gap.
    # The forearm's top edge follows the measured beige/yellow boundary.
    arm_poly = [(214, 200), (250, 194), (274, 202), (286, 222), (286, 242), (280, 258), (272, 274),
                (266, 292), (250, 301), (220, 310), (190, 319), (168, 326), (160, 300), (166, 284),
                (175, 276), (185, 268), (195, 261), (205, 257), (212, 255), (216, 255), (214, 222)]
    arm_whole = poly_mask(arm_poly) >= 0.5
    ex0, ey0, ex1, ey1 = 216.0, 255.0, 266.0, 292.0
    side = (ex1 - ex0) * (yy - ey0) - (ey1 - ey0) * (xx - ex0)
    arm_fore = (arm_whole & (side >= 0)).astype(np.float32)
    upper_hard = (arm_whole & (side < 0)).astype(np.float32)
    upper_poly = arm_poly
    # Shoulder and back of the upper arm blend into the body; the crease stays hard.
    shoulder = ramp(yy, 222, 206)
    back = ramp(xx, 268, 284)
    arm_upper = upper_hard * np.minimum(shoulder, back)

    far_poly = [(106, 268), (96, 272), (89, 285), (88, 302), (91, 318), (98, 327), (110, 326), (113, 300),
                (112, 276)]
    far_area = poly_mask(far_poly) > 0.5
    far_mitten = cv2.dilate((olive & far_area).astype(np.uint8), np.ones((3, 3), np.uint8)) > 0
    hand_far = ((far_mitten & far_area) | (far_area & (xx < 103) & S)).astype(np.float32)
    # Belly front edge hidden behind the far hand.
    belly_front = 97.0 + (yy - 272.0) * 0.09

    eye_near = disc(181.5, 115.0, 14.5, 1.5)

    # Head: everything above the neck line, softened into the body.
    neck = 171.0 + (xx - 150.0) * 0.02
    head = ramp(yy - neck, 0.0, 18.0)

    # Body owns down to the belly crease, then fades over the leg tops.
    body = ramp(yy, 401, 410)

    shadow = S & (yy >= 478) & (rgb.min(axis=2) > 165) & (chroma < 60)
    shadow = cv2.morphologyEx(shadow.astype(np.uint8), cv2.MORPH_OPEN, np.ones((2, 2), np.uint8)) > 0
    shadow |= S & (yy >= 478) & (A < 255) & (luma > 180)
    low = S & ~shadow
    foot_far = (low & (yy >= 478) & (xx < 207)).astype(np.float32) * ramp(yy, 486, 478)
    foot_near = (low & (yy >= 490) & (xx >= 200) & ~(foot_far > 0)).astype(np.float32) * ramp(yy, 498, 490)
    leg_far = ((xx < 204.5) & (yy >= 380) & ~shadow).astype(np.float32)
    leg_near = ((xx >= 204.5) & (yy >= 380) & ~shadow).astype(np.float32)
    shadow_soft = shadow.astype(np.float32)

    hidden_arm = cv2.dilate(
        (arm_whole | (hand_near > 0)).astype(np.uint8),
        np.ones((5, 5), np.uint8),
    ) > 0
    def leg_cols(lo: float, hi: float, ankle: tuple[float, float]):
        return ((yy >= 360) & (yy < 414) & (xx >= lo) & (xx <= hi)) | (disc(ankle[0], ankle[1] + 4, 16, 3) > 0)
    return [
        PartSpec("hand_near", hand_near, None, pivot=(167, 296), bone="hand_near"),
        PartSpec("arm_fore", arm_fore, disc(167, 296, 15, 3) > 0, pivot=(243, 282), bone="arm_fore"),
        PartSpec("arm_upper", arm_upper, disc(243, 282, 24, 3) > 0, pivot=(246, 212), bone="arm_upper"),
        PartSpec("hand_far", hand_far, None, pivot=(108, 298), bone="hand_far"),
        PartSpec("eye_near", eye_near, None, pivot=(181.5, 115), bone="eye_near"),
        PartSpec("head", head, eye_near > 0, pivot=(205, 190), bone="head"),
        PartSpec(
            "body",
            body,
            hidden_arm | (far_area & (xx >= belly_front)) | (head > 0),
            pivot=(200, 405),
            bone="body",
        ),
        PartSpec("foot_near", foot_near, None, pivot=(245, 515), bone="foot_near"),
        PartSpec("foot_far", foot_far, None, pivot=(165, 498), bone="foot_far"),
        PartSpec("leg_near", leg_near, leg_cols(206, 290, (247, 494)), pivot=(245, 494), bone="leg_near"),
        PartSpec("leg_far", leg_far, leg_cols(130, 204, (188, 484)), pivot=(185, 482), bone="leg_far"),
        PartSpec("shadow", shadow_soft, None, pivot=(180, 522), bone="shadow"),
    ]


def cut(ref: np.ndarray) -> list[Part]:
    specs = build_specs(ref)
    A = ref[:, :, 3]
    S = A > 0
    opaque = A == 255
    n = len(specs)
    owner = -np.ones((H, W), np.int32)
    feather = np.zeros((n, H, W), np.float32)
    for i, spec in enumerate(specs):
        m = spec.soft
        free = S & (owner < 0)
        full = free & (((m >= 0.999) & opaque) | ((m >= 0.5) & ~opaque))
        owner[full] = i
        part_soft = free & opaque & (m > 0.002) & (m < 0.999)
        feather[i][part_soft] = m[part_soft]
    # Anything unclaimed is the body's.
    body_i = next(i for i, s in enumerate(specs) if s.name == "body")
    leftover = S & (owner < 0)
    owner[leftover] = body_i
    # A feathered copy must sit above its owner.
    for i in range(n):
        bad = (feather[i] > 0) & (owner <= i)
        feather[i][bad] = 0.0

    parts = []
    rgb = ref[:, :, :3]
    for i, spec in enumerate(specs):
        layer = np.zeros((H, W, 4), np.uint8)
        own = owner == i
        layer[own] = ref[own]
        f = feather[i] > 0
        layer[f, :3] = rgb[f]
        layer[f, 3] = np.round(feather[i][f] * 255).astype(np.uint8)
        if spec.underfill is not None:
            # Hidden only: a part in front of this one owns it fully opaque.
            hidden = spec.underfill & opaque & (owner >= 0) & (owner < i) & (feather[i] == 0)
            if spec.name == "body":
                head_i = next(k for k, s in enumerate(specs) if s.name == "head")
                dup = hidden & (owner == head_i) | (hidden & (owner == next(k for k, s in enumerate(specs) if s.name == "eye_near")))
                layer[dup] = ref[dup]
                synth = hidden & ~dup
            else:
                synth = hidden
            known = own | (layer[:, :, 3] == 255) & ~synth
            if synth.any():
                filled = harmonic_fill(rgb, known & (layer[:, :, 3] > 0), synth)
                layer[synth, :3] = filled[synth]
                layer[synth, 3] = 255
        ys, xs = np.where(layer[:, :, 3] > 0)
        bbox = (int(xs.min()), int(ys.min()), int(xs.max()) + 1, int(ys.max()) + 1)
        parts.append(Part(spec.name, spec.bone, spec.pivot, layer, bbox, own))
    return parts


def composite_setup(parts: list[Part]) -> np.ndarray:
    """Back-to-front straight-alpha over, in float, rounded once."""
    acc = np.zeros((H, W, 3), np.float64)
    acc_a = np.zeros((H, W), np.float64)
    for part in reversed(parts):
        a = part.rgba[:, :, 3].astype(np.float64) / 255.0
        c = part.rgba[:, :, :3].astype(np.float64)
        acc = c * a[..., None] + acc * (1 - a[..., None])
        acc_a = a + acc_a * (1 - a)
    out = np.zeros((H, W, 4), np.uint8)
    safe = np.maximum(acc_a, 1e-9)
    out[:, :, :3] = np.clip(np.round(acc / safe[..., None]), 0, 255)
    out[:, :, 3] = np.clip(np.round(acc_a * 255), 0, 255)
    out[acc_a <= 0] = 0
    return out
