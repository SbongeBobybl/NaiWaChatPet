#!/usr/bin/env python3
"""Build the skeletal (Spine 4.2) version of the three skill clips.

    python3 tools/skeletal/build.py            # rig, atlas, animations, frame clips, previews
    python3 tools/skeletal/build.py --no-clips # skeletal/ only, leave animations/ alone

Pipeline: cut the reference into parts -> effect sprites -> skeleton JSON and
atlas -> pose every frame with the official spine-core runtime (Node) ->
rasterise those runtime poses -> frame clips, GIFs, comparisons.
"""

from __future__ import annotations

import hashlib
import json
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

import cv2
import numpy as np
from PIL import Image

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
sys.path.insert(0, str(HERE.parent))

import fx  # noqa: E402
from parts import ORIGIN, H, W, REFERENCE, Part, cut, load_reference, to_world  # noqa: E402
from rig import FPS, Anim, Rig  # noqa: E402

ROOT = HERE.parents[1]
OUT = ROOT / "skeletal"
ANIM_DIR = ROOT / "animations"
SKILLS_PATH = ROOT / "source" / "emotion" / "skills.json"
ARTIFACTS = Path(os.environ.get("ARTIFACTS", "/opt/cursor/artifacts")) / "skeletal"
SKELETON_NAME = "naiwa"
# Last commit with the approved frame-by-frame clips, used for comparisons.
ORIGINAL_REV = "8956ec2"
SPINE_VERSION = "4.2.43"
TUCK_RATIO = 0.78
BALL_RATIO = 0.52

RIG_SLOTS = [
    "shadow", "leg_far", "foot_far", "leg_near", "foot_near", "body", "head", "eye_near",
    "hand_far", "arm_upper", "arm_fore", "fan_held", "hand_near",
]
CHARACTER_PARTS = [s for s in RIG_SLOTS if s != "fan_held"]


# ---------------------------------------------------------------- regions


class Region:
    def __init__(self, name: str, rgba: np.ndarray):
        self.name = name
        self.rgba = rgba
        self.h, self.w = rgba.shape[:2]
        self.xy = (0, 0)  # atlas position


def curl_images() -> dict[str, tuple[np.ndarray, tuple[int, int, int, int]]]:
    """The approved tuck and ball drawings, placed like the original roll."""
    from render_emotions import CANON_PNG, canon_metrics
    from render_skills import place_shorter

    canon = np.array(Image.open(CANON_PNG).convert("RGBA"))
    cm = canon_metrics(canon)
    out = {}
    for name, key, ratio in (("tuck", "skill-roll-tuck.jpg", TUCK_RATIO), ("ball", "skill-roll-ball.jpg", BALL_RATIO)):
        layer = place_shorter(key, cm, ratio)
        ys, xs = np.where(layer[:, :, 3] > 0)
        bbox = (int(xs.min()), int(ys.min()), int(xs.max()) + 1, int(ys.max()) + 1)
        out[name] = (layer, bbox)
    return out


def pack(regions: list[Region], page_w: int = 2048, pad: int = 2) -> tuple[int, int]:
    """Shelf packer. Returns the page size."""
    order = sorted(regions, key=lambda r: (-r.h, -r.w))
    x = y = shelf = 0
    for r in order:
        if x + r.w + pad > page_w:
            x, y, shelf = 0, y + shelf + pad, 0
        r.xy = (x, y)
        x += r.w + pad
        shelf = max(shelf, r.h)
    height = y + shelf
    page_h = 1
    while page_h < height:
        page_h *= 2
    return page_w, page_h


def write_atlas(regions: list[Region], page: tuple[int, int], png_name: str) -> str:
    lines = [png_name, f"size:{page[0]},{page[1]}", "filter:Linear,Linear", "pma:false"]
    for r in sorted(regions, key=lambda r: r.name):
        lines += [r.name, f"bounds:{r.xy[0]},{r.xy[1]},{r.w},{r.h}"]
    return "\n".join(lines) + "\n"


# ---------------------------------------------------------------- skeleton


class Build:
    def __init__(self):
        self.ref = load_reference()
        self.parts: list[Part] = cut(self.ref)
        self.part = {p.name: p for p in self.parts}
        self.fx = fx.build_all()
        self.curl = curl_images()
        self.rig = Rig()
        self.regions: dict[str, Region] = {}
        self.skin: dict[str, dict] = {}
        self.slots: list[dict] = []
        self.anims: dict[str, Anim] = {}
        self.extra: dict = {}

    # region helpers
    def region(self, name: str, rgba: np.ndarray) -> Region:
        if name not in self.regions:
            self.regions[name] = Region(name, rgba)
        return self.regions[name]

    def attach(self, slot: str, att_name: str, region: str, cx: float, cy: float, **extra):
        """Region attachment whose image centre sits at (cx, cy) in the bone's setup frame (y up)."""
        r = self.regions[region]
        d = {"x": round(cx, 3), "y": round(cy, 3), "width": r.w, "height": r.h}
        if att_name != region:
            d["path"] = region
        d.update(extra)
        self.skin.setdefault(slot, {})[att_name] = d

    def point(self, slot: str, att_name: str, x: float, y: float):
        self.skin.setdefault(slot, {})[att_name] = {"type": "point", "x": round(x, 3), "y": round(y, 3)}

    def slot(self, name: str, bone: str, attachment: str | None = None, color: str | None = None):
        d = {"name": name, "bone": bone}
        if attachment:
            d["attachment"] = attachment
        if color:
            d["color"] = color
        self.slots.append(d)

    def place_canvas(self, slot: str, att: str, region: str, bone: str, bbox: tuple[int, int, int, int]):
        """Attach a canvas-cropped image so it lands exactly where it was cut."""
        x0, y0, x1, y1 = bbox
        cx, cy = to_world((x0 + x1) / 2, (y0 + y1) / 2)
        bx, by = self.rig.get(bone).world
        self.attach(slot, att, region, cx - bx, cy - by)

    def bone_world(self, name: str) -> tuple[float, float]:
        return self.rig.get(name).world

    def build_rig(self):
        rig = self.rig
        p = self.part
        rig.add("root", None, ORIGIN)
        rig.add("fire", "root", ORIGIN)
        rig.add("shadow", "root", p["shadow"].pivot)
        rig.add("leg_far", "root", p["leg_far"].pivot)
        rig.add("foot_far", "root", p["foot_far"].pivot)
        rig.add("leg_near", "root", p["leg_near"].pivot)
        rig.add("foot_near", "root", p["foot_near"].pivot)
        rig.add("body", "root", p["body"].pivot)
        rig.add("head", "body", p["head"].pivot)
        rig.add("eye_near", "head", p["eye_near"].pivot)
        rig.add("hand_far", "body", p["hand_far"].pivot)
        rig.add("arm_upper", "body", p["arm_upper"].pivot)
        rig.add("arm_fore", "arm_upper", p["arm_fore"].pivot)
        rig.add("hand_near", "arm_fore", p["hand_near"].pivot)
        rig.add("fan_held", "hand_near", (141.0, 297.0))

        tuck_layer, tuck_box = self.curl["tuck"]
        ball_layer, ball_box = self.curl["ball"]
        a = ball_layer[:, :, 3].astype(np.float64)
        ys, xs = np.mgrid[0:H, 0:W]
        ball_c = (float((xs * a).sum() / a.sum()), float((ys * a).sum() / a.sum()))
        rig.add("tuck", "root", ((tuck_box[0] + tuck_box[2]) / 2, float(tuck_box[3])))
        rig.add("ball", "root", ball_c)

        # Effect bones. Positions are setup anchors; animations move them.
        rig.add("fx", "root", ORIGIN)
        rig.add("swoosh", "fx", (150.0, 250.0))
        rig.add("fan_fly", "fx", (141.0, 280.0))
        rig.add("fan_trail", "fx", (141.0, 280.0))
        rig.add("boom", "fx", (-55.0, 280.0))

    def build_parts(self):
        for part in self.parts:
            self.region(part.name, part.crop())
        for name in ("tuck", "ball"):
            layer, box = self.curl[name]
            self.region(name, layer[box[1] : box[3], box[0] : box[2]])
        for name, arr in self.fx.items():
            self.region(name, arr)

        # Back to front. Fire and the distant burst sit behind the whole character.
        from effects import layout_boom, layout_projectile, trail_slot

        self.fire_slots()
        layout_boom(self)
        for name in RIG_SLOTS:
            if name == "fan_held":
                self.slot("fan_held", "fan_held")
                continue
            self.slot(name, self.part[name].bone, name)
            self.place_canvas(name, name, name, self.part[name].bone, self.part[name].bbox)
            if name == "hand_far":
                trail_slot(self)
        self.slot("tuck", "tuck")
        self.place_canvas("tuck", "tuck", "tuck", "tuck", self.curl["tuck"][1])
        self.slot("ball", "ball")
        self.place_canvas("ball", "ball", "ball", "ball", self.curl["ball"][1])

        # Fan in the hand: the guard rivet is on the bone.
        rx, ry = fx.FAN_RIVET
        self.attach("fan_held", "fan", "fan", fx.FAN_W / 2 - rx, ry - fx.FAN_H / 2)
        layout_projectile(self)
        self.mouth_points()

    def mouth_points(self):
        """Point attachments that follow the mouth, for the lip-sync anchor."""
        hx, hy = self.bone_world("head")
        mx, my = to_world(153.5, 138.0)
        self.slot("mouth", "head", "mouth")
        self.point("mouth", "mouth", mx - hx, my - hy)
        for name in ("tuck", "ball"):
            layer, _ = self.curl[name]
            rgb = layer[:, :, :3].astype(np.int16)
            eye = (layer[:, :, 3] > 200) & (rgb[:, :, 1] > rgb[:, :, 0] + 15) & (rgb[:, :, 1] > 100)
            ys, xs = np.where(eye)
            ex, ey = float(xs.mean()), float(ys.mean())
            # Reference: mouth sits 28 px ahead of and 23 px under the eye centre.
            scale = TUCK_RATIO if name == "tuck" else BALL_RATIO
            wx, wy = to_world(ex - 28 * scale, ey + 23 * scale)
            bx, by = self.bone_world(name)
            self.slot(f"mouth_{name}", name)
            self.point(f"mouth_{name}", "mouth", wx - bx, wy - by)

    def fire_slots(self):
        from fire import layout_fire

        layout_fire(self)

    # ------------------------------------------------------------ output

    def skeleton_json(self) -> dict:
        events = {}
        for anim in self.anims.values():
            for e in anim.events:
                events[e["name"]] = {}
        data = {
            "skeleton": {
                "hash": "",
                "spine": SPINE_VERSION,
                "x": -ORIGIN[0],
                "y": -(H - ORIGIN[1]),
                "width": W,
                "height": H,
                "fps": FPS,
                "images": "./images/",
                "audio": "",
            },
            "bones": self.rig.to_json(),
            "slots": self.slots,
            "skins": [{"name": "default", "attachments": self.skin}],
            "events": events,
            "animations": {name: a.to_json() for name, a in self.anims.items()},
        }
        blob = json.dumps(data, sort_keys=True).encode()
        data["skeleton"]["hash"] = hashlib.sha1(blob).hexdigest()[:16]
        return data

    def write(self):
        if OUT.exists():
            for sub in ("images",):
                shutil.rmtree(OUT / sub, ignore_errors=True)
        (OUT / "images").mkdir(parents=True, exist_ok=True)
        regions = list(self.regions.values())
        page = pack(regions)
        sheet = np.zeros((page[1], page[0], 4), np.uint8)
        for r in regions:
            x, y = r.xy
            sheet[y : y + r.h, x : x + r.w] = r.rgba
            save_png(r.rgba, OUT / "images" / f"{r.name}.png")
        save_png(sheet, OUT / f"{SKELETON_NAME}.png")
        (OUT / f"{SKELETON_NAME}.atlas").write_text(write_atlas(regions, page, f"{SKELETON_NAME}.png"))
        data = self.skeleton_json()
        (OUT / f"{SKELETON_NAME}.json").write_text(json.dumps(data, separators=(",", ":")) + "\n")
        print(f"atlas page {page}, {len(regions)} regions, {len(self.rig.bones)} bones, {len(self.slots)} slots")


def save_png(arr: np.ndarray, path: Path):
    Image.fromarray(arr, "RGBA").save(path, format="PNG", optimize=True, compress_level=9)


# ---------------------------------------------------------------- runtime poses and raster


def spine_poses(requests: dict[str, list[float]]) -> dict:
    with tempfile.TemporaryDirectory() as tmp:
        req = Path(tmp) / "req.json"
        out = Path(tmp) / "out.json"
        req.write_text(json.dumps(requests))
        subprocess.run(
            ["node", str(HERE / "pose_dump.mjs"), str(OUT / f"{SKELETON_NAME}.json"), str(OUT / f"{SKELETON_NAME}.atlas"), str(req), str(out)],
            check=True,
        )
        return json.loads(out.read_text())


class Raster:
    def __init__(self, regions: dict[str, Region]):
        self.pm = {}
        for name, r in regions.items():
            a = r.rgba[:, :, 3:4].astype(np.float32) / 255.0
            self.pm[name] = np.concatenate([r.rgba[:, :, :3].astype(np.float32) / 255.0 * a, a], axis=-1)

    def render(self, frame: dict) -> np.ndarray:
        acc = np.zeros((H, W, 4), np.float32)
        for s in frame["slots"]:
            if "region" not in s:
                continue
            src = self.pm[s["region"]]
            h, w = src.shape[:2]
            v = s["verts"]
            bl = np.array([v[0] + ORIGIN[0], ORIGIN[1] - v[1]])
            ul = np.array([v[2] + ORIGIN[0], ORIGIN[1] - v[3]])
            ur = np.array([v[4] + ORIGIN[0], ORIGIN[1] - v[5]])
            ax = (ur - ul) / w
            ay = (bl - ul) / h
            A = np.array([[ax[0], ay[0]], [ax[1], ay[1]]])
            T = ul
            col = np.array(s["color"], np.float32)
            if col[3] <= 0.0005:
                continue
            tint = np.array([col[0] * col[3], col[1] * col[3], col[2] * col[3], col[3]], np.float32)
            ident = np.allclose(A, np.eye(2), atol=1e-4) and np.allclose(T, np.round(T), atol=1e-3)
            if ident:
                tx, ty = int(round(T[0])), int(round(T[1]))
                layer = np.zeros((H, W, 4), np.float32)
                xs0, ys0 = max(0, tx), max(0, ty)
                xs1, ys1 = min(W, tx + w), min(H, ty + h)
                if xs1 <= xs0 or ys1 <= ys0:
                    continue
                layer[ys0:ys1, xs0:xs1] = src[ys0 - ty : ys1 - ty, xs0 - tx : xs1 - tx]
            else:
                # Pixel centres: dst = A (src + .5) + T - .5
                off = A @ np.array([0.5, 0.5]) + T - 0.5
                M = np.hstack([A, off[:, None]]).astype(np.float64)
                layer = cv2.warpAffine(src, M, (W, H), flags=cv2.INTER_LINEAR, borderMode=cv2.BORDER_CONSTANT, borderValue=(0, 0, 0, 0))
            layer = layer * tint
            acc = layer + acc * (1.0 - layer[:, :, 3:4])
        alpha = acc[:, :, 3]
        out = np.zeros((H, W, 4), np.uint8)
        rgb = np.divide(acc[:, :, :3], np.maximum(alpha[:, :, None], 1e-8))
        out[:, :, :3] = np.clip(np.round(rgb * 255), 0, 255).astype(np.uint8)
        out[:, :, 3] = np.clip(np.round(alpha * 255), 0, 255).astype(np.uint8)
        out[out[:, :, 3] == 0] = 0
        return out


def visible_equal(a: np.ndarray, b: np.ndarray) -> bool:
    va, vb = a.copy(), b.copy()
    va[va[:, :, 3] == 0] = 0
    vb[vb[:, :, 3] == 0] = 0
    return bool(np.array_equal(va, vb))


def main(argv: list[str]):
    b = Build()
    b.build_rig()
    b.build_parts()
    from anims import author

    author(b)
    b.write()
    from output import finish

    finish(b, write_clips="--no-clips" not in argv)


if __name__ == "__main__":
    main(sys.argv)
