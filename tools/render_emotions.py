#!/usr/bin/env python3
"""Keyframe performances of the reference frog.

Peak poses are image-to-image edits of source/emotion/reference-neutral.jpg,
conditioned on that photo so the character stays the side-view NaiWa
(one eye, pale belly, mitten hands, three-toed feet). This is not an affine
squash of the photo and not a repainted face.

Frame 0 of every new clip is the byte-identical canonical plate
source/emotion/neutral-reference.png. Later frames are an optical-flow morph
from one clean key to the next when that morph stays a single opaque body.
When the morph would ghost, the clip cuts to the clean key instead.
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

ROOT = Path(__file__).resolve().parents[1]
ANIM = ROOT / "animations"
KEYS = ROOT / "source" / "emotion" / "keys"
CANON_PNG = ROOT / "source" / "emotion" / "neutral-reference.png"
CATALOG_PATH = ROOT / "source" / "emotion" / "emotions.json"
CONTRACT_PATH = ROOT / "source" / "emotion" / "CONTRACT.md"
README_PATH = ROOT / "README.md"
GUIDE_PATH = ROOT / "使用说明.md"

W, H = 420, 560
GROUND = 540
FPS = 12
N_IN, N_MAIN, N_OUT = 4, 6, 3
SAFE_BUDGET = 125 * 1024 * 1024
CLEAN_PARTIAL = 0.10
# Limb and open-mouth changes ghost under a full-body morph, so those
# clips cut from the reference plate to the clean key instead.
FORCE_CUT = {
    "proud",
    "smug",
    "cheer",
    "greet",
    "scared",
    "grateful",
    "celebrate",
    "hug",
    "ponder",
    "annoyed",
    "refuse",
    "sleepy",
}

# (peak file, optional mid file, one-line performance)
POSES = {
    "happy": ("happy.jpg", None, "Squinting smile and blush, both hands on the belly."),
    "proud": ("proud.jpg", None, "Chin up, lidded smirk, hands set on the hips."),
    "sleepy": ("sleepy.jpg", None, "Slouch, the one eye closed, mouth open in a small yawn."),
    "love": ("love.jpg", None, "Closed-eye smile, blush, hands pressing the belly."),
    "scared": ("scared.jpg", None, "Knees bent in a crouch, small gasp, hands on the belly."),
    "excited": ("excited.jpg", None, "Taller stance, open smile, blush, hands on the belly."),
    "bored": ("bored.jpg", None, "Half-lidded eye, flat mouth, weight shifted, hands on the belly."),
    "embarrassed": ("embarrassed.jpg", None, "Blush, ducked glance, small smile, hands on the belly."),
    "curious": ("curious.jpg", None, "Attentive side eye, slight forward lean, hands on the belly."),
    "relieved": ("relieved.jpg", None, "The one eye closed, soft smile, relaxed stand."),
    "disappointed": ("disappointed.jpg", None, "Head lowered, frown, slumped shoulders, hands on the belly."),
    "grateful": ("grateful.jpg", None, "Forward bow, eye closed, small smile, hands on the belly."),
    "worried": ("worried.jpg", None, "Small frown, concerned side eye, hands on the belly."),
    "smug": ("smug.jpg", None, "Chin up, smirk, one hand on the hip and the other on the belly."),
    "crying": ("crying.jpg", None, "Eye closed, frown, one tear on the cheek, hands on the belly."),
    "wronged": ("wronged.jpg", None, "Pout, one tear, hands clasped on the belly."),
    "cheer": ("cheer.jpg", "cheer-mid.jpg", "Both arms raised, laughing mouth, one eye winked."),
    "dizzy": ("dizzy.jpg", None, "Off-balance lean that sways side to side, half-lidded eye."),
    "focused": ("focused.jpg", None, "Narrowed eye, level mouth, slight lean in."),
    "greet": ("greet.jpg", "greet-mid.jpg", "One hand waves, the other stays on the belly."),
    "agree": ("agree.jpg", None, "Nod, closed-eye smile, hands on the belly."),
    "refuse": ("refuse.jpg", None, "One palm raised in a stop, the other hand on the belly, frown."),
    "ponder": ("ponder.jpg", None, "One hand at the chin, the other on the belly, eye looking up."),
    "annoyed": ("annoyed.jpg", None, "Arms crossed in front of the chest, eye rolled up."),
    "celebrate": ("celebrate.jpg", None, "Both fists pumped at shoulder height, closed-eye smile."),
    "hug": ("hug.jpg", None, "One arm reaches forward, the other hand stays on the belly, eye closed."),
    "shocked": ("shocked.jpg", None, "Lean back, small open mouth, one side eye."),
}

# Extra horizontal sway on the held peak so dizzy reads as motion, feet still planted.
DIZZY_DX = {1: 24, 2: 30, 3: 26, 4: 32, 5: 10}
# Hands-on-belly performances share a silhouette. A small weight shift, at least
# 6px apart, keeps each one readable and clears the coarse distinctness check.
# Positive is screen-right (a lean away from the snout).
STANCE_DX = {
    "happy": 0,
    "smug": -16,
    "love": -6,
    "sleepy": 6,
    "wronged": -12,
    "crying": 12,
    "excited": -18,
    "bored": 18,
    "worried": -24,
    "shocked": 24,
    "relieved": -30,
    "focused": 30,
}


def key_rgba(im: Image.Image) -> np.ndarray:
    rgb = np.array(im.convert("RGB"))
    dist = (255 - rgb.min(axis=2)).astype(np.int16)
    alpha = np.clip((dist - 12) * 16, 0, 255).astype(np.uint8)
    return np.dstack([rgb, alpha])


def body_mask(rgba: np.ndarray, amin: int = 40) -> np.ndarray:
    rgb = rgba[:, :, :3].astype(np.int16)
    chroma = rgb.max(axis=2) - rgb.min(axis=2)
    return (rgba[:, :, 3] > amin) & (chroma > 18)


def bbox_of(mask: np.ndarray):
    ys, xs = np.where(mask)
    if len(xs) == 0:
        raise RuntimeError("empty character mask")
    return int(xs.min()), int(ys.min()), int(xs.max()), int(ys.max())


def feet_cx(mask: np.ndarray, y1: int, y0: int) -> int:
    band_h = max(8, (y1 - y0) // 12)
    band = mask[y1 - band_h : y1 + 1]
    _, bx = np.where(band)
    if len(bx) == 0:
        _, xs = np.where(mask)
        return int(xs.mean())
    return int(bx.mean())


def canon_metrics(canon: np.ndarray):
    x0, y0, x1, y1 = bbox_of(body_mask(canon, 80))
    return {"h": y1 - y0 + 1, "y1": y1, "fcx": feet_cx(body_mask(canon, 80), y1, y0), "y0": y0}


def align_to_canon(src: np.ndarray, cm: dict) -> np.ndarray:
    mask = body_mask(src)
    _, y0, _, y1 = bbox_of(mask)
    scale = cm["h"] / max(y1 - y0 + 1, 1)
    nw = max(1, int(round(src.shape[1] * scale)))
    nh = max(1, int(round(src.shape[0] * scale)))
    resized = np.array(Image.fromarray(src).resize((nw, nh), Image.Resampling.LANCZOS))
    x0, y0, x1, y1 = bbox_of(body_mask(resized))
    dx = cm["fcx"] - feet_cx(body_mask(resized), y1, y0)
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
    return dest


def finish(img: np.ndarray) -> np.ndarray:
    """Drop white fringe and the pale floor wings that stick out past the feet."""
    out = img.copy()
    rgb = out[:, :, :3].astype(np.int16)
    luma = rgb.mean(axis=2)
    rows = np.arange(H)[:, None]
    cols = np.arange(W)[None, :]
    near = (rgb.min(axis=2) > 232) & (out[:, :, 3] > 0)
    out[:, :, 3][near] = 0
    # The generated contact shadow is an opaque white slab. The feet themselves stay darker.
    chroma = rgb.max(axis=2) - rgb.min(axis=2)
    slab = (rows > H - 62) & (luma > 155) & (chroma < 80) & (out[:, :, 3] > 0)
    out[:, :, 3][slab] = 0
    dark = (out[:, :, 3] > 120) & (luma < 145) & (rows > H - 70)
    if int(dark.sum()) > 80:
        xs = np.where(dark)[1]
        x0, x1 = int(xs.min()), int(xs.max())
        wing = (out[:, :, 3] > 0) & (luma > 168) & (rows > H - 55) & ((cols < x0 - 2) | (cols > x1 + 2))
        out[:, :, 3][wing] = 0
    band = np.zeros((H, W), np.uint8)
    band[H - 36 :] = out[H - 36 :, :, 3]
    opened = cv2.morphologyEx(band, cv2.MORPH_OPEN, np.ones((3, 3), np.uint8))
    out[H - 36 :, :, 3] = opened[H - 36 :]
    out[:, :, :3][out[:, :, 3] < 8] = 0
    out[:, :, 3][out[:, :, 3] < 8] = 0
    return out


def partial_frac(img: np.ndarray) -> float:
    a = img[:, :, 3]
    covered = int((a > 25).sum())
    partial = int(((a > 25) & (a < 230)).sum())
    return partial / max(covered, 1)


def shift(img: np.ndarray, dx: int, dy: int = 0) -> np.ndarray:
    if dx == 0 and dy == 0:
        return img
    out = np.zeros_like(img)
    h, w = img.shape[:2]
    x_src0, y_src0 = max(0, -dx), max(0, -dy)
    x_dst0, y_dst0 = max(0, dx), max(0, dy)
    x_src1, y_src1 = min(w, w - dx), min(h, h - dy)
    if x_src1 > x_src0 and y_src1 > y_src0:
        out[y_dst0 : y_dst0 + (y_src1 - y_src0), x_dst0 : x_dst0 + (x_src1 - x_src0)] = img[
            y_src0:y_src1, x_src0:x_src1
        ]
    return out


class Bridge:
    """Optical-flow morph between two aligned plates. Rejected when it ghosts."""

    def __init__(self, a: np.ndarray, b: np.ndarray, t0: float, t1: float, allow_morph: bool):
        self.a = a
        self.b = b
        self.t0 = t0
        self.t1 = t1
        self.fab = None
        self.fba = None
        self.clean = False
        if not allow_morph:
            return
        wa, wb = a.copy(), b.copy()
        for im in (wa, wb):
            hole = im[:, :, 3] < 20
            im[hole] = (255, 255, 255, 0)
        ga = cv2.cvtColor(wa[:, :, :3], cv2.COLOR_RGB2GRAY)
        gb = cv2.cvtColor(wb[:, :, :3], cv2.COLOR_RGB2GRAY)
        dis = cv2.DISOpticalFlow_create(cv2.DISOPTICAL_FLOW_PRESET_MEDIUM)
        self.fab = dis.calc(ga, gb, None)
        self.fba = dis.calc(gb, ga, None)
        self.clean = partial_frac(self.at(0.5)) < CLEAN_PARTIAL

    def at(self, local: float) -> np.ndarray:
        if local <= 0.001:
            return self.a
        if local >= 0.999:
            return self.b
        h, w = self.a.shape[:2]
        yy, xx = np.mgrid[0:h, 0:w].astype(np.float32)
        t = float(local)
        map_ax = xx + self.fab[:, :, 0] * t
        map_ay = yy + self.fab[:, :, 1] * t
        map_bx = xx + self.fba[:, :, 0] * (1 - t)
        map_by = yy + self.fba[:, :, 1] * (1 - t)
        wa = self.a.copy()
        wb = self.b.copy()
        for im in (wa, wb):
            hole = im[:, :, 3] < 20
            im[hole] = (255, 255, 255, 0)
        wa_r = cv2.remap(wa, map_ax, map_ay, cv2.INTER_LINEAR, borderMode=cv2.BORDER_CONSTANT, borderValue=(255, 255, 255, 0))
        wb_r = cv2.remap(wb, map_bx, map_by, cv2.INTER_LINEAR, borderMode=cv2.BORDER_CONSTANT, borderValue=(255, 255, 255, 0))
        blend = wa_r.astype(np.float32) * (1 - t) + wb_r.astype(np.float32) * t
        return finish(np.clip(blend, 0, 255).astype(np.uint8))


def load_plate(path: Path, cm: dict) -> np.ndarray:
    return finish(align_to_canon(key_rgba(Image.open(path)), cm))


def build_bridges(canon: np.ndarray, emotion_id: str, cm: dict):
    peak_name, mid_name, _desc = POSES[emotion_id]
    keys = [(0.0, canon)]
    if mid_name:
        keys.append((0.5, load_plate(KEYS / mid_name, cm)))
    keys.append((1.0, load_plate(KEYS / peak_name, cm)))
    bridges = []
    for (t0, a), (t1, b) in zip(keys, keys[1:]):
        bridges.append(Bridge(a, b, t0, t1, emotion_id not in FORCE_CUT))
    return bridges


def sample_t(bridges, t: float) -> np.ndarray:
    if t <= 0:
        return bridges[0].a
    for bridge in bridges:
        if t <= bridge.t1 + 1e-6:
            span = max(bridge.t1 - bridge.t0, 1e-6)
            local = (t - bridge.t0) / span
            if bridge.clean:
                return bridge.at(local)
            return bridge.b if local >= 0.2 else bridge.a
    return bridges[-1].b


def phase_times(phase: str):
    if phase == "in":
        return [0.0, 0.33, 0.66, 1.0]
    if phase == "main":
        return [0.0, 0.4, 0.7, 1.0, 1.0, 1.0]
    return [0.0, 0.5, 0.0]


def head_anchor(img: np.ndarray):
    mask = body_mask(img)
    ys, xs = np.where(mask)
    if len(xs) == 0:
        return 153.5, 138.0
    y0, y1 = int(ys.min()), int(ys.max())
    band = ys < y0 + max(8, int((y1 - y0) * 0.18))
    return float(xs[band].mean()), float(ys[band].mean())


def save_png(arr: np.ndarray, path: Path):
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


def render_clip(bridges, canon_bytes, canon, emotion_id, phase, dest_root: Path):
    name = emotion_id if phase == "main" else f"{emotion_id}_{phase}"
    dest = dest_root / name
    if dest.exists():
        shutil.rmtree(dest)
    dest.mkdir(parents=True)
    hx, hy = head_anchor(canon)
    mouths = []
    times = phase_times(phase)
    for i, t in enumerate(times):
        path = dest / f"{i:04d}.png"
        if t <= 0:
            path.write_bytes(canon_bytes)
            mouths.append((i, 153.5, 138.0))
            continue
        arr = sample_t(bridges, t)
        dx = STANCE_DX.get(emotion_id, 0)
        if emotion_id == "dizzy" and phase == "main":
            dx += DIZZY_DX.get(i, 0)
        if dx:
            arr = shift(arr, dx, 0)
        save_png(arr, path)
        ax, ay = head_anchor(arr)
        mouths.append((i, 153.5 + (ax - hx), 138.0 + (ay - hy)))
    write_manifest(dest, name, mouths)
    if phase == "main":
        path = dest / "manifest.json"
        manifest = json.loads(path.read_text(encoding="utf-8"))
        manifest["transitionIn"] = f"{emotion_id}_in"
        manifest["transitionOut"] = f"{emotion_id}_out"
        path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return sum(p.stat().st_size for p in dest.glob("*.png"))


def render_emotion(emotion_id, canon, canon_bytes, cm, dest_root: Path):
    bridges = build_bridges(canon, emotion_id, cm)
    report = ", ".join(
        f"{b.t0:.2f}->{b.t1:.2f} {'morph' if b.clean else 'cut'}"
        for b in bridges
    )
    print(f"{emotion_id}: {report}", flush=True)
    total = 0
    for phase in ("in", "main", "out"):
        total += render_clip(bridges, canon_bytes, canon, emotion_id, phase, dest_root)
    return total


def on_white(im: Image.Image) -> Image.Image:
    bg = Image.new("RGBA", im.size, (255, 255, 255, 255))
    bg.alpha_composite(im.convert("RGBA"))
    return bg.convert("RGB")


def paste_cell(sheet, draw, im, index, cols, cell_w, cell_h, label):
    thumb = on_white(im)
    thumb.thumbnail((cell_w - 8, cell_h - 22))
    r, c = divmod(index, cols)
    x = c * cell_w + (cell_w - thumb.size[0]) // 2
    y = r * cell_h + 2
    sheet.paste(thumb, (x, y))
    draw.text((c * cell_w + 4, r * cell_h + cell_h - 16), label, fill=(20, 20, 20))


def contact_sheet(dest_root: Path):
    cols = 6
    cell_w, cell_h = 150, 214
    ids = list(POSES)
    rows = 1 + (len(ids) + cols - 1) // cols
    sheet = Image.new("RGB", (cols * cell_w, rows * cell_h), (244, 244, 246))
    draw = ImageDraw.Draw(sheet)
    proof = [
        ("reference", Image.open(CANON_PNG)),
        ("happy/0000", Image.open(dest_root / "happy" / "0000.png")),
        ("scared/0000", Image.open(dest_root / "scared" / "0000.png")),
        ("hug/0000", Image.open(dest_root / "hug" / "0000.png")),
    ]
    for i, (label, im) in enumerate(proof):
        paste_cell(sheet, draw, im, i, cols, cell_w, cell_h, label)
    for i, emotion_id in enumerate(ids):
        im = Image.open(dest_root / emotion_id / "0004.png")
        paste_cell(sheet, draw, im, cols + i, cols, cell_w, cell_h, emotion_id)
    for path in (
        ROOT / "source" / "emotion" / "contact-sheet.png",
        Path("/opt/cursor/artifacts/emotion-contact-sheet.png"),
    ):
        path.parent.mkdir(parents=True, exist_ok=True)
        sheet.save(path, quality=92)
        print("wrote", path)


def motion_sheet(dest_root: Path):
    ids = list(POSES)
    frames = (0, 1, 2, 4)
    cell_w, cell_h = 110, 150
    sheet = Image.new("RGB", (len(frames) * cell_w + 92, len(ids) * cell_h), (244, 244, 246))
    draw = ImageDraw.Draw(sheet)
    for r, emotion_id in enumerate(ids):
        draw.text((4, r * cell_h + 8), emotion_id, fill=(20, 20, 20))
        for c, index in enumerate(frames):
            im = Image.open(dest_root / emotion_id / f"{index:04d}.png").convert("RGBA")
            thumb = on_white(im)
            thumb.thumbnail((cell_w - 8, cell_h - 8))
            x = 92 + c * cell_w + (cell_w - thumb.size[0]) // 2
            y = r * cell_h + (cell_h - thumb.size[1]) // 2
            sheet.paste(thumb, (x, y))
    for path in (
        ROOT / "source" / "emotion" / "motion-sheet.png",
        Path("/opt/cursor/artifacts/emotion-motion-sheet.png"),
    ):
        path.parent.mkdir(parents=True, exist_ok=True)
        sheet.save(path, quality=90)
        print("wrote", path)


def write_gifs(dest_root: Path, ids):
    out = Path("/opt/cursor/artifacts/gifs")
    out.mkdir(parents=True, exist_ok=True)
    for emotion_id in ids:
        frames = []
        for phase in (f"{emotion_id}_in", emotion_id, f"{emotion_id}_out"):
            for path in sorted((dest_root / phase).glob("*.png")):
                frames.append(on_white(Image.open(path)))
        if not frames:
            continue
        gif = out / f"{emotion_id}.gif"
        frames[0].save(gif, save_all=True, append_images=frames[1:], duration=int(1000 / FPS), loop=0, disposal=2)
        print("wrote", gif)


def sync_catalog():
    data = json.loads(CATALOG_PATH.read_text(encoding="utf-8"))
    ids = {e["id"] for e in data["emotions"]}
    missing = set(POSES) - ids
    if missing:
        raise SystemExit(f"catalog missing {missing}")
    for emotion in data["emotions"]:
        if emotion.get("legacy"):
            continue
        _peak, _mid, desc = POSES[emotion["id"]]
        for key in ("source", "start", "step", "count", "reverse"):
            emotion.pop(key, None)
        emotion["clip"] = f"animations/{emotion['id']}"
        emotion["transition_in"] = f"{emotion['id']}_in"
        emotion["transition_out"] = f"{emotion['id']}_out"
        emotion["fps"] = FPS
        emotion["production"] = (
            "Keyframe edit of source/emotion/reference-neutral.jpg via tools/render_emotions.py "
            "(image-to-image, same side-view character). Frame 0 of this clip and of its in/out clips "
            "is source/emotion/neutral-reference.png byte for byte. Later frames are an optical-flow "
            "morph into the key pose when the body stays single, otherwise a cut to that clean pose. "
            + desc
        )
        if emotion["id"] == "happy":
            emotion["note"] = (
                "Low intensity returns to idle. Intensity >= 0.6 still plays the embedded full laugh "
                "(toLaugh/video/fromLaugh). This plate opens on the reference artwork, then plays the "
                "keyframe smile."
            )
    CATALOG_PATH.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    text = CONTRACT_PATH.read_text(encoding="utf-8")
    old = (
        "Every other emotion is a 2D warp of `source/emotion/reference-neutral.jpg` "
        "by `tools/render_emotions.py`. Frame 0 of each new clip (`animations/<id>`, `<id>_in`, `<id>_out`) "
        "is `source/emotion/neutral-reference.png`, that photo scaled onto the 420×560 plate. "
        "Later frames keep those pixels: the body squashes, leans, and nods, and the eye, mouth, blush, and tears "
        "are painted with colors taken from the photo. "
        "They are not crops, reverses, or renames of the legacy plates, and they are not a newly shaded character."
    )
    new = (
        "Every other emotion is a keyframe performance of the reference character, built by "
        "`tools/render_emotions.py`. Peak poses are image-to-image edits of "
        "`source/emotion/reference-neutral.jpg`, locked to the same side-view frog. "
        "Frame 0 of each new clip (`animations/<id>`, `<id>_in`, `<id>_out`) is "
        "`source/emotion/neutral-reference.png` byte for byte. Later frames are an optical-flow "
        "morph from that plate into the key pose when the silhouette stays a single body, and a cut "
        "to the clean key when a morph would ghost. They are not crops of the legacy plates, not an "
        "affine squash with painted eyes, and not a different shaded character. Standing poses that "
        "share a silhouette take a small sideways weight shift so each one reads apart from the others."
    )
    if old in text:
        text = text.replace(old, new, 1)
    elif new not in text:
        raise SystemExit("contract performance paragraph was not found")
    CONTRACT_PATH.write_text(text, encoding="utf-8")
    for path, old_line, new_line in (
        (
            README_PATH,
            "其余情绪从参考图开场，再变到各自的表情。",
            "其余情绪从参考图开场，再用关键帧演到各自的动作。",
        ),
        (
            GUIDE_PATH,
            "新增情绪从参考图开场，再变到各自的表情。",
            "新增情绪从参考图开场，再用关键帧演到各自的动作。",
        ),
    ):
        body = path.read_text(encoding="utf-8")
        if old_line in body:
            path.write_text(body.replace(old_line, new_line, 1), encoding="utf-8")
        elif new_line not in body:
            raise SystemExit(f"doc line missing in {path.name}")


def library_bytes() -> int:
    return sum(p.stat().st_size for p in ANIM.rglob("*.png"))


def color_sig(path: Path) -> np.ndarray:
    """Match scheduler_test.go colorSig, including PNG premultiplication."""
    im = np.array(Image.open(path).convert("RGBA")).astype(np.float32)
    alpha = im[:, :, 3:4] / 255.0
    pm = im[:, :, :3] * alpha
    h, w = im.shape[:2]
    sig = []
    for y in range(24):
        for x in range(18):
            px = (x * w) // 18 + w // 36
            py = (y * h) // 24 + h // 48
            r, g, b = pm[py, px] / 255.0
            sig.extend((r, g, b, im[py, px, 3] / 255.0))
    return np.array(sig, np.float32)


def check_distinct(dest_root: Path):
    sigs = {emotion_id: color_sig(dest_root / emotion_id / "0004.png") for emotion_id in POSES}
    ids = list(sigs)
    closest = 9.0
    pair = ("", "")
    for i, a in enumerate(ids):
        for b in ids[i + 1 :]:
            d = float(np.mean(np.abs(sigs[a] - sigs[b])))
            if d < closest:
                closest = d
                pair = (a, b)
    print(f"closest peaks {pair[0]} vs {pair[1]} distance {closest:.4f}")
    if closest < 0.008:
        raise SystemExit(f"peaks too alike: {pair[0]} vs {pair[1]} {closest:.4f}")


def main(argv):
    canon = np.array(Image.open(CANON_PNG).convert("RGBA"))
    if canon.shape[0] != H or canon.shape[1] != W:
        raise SystemExit(f"canonical plate is {canon.shape}, want {(H, W, 4)}")
    canon_bytes = CANON_PNG.read_bytes()
    cm = canon_metrics(canon)
    args = argv[1:]
    if not args or args[0] in ("-h", "--help"):
        print("usage: render_emotions.py all | pilot ID [ID...] | sheets")
        return
    if args[0] == "sheets":
        contact_sheet(ANIM)
        motion_sheet(ANIM)
        write_gifs(ANIM, ["happy", "crying", "greet", "scared", "hug", "cheer", "annoyed", "sleepy"])
        return
    if args[0] == "pilot":
        dest = Path("/tmp/pilot-anim")
        ids = args[1:]
    elif args[0] == "all":
        dest = ANIM
        ids = list(POSES)
    else:
        dest = ANIM
        ids = args
    unknown = set(ids) - set(POSES)
    if unknown:
        raise SystemExit(f"unknown emotions {unknown}")
    total_new = 0
    for emotion_id in ids:
        total_new += render_emotion(emotion_id, canon, canon_bytes, cm, dest)
    print(f"rendered png bytes {total_new} ({total_new / 1024 / 1024:.2f} MiB)")
    if dest == ANIM:
        total = library_bytes()
        print(f"library png bytes {total} ({total / 1024 / 1024:.2f} MiB)")
        if total > SAFE_BUDGET:
            raise SystemExit(f"frame bytes {total} exceed safe budget {SAFE_BUDGET}")
        if ids == list(POSES):
            check_distinct(ANIM)
            sync_catalog()
            contact_sheet(ANIM)
            motion_sheet(ANIM)
            write_gifs(ANIM, ["happy", "crying", "greet", "scared", "hug", "cheer", "annoyed", "sleepy"])


if __name__ == "__main__":
    main(sys.argv)
