#!/usr/bin/env python3
"""Three skill clips for the reference frog.

Frame 0 of every skill is source/emotion/neutral-reference.png byte for byte.
Throw and roll use drawn poses of that same frog. The fan, the off-screen
burst, and the fire are separate layers. Power-up never repaints the
character: fire is composited underneath the reference plate. Nothing is
optical-flowed, and the roll is a rigid spin of the curled pose.
"""

from __future__ import annotations

import hashlib
import json
import shutil
import sys
from pathlib import Path

import cv2
import numpy as np
from PIL import Image, ImageDraw

sys.path.insert(0, str(Path(__file__).resolve().parent))
from render_emotions import (  # noqa: E402
    CANON_PNG,
    GROUND,
    H,
    W,
    align_to_canon,
    body_mask,
    bbox_of,
    canon_metrics,
    finish,
    head_anchor,
    key_rgba,
    shift,
)

ROOT = Path(__file__).resolve().parents[1]
KEYS = ROOT / "source" / "emotion" / "keys"
ANIM = ROOT / "animations"
SKILLS_PATH = ROOT / "source" / "emotion" / "skills.json"
FPS = 12

# Curled poses are shorter than the standing plate. Scaling them to the full
# standing height would inflate the character.
TUCK_HEIGHT = 0.78
BALL_HEIGHT = 0.56


def load_standing(name: str, cm: dict) -> np.ndarray:
    return finish(align_to_canon(key_rgba(Image.open(KEYS / name)), cm))


def place_shorter(name: str, cm: dict, height_ratio: float) -> np.ndarray:
    src = key_rgba(Image.open(KEYS / name))
    _, y0, _, y1 = bbox_of(body_mask(src))
    scale = (cm["h"] * height_ratio) / max(y1 - y0 + 1, 1)
    nw = max(1, int(round(src.shape[1] * scale)))
    nh = max(1, int(round(src.shape[0] * scale)))
    resized = np.array(Image.fromarray(src).resize((nw, nh), Image.Resampling.LANCZOS))
    mask = body_mask(resized)
    _, y0, _, y1 = bbox_of(mask)
    from render_emotions import feet_cx

    dx = cm["fcx"] - feet_cx(mask, y1, y0)
    dy = cm["y1"] - y1
    dest = np.zeros((H, W, 4), np.uint8)
    h2, w2 = resized.shape[:2]
    x_src0, y_src0 = max(0, -dx), max(0, -dy)
    x_dst0, y_dst0 = max(0, dx), max(0, dy)
    x_src1, y_src1 = min(w2, W - dx), min(h2, H - dy)
    if x_src1 > x_src0 and y_src1 > y_src0:
        dest[y_dst0 : y_dst0 + (y_src1 - y_src0), x_dst0 : x_dst0 + (x_src1 - x_src0)] = resized[
            y_src0:y_src1, x_src0:x_src1
        ]
    return finish(dest)


def over(bg: np.ndarray, fg: np.ndarray) -> np.ndarray:
    """Straight-alpha source-over. Fully opaque foreground pixels are copied."""
    fa = fg[:, :, 3:4].astype(np.float32) / 255.0
    ba = bg[:, :, 3:4].astype(np.float32) / 255.0
    oa = fa + ba * (1.0 - fa)
    rgb = fg[:, :, :3].astype(np.float32) * fa + bg[:, :, :3].astype(np.float32) * ba * (1.0 - fa)
    rgb = np.divide(rgb, np.maximum(oa, 1e-6), where=oa > 1e-4, out=np.zeros_like(rgb))
    out = np.zeros_like(bg)
    out[:, :, :3] = np.clip(rgb, 0, 255).astype(np.uint8)
    out[:, :, 3] = np.clip(oa[:, :, 0] * 255.0, 0, 255).astype(np.uint8)
    solid = fg[:, :, 3] == 255
    out[solid] = fg[solid]
    return out


def under(character: np.ndarray, effect: np.ndarray) -> np.ndarray:
    """Effect behind the character. Opaque character pixels stay exact."""
    return over(effect, character)


def save_png(arr: np.ndarray, path: Path):
    Image.fromarray(arr, "RGBA").save(path, format="PNG", optimize=True, compress_level=9)


def throwing_hand(plate: np.ndarray) -> tuple[int, int]:
    """Screen-left olive mitten. The snout and the bright eye are not the hand."""
    mask = body_mask(plate, 40)
    rgb = plate[:, :, :3].astype(np.int16)
    r, g, b = rgb[:, :, 0], rgb[:, :, 1], rgb[:, :, 2]
    luma = rgb.mean(axis=2)
    chroma = rgb.max(axis=2) - rgb.min(axis=2)
    mitten = mask & (luma > 35) & (luma < 130) & (chroma > 8) & (np.abs(r - g) < 45) & (b < g + 10)
    ys, xs = np.where(mask)
    # Feet share the mitten color. Keep the upper body.
    y_cut = int(ys.min() + (ys.max() - ys.min()) * 0.82)
    mitten = mitten.copy()
    mitten[y_cut:, :] = False
    eye = mask & (g > r + 15) & (g > 100)
    mitten[eye] = False
    yy, xx = np.where(mitten)
    if len(xx) < 20:
        raise SystemExit("throwing hand not found")
    i = int(np.argmin(xx))
    lx, ly = int(xx[i]), int(yy[i])
    near = (xx - lx) ** 2 + (yy - ly) ** 2 < 48 ** 2
    return int(round(xx[near].mean())), int(round(yy[near].mean()))


def fan_sprite(spin: float, scale: float) -> np.ndarray:
    size = max(24, int(78 * scale))
    img = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img)
    pivot = (size // 2, int(size * 0.78))
    radius = int(size * 0.62)
    # Pleated paper. Cream and gold stay distinct from the yellow body.
    colors = [(243, 226, 184, 255), (231, 177, 74, 255), (250, 236, 206, 255)]
    start = -58
    sweep = 116
    wedges = 6
    for i in range(wedges):
        a0 = start + sweep * i / wedges
        a1 = start + sweep * (i + 1) / wedges
        draw.pieslice(
            [pivot[0] - radius, pivot[1] - radius, pivot[0] + radius, pivot[1] + radius],
            a0,
            a1,
            fill=colors[i % len(colors)],
        )
    for i in range(wedges + 1):
        ang = np.deg2rad(start + sweep * i / wedges)
        tip = (pivot[0] + radius * np.cos(ang), pivot[1] + radius * np.sin(ang))
        draw.line([pivot, tip], fill=(92, 70, 50, 255), width=max(1, size // 28))
    draw.ellipse(
        [pivot[0] - 4, pivot[1] - 4, pivot[0] + 4, pivot[1] + 4],
        fill=(70, 52, 36, 255),
    )
    rotated = img.rotate(spin, resample=Image.Resampling.BICUBIC, expand=False)
    return np.array(rotated)


def paste_center(base: np.ndarray, sprite: np.ndarray, cx: int, cy: int) -> np.ndarray:
    sh, sw = sprite.shape[:2]
    x0, y0 = int(cx - sw / 2), int(cy - sh / 2)
    out = base
    xs0, ys0 = max(0, x0), max(0, y0)
    xs1, ys1 = min(W, x0 + sw), min(H, y0 + sh)
    if xs1 <= xs0 or ys1 <= ys0:
        return out
    crop = sprite[ys0 - y0 : ys1 - y0, xs0 - x0 : xs1 - x0]
    patch = out.copy()
    patch[ys0:ys1, xs0:xs1] = over(out[ys0:ys1, xs0:xs1], crop)
    return patch


def explosion(stage: int, cx: int, cy: int) -> np.ndarray:
    """Impact just off the left edge. The burst enters the frame. No figure."""
    layer = np.zeros((H, W, 4), np.uint8)
    canvas = Image.fromarray(layer, "RGBA")
    draw = ImageDraw.Draw(canvas)
    if stage == 0:
        for rad, col in (
            (168, (255, 244, 200, 150)),
            (108, (255, 196, 80, 210)),
            (64, (255, 120, 30, 235)),
            (28, (255, 250, 230, 255)),
        ):
            draw.ellipse([cx - rad, cy - rad, cx + rad, cy + rad], fill=col)
    elif stage == 1:
        for i in range(18):
            ang = np.deg2rad(-70 + i * (220 / 17))
            length = 70 + (i % 4) * 28
            x1 = cx + np.cos(ang) * 16
            y1 = cy + np.sin(ang) * 16
            x2 = cx + np.cos(ang) * length
            y2 = cy + np.sin(ang) * length
            draw.line([(x1, y1), (x2, y2)], fill=(255, 132, 36, 230), width=5)
            draw.ellipse([x2 - 4, y2 - 4, x2 + 4, y2 + 4], fill=(255, 220, 120, 240))
        draw.ellipse([cx - 26, cy - 26, cx + 26, cy + 26], fill=(255, 236, 170, 245))
    else:
        fade = 168 if stage == 2 else 88
        lift = 0 if stage == 2 else 18
        puffs = ((36, -8, 46), (78, -36, 34), (18, 28, 30), (96, 8, 26), (54, -58, 22), (120, -16, 18))
        for dx, dy, rad in puffs:
            draw.ellipse(
                [cx + dx - rad, cy + dy - lift - rad, cx + dx + rad, cy + dy - lift + rad],
                fill=(128, 120, 112, fade),
            )
        # Sparks still traveling into the frame after the hit.
        for i in range(7):
            sx = cx + 40 + i * 16
            sy = cy - 20 + (i % 3) * 14 - lift
            draw.ellipse([sx - 3, sy - 3, sx + 3, sy + 3], fill=(255, 170, 60, fade))
    return np.array(canvas)


def quantize(layer: np.ndarray, step: int = 8) -> np.ndarray:
    q = (layer // step) * step
    q[:, :, 3] = np.where(layer[:, :, 3] < step, 0, q[:, :, 3])
    return q.astype(np.uint8)


def additive(base: np.ndarray, src: np.ndarray) -> np.ndarray:
    """Add src over base. Used for flame light, not for the character."""
    a = src[:, :, 3:4].astype(np.float32) / 255.0
    out = base.astype(np.float32)
    out[:, :, :3] = np.clip(out[:, :, :3] + src[:, :, :3].astype(np.float32) * a, 0, 255)
    out[:, :, 3] = np.clip(np.maximum(out[:, :, 3], src[:, :, 3].astype(np.float32) * 0.92), 0, 255)
    return out.astype(np.uint8)


def ribbon(draw: ImageDraw.ImageDraw, pts: list[tuple[float, float]], widths: list[float], color: tuple[int, int, int, int]):
    left = []
    right = []
    for i, ((x, y), w) in enumerate(zip(pts, widths)):
        if i == 0:
            dx, dy = pts[1][0] - x, pts[1][1] - y
        elif i == len(pts) - 1:
            dx, dy = x - pts[i - 1][0], y - pts[i - 1][1]
        else:
            dx = pts[i + 1][0] - pts[i - 1][0]
            dy = pts[i + 1][1] - pts[i - 1][1]
        norm = float(np.hypot(dx, dy)) or 1.0
        px, py = -dy / norm * w, dx / norm * w
        left.append((x + px, y + py))
        right.append((x - px, y - py))
    draw.polygon(left + right[::-1], fill=color)


def tongue_wobble(t: float, seed: float, phase: float) -> float:
    return (
        np.sin(t * 3.2 + seed + phase) * 0.62
        + np.sin(t * 7.4 + seed * 1.6 - phase * 1.35) * 0.28
        + np.sin(t * 12.5 + seed * 0.45 + phase * 2.05) * 0.14
    )


def fire_layers(character: np.ndarray, frame_i: int, frame_n: int) -> tuple[np.ndarray, np.ndarray]:
    """Behind-body fire and a hotter front lick. Shapes are rebuilt every frame."""
    phase = 2.0 * np.pi * frame_i / frame_n
    pulse = 0.84 + 0.16 * (0.5 + 0.5 * np.sin(phase))
    mask = (character[:, :, 3] > 80).astype(np.uint8) * 255
    contours, _ = cv2.findContours(mask, cv2.RETR_EXTERNAL, cv2.CHAIN_APPROX_NONE)
    contour = max(contours, key=cv2.contourArea)[:, 0, :]
    filled = np.zeros_like(mask)
    cv2.drawContours(filled, [contour], -1, 255, thickness=-1)
    cx = float(contour[:, 0].mean())
    cy = float(contour[:, 1].mean())

    anchors = []
    step = max(1, len(contour) // 18)
    for n, idx in enumerate(range(0, len(contour), step)):
        x, y = float(contour[idx][0]), float(contour[idx][1])
        direction = np.array([x - cx, (y - cy) * 0.45 - 26.0], np.float32)
        norm = float(np.linalg.norm(direction))
        if norm < 8:
            continue
        direction /= norm
        if y > GROUND - 16 and direction[1] > 0.2:
            continue
        anchors.append((n, x, y, direction))

    body = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    hot = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    bdraw = ImageDraw.Draw(body)
    hdraw = ImageDraw.Draw(hot)
    for n, x, y, direction in anchors:
        seed = n * 1.37
        height_wave = 0.62 + 0.38 * np.sin(phase * 2.0 + seed * 1.8)
        length = (34 + 78 * pulse) * height_wave
        width = (8 + 7 * pulse) * (0.75 + 0.25 * np.sin(phase + seed))
        # Tongues rise. Side anchors lean out a little; none are a copied sprite.
        pts = []
        for s in range(11):
            t = s / 10
            wobble = tongue_wobble(t, seed, phase) * width * (0.25 + t * 1.15)
            lean = direction[0] * length * t * 0.42
            pts.append((x + lean + wobble, y - length * (t ** 0.92) + direction[1] * length * t * 0.12))
        outer_w = [width * (1.0 - t) ** 0.55 * (1.05 + 0.18 * np.sin(phase + seed + t * 5)) for t in np.linspace(0, 1, 11)]
        mid_w = [w * 0.62 for w in outer_w]
        core_w = [w * 0.28 for w in outer_w]
        ribbon(bdraw, pts, outer_w, (176, 28, 6, int(150 * pulse)))
        ribbon(bdraw, pts, mid_w, (255, 104, 12, int(185 * pulse)))
        ribbon(bdraw, pts[:8], [width * 0.34 * (1 - s / 7) for s in range(8)], (255, 214, 70, int(200 * pulse)))
        ribbon(hdraw, pts[:6], core_w[:6], (255, 248, 220, int(210 * pulse)))

    flames = np.array(body)
    cores = np.array(hot)
    # Soft additive glow follows the tongues, then falls off.
    glow_src = flames.copy()
    blur_tight = cv2.GaussianBlur(glow_src, (0, 0), 5)
    blur_wide = cv2.GaussianBlur(glow_src, (0, 0), 16)
    wide = blur_wide.copy()
    wide[:, :, 0] = np.clip(wide[:, :, 0].astype(np.int16) + 20, 0, 255).astype(np.uint8)
    wide[:, :, 1] = (wide[:, :, 1].astype(np.int16) // 2).astype(np.uint8)
    wide[:, :, 3] = (wide[:, :, 3].astype(np.float32) * 0.55).astype(np.uint8)
    glow = additive(wide, blur_tight)

    # Warm pool on the ground, wide and soft, not a white disc.
    yy, xx = np.mgrid[0:H, 0:W]
    feet = np.where(mask[GROUND - 30 : GROUND, :] > 0)
    foot_x = float(feet[1].mean()) if len(feet[1]) else cx
    nx = (xx - foot_x) / 168.0
    ny = (yy - (GROUND - 8)) / 36.0
    pool = np.clip(1.0 - (nx * nx + ny * ny), 0, 1) ** 1.7
    ground = np.zeros((H, W, 4), np.uint8)
    ground[:, :, 0] = 255
    ground[:, :, 1] = 78
    ground[:, :, 2] = 12
    ground[:, :, 3] = (np.floor(pool * (55 + 40 * pulse) / 4.0) * 4).astype(np.uint8)
    ground[filled > 0, 3] = 0

    embers = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    edraw = ImageDraw.Draw(embers)
    for i in range(22):
        life = (i / 22.0 + frame_i / frame_n) % 1.0
        fade = np.sin(life * np.pi)
        if fade < 0.08:
            continue
        ang = i * 2.399
        sx = cx + np.cos(ang) * 78 + np.sin(phase + i) * 10
        sy = cy - 20 + np.sin(ang) * 90
        ex = sx + np.sin(life * 6.2 + i) * 7
        ey = sy - life * (64 + (i % 5) * 22)
        if ey < 24 or not (0 <= int(ex) < W and 0 <= int(ey) < H):
            continue
        if filled[int(ey), int(ex)] > 0:
            continue
        rad = 3 if i % 3 else 4
        hot_ember = i % 4 == 0
        col = (255, 250, 220, int(230 * fade)) if hot_ember else (255, 140, 24, int(220 * fade))
        edraw.ellipse([ex - rad, ey - rad * 1.4, ex + rad, ey + rad], fill=col)

    behind = additive(glow, flames)
    behind = additive(behind, ground)
    behind = additive(behind, np.array(embers))
    behind = heat_haze(behind, phase)
    behind[filled > 0] = 0
    behind = quantize(behind, 8)
    cores[filled > 0] = 0
    cores = quantize(cores, 8)
    return behind, cores


def heat_haze(layer: np.ndarray, phase: float) -> np.ndarray:
    """Waver the fire only. The character is composited after this."""
    ys, xs = np.mgrid[0:H, 0:W]
    shift = np.sin(ys * 0.085 + phase) * 2.4 + np.sin(ys * 0.21 - phase * 1.5) * 1.1
    # More haze where the flames are bright, none as a hard warp of empty corners.
    weight = np.clip(layer[:, :, 3].astype(np.float32) / 80.0, 0, 1)
    map_x = (xs + shift * weight).astype(np.float32)
    map_y = ys.astype(np.float32)
    return cv2.remap(layer, map_x, map_y, cv2.INTER_LINEAR, borderMode=cv2.BORDER_CONSTANT, borderValue=(0, 0, 0, 0))


def lick(character: np.ndarray, behind: np.ndarray, front: np.ndarray) -> np.ndarray:
    """Fire behind the body, hotter tips around the edge. Solid pixels stay exact."""
    frame = under(character, behind)
    frame = over(frame, front)
    solid = character[:, :, 3] == 255
    frame[solid] = character[solid]
    return frame


def spin_ball(ball: np.ndarray, deg: float, extra_dx: int, ground_y: int) -> np.ndarray:
    mask = body_mask(ball, 80)
    ys, xs = np.where(mask)
    cx, cy = float(xs.mean()), float(ys.mean())
    matrix = cv2.getRotationMatrix2D((cx, cy), deg, 1.0)
    rotated = cv2.warpAffine(
        ball,
        matrix,
        (W, H),
        flags=cv2.INTER_LINEAR,
        borderMode=cv2.BORDER_CONSTANT,
        borderValue=(0, 0, 0, 0),
    )
    mask2 = body_mask(rotated, 40)
    y1 = int(np.where(mask2)[0].max())
    return finish(shift(rotated, extra_dx, ground_y - y1))


def build_fan(cm: dict, canon: np.ndarray) -> list[np.ndarray]:
    """Straight horizontal throw toward screen-left, the way the character faces."""
    throw = load_standing("skill-throw-straight.jpg", cm)
    x1 = bbox_of(body_mask(throw, 40))[2]
    shift_x = max(0, (W - 12) - int(x1))
    pose = shift(throw, shift_x, 0)
    hand = throwing_hand(pose)
    # Constant height. The fan leaves the forward hand and keeps going left.
    y = hand[1]
    start_x = hand[0] - 22
    end_x = -36
    path = [(int(round(x)), y) for x in np.linspace(start_x, end_x, 8)]
    if len({p[1] for p in path}) != 1:
        raise SystemExit(f"fan path is not horizontal: {path}")
    if any(path[i][0] >= path[i - 1][0] for i in range(1, len(path))):
        raise SystemExit(f"fan path does not travel forward: {path}")
    print(f"fan hand={hand} shift={shift_x} path={path}", flush=True)
    frames = []
    for i, (fx, fy) in enumerate(path):
        # A little tumble, still a fan. The center stays on the horizontal line.
        spin = -12 - i * 4
        frames.append(paste_center(pose, fan_sprite(spin, 1.05), fx, fy))
    # Impact continues that same line, just off the left edge. No enemy.
    for stage in (0, 1, 2, 3):
        boom = under(pose, explosion(stage, end_x + 16, y))
        assert_character_intact(pose, boom, f"fan burst {stage}")
        frames.append(boom)
    return frames


def build_roll(cm: dict) -> list[np.ndarray]:
    """Curl, then roll only forward (screen-left) with no-slip rotation."""
    tuck = place_shorter("skill-roll-tuck.jpg", cm, TUCK_HEIGHT)
    ball = place_shorter("skill-roll-ball.jpg", cm, 0.46)
    mask = body_mask(ball, 80)
    ys, xs = np.where(mask)
    x0, y0, y1 = int(xs.min()), int(ys.min()), int(ys.max())
    ground_y = y1
    radius = max((y1 - y0) / 2.0, 1.0)
    tuck_x0 = int(np.where(body_mask(tuck, 40))[1].min())
    # Stop before either sprite clips the left edge.
    travel = min(10 - x0, 10 - tuck_x0)
    if travel >= -8:
        raise SystemExit(f"roll has no forward room, travel={travel}")
    steps = 9
    frames = [tuck]
    for i in range(steps):
        dx = int(round(travel * (i / (steps - 1))))
        # Moving left: positive angle sends the top of the ball left with the roll.
        # The sprite is taller than the belly it rolls on, so the contact radius
        # is smaller than half the body height.
        deg = -dx / (radius * 0.36) * (180.0 / np.pi)
        frames.append(spin_ball(ball, deg, dx, ground_y))
    frames.append(shift(tuck, travel, 0))
    print(
        f"roll travel={travel}px radius={radius:.1f} final_deg={-travel / (radius * 0.36) * (180.0 / np.pi):.1f}",
        flush=True,
    )
    return frames


def build_power(canon: np.ndarray) -> list[np.ndarray]:
    frame_n = 12
    frames = []
    for i in range(frame_n):
        behind, front = fire_layers(canon, i, frame_n)
        frame = lick(canon, behind, front)
        assert_character_intact(canon, frame, f"power {i}")
        frames.append(frame)
    return frames


def write_manifest(directory: Path, name: str, note_frames: list[tuple[int, float, float]], loop: bool = False):
    frames = []
    blob = bytearray()
    for path in sorted(directory.glob("*.png")):
        data = path.read_bytes()
        blob += data
        frames.append({"file": path.name, "sha256": hashlib.sha256(data).hexdigest()})
    keys = []
    for frame, x, y in note_frames:
        if keys and keys[-1]["frame"] == frame:
            continue
        keys.append({"frame": frame, "x": round(float(x), 2), "y": round(float(y), 2)})
    if len(keys) == 1 and frames:
        keys.append({"frame": len(frames) - 1, "x": keys[0]["x"], "y": keys[0]["y"]})
    anchor = keys[0]
    manifest = {
        "name": name,
        "emotion": "",
        "skill": name,
        "kind": "skill",
        "fps": FPS,
        "frameCount": len(frames),
        "loop": loop,
        "priority": 40,
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
    return len(frames)


def mouths_for(canon: np.ndarray, frames: list[np.ndarray]) -> list[tuple[int, float, float]]:
    hx, hy = head_anchor(canon)
    out = [(0, 153.5, 138.0)]
    for i, frame in enumerate(frames, start=1):
        # Canon-backed frames keep the reference mouth. Drawn poses track the head.
        if frame is canon or np.shares_memory(frame, canon):
            out.append((i, 153.5, 138.0))
            continue
        ax, ay = head_anchor(frame)
        out.append((i, 153.5 + (ax - hx), 138.0 + (ay - hy)))
    return out


def render_clip(name: str, plates: list[np.ndarray], canon: np.ndarray, canon_bytes: bytes, dest_root: Path, loop: bool = False):
    dest = dest_root / name
    if dest.exists():
        shutil.rmtree(dest)
    dest.mkdir(parents=True)
    (dest / "0000.png").write_bytes(canon_bytes)
    saved = []
    for i, plate in enumerate(plates, start=1):
        path = dest / f"{i:04d}.png"
        # The closing frame of throw and roll is the plate again.
        if plate is None:
            path.write_bytes(canon_bytes)
            saved.append(canon)
        else:
            save_png(plate, path)
            saved.append(plate)
    notes = mouths_for(canon, saved)
    count = write_manifest(dest, name, notes, loop=loop)
    # Frame 0 is not inside `saved`, so the manifest count includes it.
    print(f"{name}: {count} frames", flush=True)
    return count


def assert_character_intact(canon: np.ndarray, frame: np.ndarray, label: str):
    solid = canon[:, :, 3] == 255
    if not np.array_equal(frame[:, :, :3][solid], canon[:, :, :3][solid]):
        raise SystemExit(f"{label} repainted opaque character pixels")
    if not np.array_equal(frame[:, :, 3][solid], canon[:, :, 3][solid]):
        raise SystemExit(f"{label} changed opaque character alpha")


def write_skills_json(counts: dict[str, int]):
    data = {
        "skills": [
            {
                "id": "skill_fan_throw",
                "zh": "掷扇",
                "clip": "animations/skill_fan_throw",
                "fps": FPS,
                "frame_count": counts["skill_fan_throw"],
                "note": "The arm thrusts straight forward, the way the character faces. The fan flies in a straight horizontal line and detonates off the left edge. No enemy is drawn. The clip ends on the reference plate.",
            },
            {
                "id": "skill_roll",
                "zh": "翻滚",
                "clip": "animations/skill_roll",
                "fps": FPS,
                "frame_count": counts["skill_roll"],
                "note": "Curls into a ball and rolls forward only, screen-left, with rotation matched to that travel, then uncurls onto the reference plate.",
            },
            {
                "id": "skill_power_up",
                "zh": "蓄力",
                "clip": "animations/skill_power_up",
                "fps": FPS,
                "frame_count": counts["skill_power_up"],
                "note": "Layered fire tongues, additive glow, rising embers, and a soft ground pool sit around the reference plate. The burn cycle loops. Opaque character pixels are not repainted.",
            },
        ]
    }
    SKILLS_PATH.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def on_white(im: Image.Image) -> Image.Image:
    bg = Image.new("RGBA", im.size, (255, 255, 255, 255))
    bg.alpha_composite(im.convert("RGBA"))
    return bg.convert("RGB")


def write_sheets(dest_root: Path):
    ids = ["skill_fan_throw", "skill_roll", "skill_power_up"]
    cell_w, cell_h = 140, 190
    contact = Image.new("RGB", (cell_w * 4, cell_h), (244, 244, 246))
    draw = ImageDraw.Draw(contact)
    labels = [("reference", CANON_PNG)]
    for skill_id in ids:
        labels.append((skill_id, dest_root / skill_id / "0004.png"))
    for i, (label, path) in enumerate(labels):
        im = on_white(Image.open(path))
        im.thumbnail((cell_w - 8, cell_h - 22))
        contact.paste(im, (i * cell_w + (cell_w - im.size[0]) // 2, 2))
        draw.text((i * cell_w + 4, cell_h - 16), label, fill=(20, 20, 20))
    motion_h = cell_h
    # One row per skill, every frame.
    widths = []
    rows = []
    for skill_id in ids:
        frames = sorted((dest_root / skill_id).glob("*.png"))
        rows.append((skill_id, frames))
        widths.append(len(frames))
    motion = Image.new("RGB", (max(widths) * 110 + 150, len(rows) * motion_h), (244, 244, 246))
    mdraw = ImageDraw.Draw(motion)
    for r, (skill_id, frames) in enumerate(rows):
        mdraw.text((4, r * motion_h + 8), skill_id, fill=(20, 20, 20))
        for c, path in enumerate(frames):
            im = on_white(Image.open(path))
            im.thumbnail((100, motion_h - 8))
            motion.paste(im, (150 + c * 110, r * motion_h + (motion_h - im.size[1]) // 2))
    for path, image in (
        (ROOT / "source" / "emotion" / "skill-contact-sheet.png", contact),
        (Path("/opt/cursor/artifacts/skill-contact-sheet.png"), contact),
        (ROOT / "source" / "emotion" / "skill-motion-sheet.png", motion),
        (Path("/opt/cursor/artifacts/skill-motion-sheet.png"), motion),
    ):
        path.parent.mkdir(parents=True, exist_ok=True)
        image.save(path, quality=90)
        print("wrote", path)


def write_gifs(dest_root: Path):
    out = Path("/opt/cursor/artifacts/gifs")
    out.mkdir(parents=True, exist_ok=True)
    for skill_id in ("skill_fan_throw", "skill_roll", "skill_power_up"):
        paths = sorted((dest_root / skill_id).glob("*.png"))
        # The burn is a closed cycle. Skip the locked plate so the GIF loops the fire.
        if skill_id == "skill_power_up" and len(paths) > 1:
            paths = paths[1:]
        frames = [on_white(Image.open(p)) for p in paths]
        unique = [frames[0]]
        durations = [int(1000 / FPS)]
        for frame in frames[1:]:
            if frame.tobytes() == unique[-1].tobytes():
                durations[-1] += int(1000 / FPS)
            else:
                unique.append(frame)
                durations.append(int(1000 / FPS))
        gif = out / f"{skill_id}.gif"
        unique[0].save(gif, save_all=True, append_images=unique[1:], duration=durations, loop=0, disposal=2)
        print("wrote", gif, "frames", len(unique))


def main(argv: list[str]):
    canon = np.array(Image.open(CANON_PNG).convert("RGBA"))
    canon_bytes = CANON_PNG.read_bytes()
    cm = canon_metrics(canon)
    dest = Path("/tmp/skill-anim") if argv[1:] == ["pilot"] else ANIM
    fan = build_fan(cm, canon)
    roll = build_roll(cm)
    power = build_power(canon)
    counts = {}
    counts["skill_fan_throw"] = render_clip("skill_fan_throw", fan + [None], canon, canon_bytes, dest)
    counts["skill_roll"] = render_clip("skill_roll", roll + [None], canon, canon_bytes, dest)
    counts["skill_power_up"] = render_clip("skill_power_up", power, canon, canon_bytes, dest, loop=True)
    if dest == ANIM:
        write_skills_json(counts)
        total = sum(p.stat().st_size for p in ANIM.rglob("*.png"))
        print(f"library png bytes {total} ({total / 1024 / 1024:.2f} MiB)")
        cap = 125 * 1024 * 1024
        if total > cap:
            raise SystemExit(f"frame bytes {total} exceed {cap}")
    write_sheets(dest)
    write_gifs(dest)


if __name__ == "__main__":
    main(sys.argv)
