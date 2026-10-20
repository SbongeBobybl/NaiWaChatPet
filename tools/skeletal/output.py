"""Pose with spine-core, rasterise, and write clips, previews, and comparisons."""

from __future__ import annotations

import hashlib
import io
import json
import subprocess
from pathlib import Path

import numpy as np
from PIL import Image, ImageDraw

from build import (
    ANIM_DIR, ARTIFACTS, OUT, ORIGINAL_REV, REFERENCE, ROOT, SKELETON_NAME, SKILLS_PATH, Raster, save_png, spine_poses, visible_equal,
)
from parts import H, W
from rig import FPS

CLIPS = {
    # skill id: (animation, frame count, closes on the reference, loop)
    "skill_fan_throw": ("skill_fan_throw", 14, True, False),
    "skill_roll": ("skill_roll", 13, True, False),
    "skill_power_up": ("skill_power_up", 13, False, True),
}
PREVIEW_FPS = 30
GOLDEN_BONES = [
    "root", "body", "head", "eye_near", "arm_upper", "arm_fore", "hand_near", "fan_held", "hand_far", "leg_far", "leg_near",
    "tuck", "ball", "fan_fly", "boom", "flame_0", "flame_big_0", "ember_0",
]


def times_for(duration: float, fps: int) -> list[float]:
    n = int(round(duration * fps))
    return [round(k / fps, 6) for k in range(n + 1)]


def on_white(arr: np.ndarray) -> Image.Image:
    im = Image.fromarray(arr, "RGBA")
    bg = Image.new("RGBA", im.size, (255, 255, 255, 255))
    bg.alpha_composite(im)
    return bg.convert("RGB")


def original_frames(skill: str, count: int) -> list[np.ndarray]:
    out = []
    for k in range(count):
        blob = subprocess.run(
            ["git", "-C", str(ROOT), "show", f"{ORIGINAL_REV}:animations/{skill}/{k:04d}.png"], check=True, capture_output=True
        ).stdout
        out.append(np.array(Image.open(io.BytesIO(blob)).convert("RGBA")))
    return out


def solid_intact(ref: np.ndarray, frame: np.ndarray) -> bool:
    solid = ref[:, :, 3] == 255
    return bool(np.array_equal(frame[solid], ref[solid]))


def write_manifest(directory: Path, name: str, mouths: list[tuple[int, float, float]], loop: bool, animation: str):
    frames = []
    blob = bytearray()
    for path in sorted(directory.glob("*.png")):
        data = path.read_bytes()
        blob += data
        frames.append({"file": path.name, "sha256": hashlib.sha256(data).hexdigest()})
    keys = [{"frame": f, "x": round(x, 2), "y": round(y, 2)} for f, x, y in mouths]
    manifest = {
        "name": name,
        "emotion": "",
        "skill": name,
        "kind": "skill",
        "fps": FPS,
        "frameCount": len(frames),
        "loop": loop,
        "priority": 40,
        "mouthAnchor": {"frame": keys[0]["frame"], "x": keys[0]["x"], "y": keys[0]["y"]},
        "mouthKeyframes": keys,
        "transitionIn": "",
        "transitionOut": "",
        "canvas": {"width": W, "height": H},
        "groundY": 540,
        "frames": frames,
        "reviewed": True,
        "sourceSha256": hashlib.sha256(blob).hexdigest(),
        "skeletal": {"skeleton": f"skeletal/{SKELETON_NAME}.json", "atlas": f"skeletal/{SKELETON_NAME}.atlas", "animation": animation},
        "issues": [],
    }
    (directory / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return len(frames)


def mouth_of(frame: dict) -> tuple[float, float] | None:
    for s in frame["slots"]:
        if s.get("point") == "mouth":
            return s["x"] + 195.0, 540.0 - s["y"]
    return None


def write_skills_json(counts: dict[str, int]):
    notes = {
        "skill_fan_throw": "Wind-up with the fan flicking open, a whip-fast straight thrust, then the fan spins away on a horizontal line and bursts far past the left edge (flash, fireball, shock ring, sparks, paper shreds, smoke). No enemy is drawn. The clip ends on the reference plate.",
        "skill_roll": "Crouch, tuck, one full forward somersault screen-left (about 110 px of root travel), land in a tuck and rise. Rotation follows the travel and the ball keeps ground contact. Frame clips cannot carry root motion, so the last frame is the reference plate.",
        "skill_power_up": "Layered flame tongues, big back flames, peeling licks, rising embers, a pulsing glow and a ground pool, all behind the reference plate. The burn cycle loops. Opaque character pixels are not repainted.",
    }
    zh = {"skill_fan_throw": "掷扇", "skill_roll": "翻滚", "skill_power_up": "蓄力"}
    skills = []
    for sid, (anim, _, _, loop) in CLIPS.items():
        entry = {
            "id": sid,
            "zh": zh[sid],
            "clip": f"animations/{sid}",
            "fps": FPS,
            "frame_count": counts[sid],
            "note": notes[sid],
            "skeletal": {"skeleton": f"skeletal/{SKELETON_NAME}.json", "atlas": f"skeletal/{SKELETON_NAME}.atlas", "animation": anim},
        }
        if sid == "skill_power_up":
            entry["skeletal"]["loop_animation"] = "skill_power_up_loop"
        skills.append(entry)
    SKILLS_PATH.write_text(json.dumps({"skills": skills}, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def save_gif(frames: list[Image.Image], path: Path, fps: int):
    path.parent.mkdir(parents=True, exist_ok=True)
    pal = [f.convert("P", palette=Image.Palette.ADAPTIVE, colors=255) for f in frames]
    pal[0].save(path, save_all=True, append_images=pal[1:], duration=int(round(1000 / fps)), loop=0, disposal=1, optimize=False)


def label(im: Image.Image, text: str) -> Image.Image:
    d = ImageDraw.Draw(im)
    d.rectangle([0, 0, len(text) * 6 + 8, 14], fill=(30, 30, 34))
    d.text((4, 2), text, fill=(255, 255, 255))
    return im


def side_by_side(orig: list[np.ndarray], new: list[np.ndarray], skill: str) -> tuple[list[Image.Image], Image.Image]:
    gif = []
    for k, (o, n) in enumerate(zip(orig, new)):
        canvas = Image.new("RGB", (W * 2 + 6, H), (210, 210, 214))
        canvas.paste(label(on_white(o), f"original {k:02d}"), (0, 0))
        canvas.paste(label(on_white(n), f"skeletal {k:02d}"), (W + 6, 0))
        gif.append(canvas)
    tw, th = 140, 187
    sheet = Image.new("RGB", (tw * len(new) + 90, th * 2 + 26), (244, 244, 246))
    d = ImageDraw.Draw(sheet)
    d.text((4, 4), f"{skill}  top: original frames  bottom: skeletal (12 fps samples)", fill=(20, 20, 20))
    for row, seq in enumerate((orig, new)):
        d.text((4, 26 + row * th + th // 2), ("original", "skeletal")[row], fill=(20, 20, 20))
        for k, f in enumerate(seq):
            im = on_white(f).resize((tw - 4, th - 4), Image.Resampling.LANCZOS)
            sheet.paste(im, (90 + k * tw, 24 + row * th))
            d.text((92 + k * tw, 26 + row * th), str(k), fill=(120, 120, 120))
    return gif, sheet


def finish(b, write_clips: bool = True):
    ref = b.ref
    ref_bytes = REFERENCE.read_bytes()
    raster = Raster(b.regions)
    requests = {"setup": [0.0]}
    clip_times = {}
    for sid, (anim, count, _, _) in CLIPS.items():
        clip_times[anim] = [round(k / FPS, 6) for k in range(count)]
    data = json.loads((OUT / f"{SKELETON_NAME}.json").read_text())
    durations = {}
    for name in data["animations"]:
        # Duration is the last key time; spine-core reports it after load, so ask for both sets.
        requests[name] = sorted(set(clip_times.get(name, [])))
    poses = spine_poses(requests)
    for name in data["animations"]:
        durations[name] = poses["animations"][name]["duration"]
    preview_req = {name: times_for(durations[name], PREVIEW_FPS) for name in data["animations"]}
    preview = spine_poses(preview_req)

    setup = raster.render(poses["animations"]["setup"]["frames"][0])
    if not visible_equal(setup, ref):
        diff = np.abs(setup.astype(int) - ref.astype(int)).max(axis=2)
        raise SystemExit(f"setup pose does not match the reference: {(diff > 0).sum()} pixels differ")
    print("setup pose == neutral-reference.png (spine-core transforms, every visible pixel)")
    (OUT / "previews").mkdir(parents=True, exist_ok=True)
    save_png(setup, OUT / "previews" / "setup_pose.png")

    report = {}
    counts = {}
    for sid, (anim, count, closes, loop) in CLIPS.items():
        frames_json = poses["animations"][anim]["frames"]
        renders = [raster.render(f) for f in frames_json]
        if not visible_equal(renders[0], ref):
            raise SystemExit(f"{anim} frame 0 is not the setup pose")
        if anim == "skill_fan_throw" and not visible_equal(renders[-1], ref):
            raise SystemExit(f"{anim} does not settle on the setup pose")
        if anim == "skill_power_up":
            for k, r in enumerate(renders):
                if not solid_intact(ref, r):
                    raise SystemExit(f"{anim} frame {k} repaints opaque character pixels")
        bboxes = []
        for r in renders:
            ys, xs = np.where(r[:, :, 3] > 0)
            bboxes.append((int(xs.min()), int(ys.min()), int(xs.max()), int(ys.max())))
        clip_frames = list(renders)
        if closes:
            clip_frames[-1] = ref
        mouths = []
        for k, f in enumerate(frames_json):
            m = mouth_of(f)
            if k == len(frames_json) - 1 and closes or m is None:
                m = (153.5, 138.0)
            mouths.append((k, m[0], m[1]))
        if write_clips:
            dest = ANIM_DIR / sid
            for old in dest.glob("*.png"):
                old.unlink()
            for k, fr in enumerate(clip_frames):
                path = dest / f"{k:04d}.png"
                if k == 0 or (closes and k == count - 1):
                    path.write_bytes(ref_bytes)
                else:
                    save_png(fr, path)
            counts[sid] = write_manifest(dest, sid, mouths, loop, anim)
        else:
            counts[sid] = count
        orig = original_frames(sid, count)
        new_for_cmp = list(clip_frames)
        diffs = []
        for o, n in zip(orig, new_for_cmp):
            oa, na = on_white(o), on_white(n)
            diffs.append(float(np.abs(np.asarray(oa, np.int16) - np.asarray(na, np.int16)).mean()))
        report[sid] = {"frames": count, "mean_abs_diff_vs_original": [round(d, 2) for d in diffs], "bboxes": bboxes}
        gif, sheet = side_by_side(orig, new_for_cmp, sid)
        ARTIFACTS.mkdir(parents=True, exist_ok=True)
        save_gif(gif, ARTIFACTS / f"{sid}_compare.gif", FPS)
        save_gif([on_white(f) for f in clip_frames], ARTIFACTS / f"{sid}_clip_12fps.gif", FPS)
        for dest_dir in (ARTIFACTS, OUT / "previews"):
            sheet.save(dest_dir / f"{sid}_compare.png")
        print(f"{sid}: {count} frames, bbox x-range {min(b_[0] for b_ in bboxes)}..{max(b_[2] for b_ in bboxes)}")

    for name, pdata in preview["animations"].items():
        imgs = [on_white(raster.render(f)) for f in pdata["frames"]]
        if name == "skill_roll":
            # Show root motion: the canvas is wider than the frame clip.
            imgs = [roll_wide(raster, f) for f in pdata["frames"]]
        save_gif(imgs, ARTIFACTS / f"{name}_{PREVIEW_FPS}fps.gif", PREVIEW_FPS)
    if write_clips:
        write_skills_json(counts)
        from render_skills import write_sheets

        write_sheets(ANIM_DIR)
    golden(poses)
    (OUT / "previews" / "report.json").write_text(json.dumps(report, indent=1) + "\n")
    print(json.dumps({k: v["mean_abs_diff_vs_original"] for k, v in report.items()}))


def roll_wide(raster: Raster, frame: dict) -> Image.Image:
    """Render the roll on a canvas shifted so root travel stays in view."""
    shifted = json.loads(json.dumps(frame))
    for s in shifted["slots"]:
        if "verts" in s:
            s["verts"] = [v + 60 if i % 2 == 0 else v for i, v in enumerate(s["verts"])]
    return on_white(raster.render(shifted))


def golden_slot(s: dict) -> list:
    """[slot, attachment, r, g, b, a, 8 quad verts] or [slot, point, x, y]."""
    if "point" in s:
        return [s["slot"], s["point"], round(s["x"], 3), round(s["y"], 3)]
    return [s["slot"], s["region"]] + [round(c, 4) for c in s["color"]] + [round(v, 3) for v in s["verts"]]


def golden(poses: dict):
    """Bone world matrices from spine-core, for the Go loader test."""
    out = {"source": f"spine-core {poses['spine']}", "animations": {}}
    for name, a in poses["animations"].items():
        out["animations"][name] = [
            {"t": f["t"], "bones": {k: [round(v, 4) for v in f["bones"][k]] for k in GOLDEN_BONES if k in f["bones"]},
             "slots": [golden_slot(s) for s in f["slots"]]}
            for f in a["frames"]
        ]
    path = OUT / "validation" / "spine-core-poses.json"
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(out, separators=(",", ":")) + "\n")
