"""Bones, slots, and a keyframe authoring helper that writes Spine 4.2 timelines."""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from parts import ORIGIN, to_world

FPS = 12

# Normalised cubic-bezier handles (x1, y1, x2, y2), like CSS timing functions.
EASES = {
    "inout": (0.42, 0.0, 0.58, 1.0),
    "sine": (0.37, 0.0, 0.63, 1.0),
    "in": (0.55, 0.0, 1.0, 0.45),
    "out": (0.0, 0.55, 0.45, 1.0),
    "outcubic": (0.33, 1.0, 0.68, 1.0),
    "outexpo": (0.16, 1.0, 0.3, 1.0),
    "incubic": (0.32, 0.0, 0.67, 0.0),
    "outback": (0.34, 1.45, 0.64, 1.0),
    "snap": (0.7, 0.0, 0.2, 1.0),
}


@dataclass
class Bone:
    name: str
    parent: str | None
    pivot: tuple[float, float]  # canvas position in the setup pose
    inherit: str = "normal"

    @property
    def world(self) -> tuple[float, float]:
        return to_world(*self.pivot)


@dataclass
class Rig:
    bones: list[Bone] = field(default_factory=list)

    def add(self, name: str, parent: str | None, pivot: tuple[float, float]) -> Bone:
        bone = Bone(name, parent, pivot)
        self.bones.append(bone)
        return bone

    def get(self, name: str) -> Bone:
        for b in self.bones:
            if b.name == name:
                return b
        raise KeyError(name)

    def local(self, name: str) -> tuple[float, float]:
        b = self.get(name)
        wx, wy = b.world
        if b.parent is None:
            return wx, wy
        px, py = self.get(b.parent).world
        return wx - px, wy - py

    def to_json(self) -> list[dict]:
        out = []
        for b in self.bones:
            x, y = self.local(b.name)
            d = {"name": b.name}
            if b.parent:
                d["parent"] = b.parent
            if abs(x) > 1e-9:
                d["x"] = round(x, 3)
            if abs(y) > 1e-9:
                d["y"] = round(y, 3)
            out.append(d)
        return out


def t_of(frame: float) -> float:
    return round(frame / FPS, 6)


def _curve(f0: float, f1: float, v0: list[float], v1: list[float], ease):
    if ease in (None, "linear"):
        return None
    if ease == "step":
        return "stepped"
    a, b, c, d = EASES[ease] if isinstance(ease, str) else ease
    t0, t1 = t_of(f0), t_of(f1)
    dt = t1 - t0
    out = []
    for x0, x1 in zip(v0, v1):
        dv = x1 - x0
        out += [round(t0 + a * dt, 6), round(x0 + b * dv, 5), round(t0 + c * dt, 6), round(x0 + d * dv, 5)]
    return out


class Anim:
    def __init__(self, name: str):
        self.name = name
        self.bones: dict[str, dict] = {}
        self.slots: dict[str, dict] = {}
        self.events: list[dict] = []
        self.raw: dict[str, dict[str, list]] = {}  # for the planning evaluator

    def _timeline(self, bone: str, kind: str, keys: list[tuple], names: tuple[str, ...]):
        keys = sorted(keys, key=lambda k: k[0])
        tl = []
        for i, key in enumerate(keys):
            frame, vals, ease = key[0], list(key[1 : 1 + len(names)]), key[1 + len(names)] if len(key) > 1 + len(names) else "inout"
            entry = {"time": t_of(frame)}
            for n, v in zip(names, vals):
                entry[n] = round(float(v), 5)
            if i + 1 < len(keys):
                nxt = keys[i + 1]
                c = _curve(frame, nxt[0], vals, list(nxt[1 : 1 + len(names)]), ease)
                if c is not None:
                    entry["curve"] = c
            tl.append(entry)
        self.bones.setdefault(bone, {})[kind] = tl
        self.raw.setdefault(bone, {})[kind] = [(k[0],) + tuple(k[1 : 1 + len(names)]) + ((k[1 + len(names)] if len(k) > 1 + len(names) else "inout"),) for k in keys]

    def rotate(self, bone: str, keys):
        self._timeline(bone, "rotate", keys, ("value",))

    def translate(self, bone: str, keys):
        self._timeline(bone, "translate", keys, ("x", "y"))

    def scale(self, bone: str, keys):
        self._timeline(bone, "scale", keys, ("x", "y"))

    def attach(self, slot: str, keys):
        self.slots.setdefault(slot, {})["attachment"] = [{"time": t_of(f), "name": n} for f, n in sorted(keys, key=lambda k: k[0])]

    def alpha(self, slot: str, keys, rgb: str = "ffffff"):
        """Slot alpha keys (frame, alpha, ease). Colour channels stay white."""
        keys = sorted(keys, key=lambda k: k[0])
        tl = []
        for i, key in enumerate(keys):
            frame, a = key[0], float(key[1])
            ease = key[2] if len(key) > 2 else "linear"
            entry = {"time": t_of(frame), "color": rgb + f"{int(round(max(0, min(1, a)) * 255)):02x}"}
            if i + 1 < len(keys):
                if ease == "step":
                    entry["curve"] = "stepped"
                elif ease not in (None, "linear"):
                    nxt = keys[i + 1]
                    c = _curve(frame, nxt[0], [1, 1, 1, a], [1, 1, 1, float(nxt[1])], ease)
                    entry["curve"] = c
            tl.append(entry)
        self.slots.setdefault(slot, {})["rgba"] = tl

    def event(self, frame: float, name: str):
        self.events.append({"time": t_of(frame), "name": name})

    def to_json(self) -> dict:
        d: dict = {}
        if self.slots:
            d["slots"] = self.slots
        if self.bones:
            d["bones"] = self.bones
        if self.events:
            d["events"] = sorted(self.events, key=lambda e: e["time"])
        return d


# ---------------------------------------------------------------- planning FK


def _bezier_y(ease, u: float) -> float:
    if ease in (None, "linear"):
        return u
    if ease == "step":
        return 0.0
    x1, y1, x2, y2 = EASES[ease] if isinstance(ease, str) else ease
    lo, hi = 0.0, 1.0
    for _ in range(40):
        s = (lo + hi) / 2
        x = 3 * (1 - s) ** 2 * s * x1 + 3 * (1 - s) * s * s * x2 + s ** 3
        if x < u:
            lo = s
        else:
            hi = s
    s = (lo + hi) / 2
    return 3 * (1 - s) ** 2 * s * y1 + 3 * (1 - s) * s * s * y2 + s ** 3


def channel(anim: Anim, bone: str, kind: str, frame: float, default: tuple[float, ...]) -> tuple[float, ...]:
    keys = anim.raw.get(bone, {}).get(kind)
    if not keys:
        return default
    if frame <= keys[0][0]:
        return tuple(keys[0][1:-1])
    for k0, k1 in zip(keys, keys[1:]):
        if k0[0] <= frame < k1[0]:
            u = (frame - k0[0]) / (k1[0] - k0[0])
            w = _bezier_y(k0[-1], u)
            return tuple(a + (b - a) * w for a, b in zip(k0[1:-1], k1[1:-1]))
    return tuple(keys[-1][1:-1])


def world_pose(rig: Rig, anim: Anim | None, frame: float) -> dict[str, tuple[float, ...]]:
    """Bone world matrices (a, b, c, d, x, y), Spine 'normal' inheritance, no shear."""
    out: dict[str, tuple[float, ...]] = {}
    for b in rig.bones:
        lx, ly = rig.local(b.name)
        r, = channel(anim, b.name, "rotate", frame, (0.0,)) if anim else (0.0,)
        tx, ty = channel(anim, b.name, "translate", frame, (0.0, 0.0)) if anim else (0.0, 0.0)
        sx, sy = channel(anim, b.name, "scale", frame, (1.0, 1.0)) if anim else (1.0, 1.0)
        x, y = lx + tx, ly + ty
        rad = math.radians(r)
        la, lb = math.cos(rad) * sx, -math.sin(rad) * sy
        lc, ld = math.sin(rad) * sx, math.cos(rad) * sy
        if b.parent is None:
            out[b.name] = (la, lb, lc, ld, x, y)
            continue
        pa, pb, pc, pd, px, py = out[b.parent]
        out[b.name] = (
            pa * la + pb * lc,
            pa * lb + pb * ld,
            pc * la + pd * lc,
            pc * lb + pd * ld,
            pa * x + pb * y + px,
            pc * x + pd * y + py,
        )
    return out


def apply(m: tuple[float, ...], x: float, y: float) -> tuple[float, float]:
    a, b, c, d, wx, wy = m
    return a * x + b * y + wx, c * x + d * y + wy


def world_to_canvas(x: float, y: float) -> tuple[float, float]:
    return x + ORIGIN[0], ORIGIN[1] - y
