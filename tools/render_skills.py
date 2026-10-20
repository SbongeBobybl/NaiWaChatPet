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


def fire_layer(character: np.ndarray, intensity: float, frame_i: int) -> np.ndarray:
    """Flames and glow outside the silhouette. Interior gaps stay empty."""
    mask = (character[:, :, 3] > 80).astype(np.uint8) * 255
    # Close the gap between the legs so the aura rings the body instead of
    # pooling into a disc under the belly.
    closed = cv2.morphologyEx(mask, cv2.MORPH_CLOSE, cv2.getStructuringElement(cv2.MORPH_ELLIPSE, (47, 47)))
    contours, _ = cv2.findContours(closed, cv2.RETR_EXTERNAL, cv2.CHAIN_APPROX_NONE)
    contour = max(contours, key=cv2.contourArea)
    filled = np.zeros_like(mask)
    cv2.drawContours(filled, [contour], -1, 255, thickness=-1)
    ring = cv2.dilate(filled, cv2.getStructuringElement(cv2.MORPH_ELLIPSE, (29, 29)))
    ring[filled > 0] = 0
    glow = cv2.GaussianBlur(ring, (0, 0), 12).astype(np.float32) / 255.0
    alpha = np.clip(glow * (80 + 140 * intensity), 0, 176)
    alpha = (alpha // 8) * 8
    alpha[filled > 0] = 0
    layer = np.zeros((H, W, 4), np.uint8)
    layer[:, :, 0] = 255
    layer[:, :, 1] = 92
    layer[:, :, 2] = 18
    layer[:, :, 3] = alpha.astype(np.uint8)

    pts = contour[:, 0, :]
    cx, cy = float(pts[:, 0].mean()), float(pts[:, 1].mean())
    step = max(1, len(pts) // 14)
    flames = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    draw = ImageDraw.Draw(flames)
    for n, idx in enumerate(range(0, len(pts), step)):
        x, y = float(pts[idx][0]), float(pts[idx][1])
        direction = np.array([x - cx, (y - cy) * 0.55 - 28.0], np.float32)
        norm = float(np.linalg.norm(direction))
        if norm < 8:
            continue
        direction /= norm
        if y > GROUND - 10 and direction[1] > 0.05:
            continue
        flicker = 0.72 + 0.28 * np.sin(frame_i * 1.15 + n * 1.3)
        length = (26 + 62 * intensity) * flicker
        width = 10 + 7 * intensity
        perp = np.array([-direction[1], direction[0]])
        outer = []
        inner_pts = []
        for s in range(8):
            t = s / 7
            bulge = np.sin(t * np.pi) ** 0.55
            rad = width * (1.0 - t) * (0.35 + 0.75 * bulge)
            outer.append((x + direction[0] * length * t + perp[0] * rad, y + direction[1] * length * t + perp[1] * rad))
            inner_pts.append(
                (x + direction[0] * length * t * 0.82 + perp[0] * rad * 0.38, y + direction[1] * length * t * 0.82 + perp[1] * rad * 0.38)
            )
        outer.append((x + direction[0] * length, y + direction[1] * length))
        for s in range(7, -1, -1):
            t = s / 7
            bulge = np.sin(t * np.pi) ** 0.55
            rad = width * (1.0 - t) * (0.35 + 0.75 * bulge)
            outer.append((x + direction[0] * length * t - perp[0] * rad, y + direction[1] * length * t - perp[1] * rad))
            inner_pts.append(
                (x + direction[0] * length * t * 0.82 - perp[0] * rad * 0.38, y + direction[1] * length * t * 0.82 - perp[1] * rad * 0.38)
            )
        draw.polygon(outer, fill=(255, 96, 16, int(165 + 70 * intensity)))
        draw.polygon(inner_pts, fill=(255, 228, 140, int(150 + 80 * intensity)))
    flame_arr = np.array(flames)
    flame_arr[filled > 0] = 0
    rng = np.random.default_rng(20 + frame_i)
    embers = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    edraw = ImageDraw.Draw(embers)
    for _ in range(int(10 * intensity)):
        ex = int(rng.integers(30, W - 30))
        ey = int(rng.integers(60, GROUND - 16))
        if filled[ey, ex] > 0 or character[ey, ex, 3] > 16:
            continue
        rad = int(rng.integers(2, 4))
        edraw.ellipse([ex - rad, ey - rad, ex + rad, ey + rad], fill=(255, 196, 70, 190))
    return over(over(layer, flame_arr), np.array(embers))


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
    windup = load_standing("skill-throw-windup.jpg", cm)
    release = load_standing("skill-throw-release.jpg", cm)
    follow = load_standing("skill-throw-follow.jpg", cm)
    hand_w = throwing_hand(windup)
    hand_r = throwing_hand(release)
    print(f"throwing hands windup={hand_w} release={hand_r}", flush=True)
    # Held in the raised hand, then an arc through the open air to an unseen
    # target off the upper-left corner. The last point still clips the frame.
    hold = (hand_w[0] - 14, hand_w[1] - 8)
    exit_x, exit_y = -28, 22
    path = [hold, (34, 124), (20, 82), (6, 50), (2, 34)]
    print(f"fan path {path}", flush=True)
    frames = []
    poses = [windup, release, release, follow, follow]
    spins = [-24, 15, 55, 110, 170]
    scales = [0.9, 0.95, 1.0, 1.05, 1.1]
    for pose, (fx, fy), spin, scale in zip(poses, path, spins, scales):
        frames.append(paste_center(pose, fan_sprite(spin, scale), fx, fy))
    # The hit is off-canvas. The flash, streaks, and smoke come back into frame.
    for stage in (0, 1, 2, 3):
        frames.append(under(canon, explosion(stage, exit_x, exit_y)))
    return frames


def build_roll(cm: dict) -> list[np.ndarray]:
    tuck = place_shorter("skill-roll-tuck.jpg", cm, TUCK_HEIGHT)
    ball = place_shorter("skill-roll-ball.jpg", cm, BALL_HEIGHT)
    ground_y = int(np.where(body_mask(ball, 80))[0].max())
    spins = [
        (0, 0),
        (-60, -22),
        (-120, -44),
        (-180, -66),
        (-240, -66),
        (-300, -44),
        (-360, -22),
        (-400, 0),
    ]
    frames = [tuck]
    for deg, dx in spins:
        frames.append(spin_ball(ball, deg, dx, ground_y))
    frames.append(tuck)
    return frames


def build_power(canon: np.ndarray) -> list[np.ndarray]:
    levels = (0.22, 0.4, 0.58, 0.74, 0.88, 1.0, 1.0, 1.0)
    frames = []
    for i, level in enumerate(levels, start=1):
        frames.append(under(canon, fire_layer(canon, level, i)))
    return frames


def write_manifest(directory: Path, name: str, note_frames: list[tuple[int, float, float]]):
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
        "loop": False,
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


def render_clip(name: str, plates: list[np.ndarray], canon: np.ndarray, canon_bytes: bytes, dest_root: Path):
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
    count = write_manifest(dest, name, notes)
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
                "note": "Throws a fan off the left edge. The burst is the impact on an unseen target. No enemy is drawn. The character winds up, releases, and returns to the reference plate.",
            },
            {
                "id": "skill_roll",
                "zh": "翻滚",
                "clip": "animations/skill_roll",
                "fps": FPS,
                "frame_count": counts["skill_roll"],
                "note": "Curls into a ball, rolls by rotating that pose, then uncurls onto the reference plate.",
            },
            {
                "id": "skill_power_up",
                "zh": "蓄力",
                "clip": "animations/skill_power_up",
                "fps": FPS,
                "frame_count": counts["skill_power_up"],
                "note": "Fire and glow are layers behind the reference plate. Opaque character pixels are not repainted.",
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
        frames = [on_white(Image.open(p)) for p in sorted((dest_root / skill_id).glob("*.png"))]
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
    for i, frame in enumerate(power, start=1):
        assert_character_intact(canon, frame, f"power {i}")
    for i, frame in enumerate(fan[5:], start=6):
        assert_character_intact(canon, frame, f"fan burst {i}")
    counts = {}
    counts["skill_fan_throw"] = render_clip("skill_fan_throw", fan + [None], canon, canon_bytes, dest)
    counts["skill_roll"] = render_clip("skill_roll", roll + [None], canon, canon_bytes, dest)
    counts["skill_power_up"] = render_clip("skill_power_up", power, canon, canon_bytes, dest)
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
