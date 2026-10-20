"""The three skill animations, authored as bone and slot keys.

Frames are at 12 fps so integer frames line up with the frame clips; keys on
half frames only show up when a runtime plays the skeleton at a higher rate.
"""

from __future__ import annotations

import math

import numpy as np

from effects import N_DEBRIS, N_SMOKE, N_SPARKS
from parts import to_world
from rig import FPS, Anim, apply, world_pose

RIG_VISUAL = ["shadow", "leg_far", "foot_far", "leg_near", "foot_near", "body", "head", "eye_near", "hand_far", "arm_upper", "arm_fore", "hand_near"]


def spline(keys: dict[float, float], frames: np.ndarray) -> np.ndarray:
    """Catmull-Rom through (frame, value) keys, clamped at the ends."""
    xs = sorted(keys)
    ys = [keys[x] for x in xs]
    out = np.zeros(len(frames))
    for n, f in enumerate(frames):
        if f <= xs[0]:
            out[n] = ys[0]
            continue
        if f >= xs[-1]:
            out[n] = ys[-1]
            continue
        i = max(k for k in range(len(xs) - 1) if xs[k] <= f)
        x0, x1 = xs[i], xs[i + 1]
        p0 = ys[i - 1] if i > 0 else ys[i]
        p1, p2 = ys[i], ys[i + 1]
        p3 = ys[i + 2] if i + 2 < len(xs) else ys[i + 1]
        u = (f - x0) / (x1 - x0)
        out[n] = 0.5 * ((2 * p1) + (-p0 + p2) * u + (2 * p0 - 5 * p1 + 4 * p2 - p3) * u * u + (-p0 + 3 * p1 - 3 * p2 + p3) * u ** 3)
    return out


def author(b):
    b.anims["skill_fan_throw"] = fan_throw(b)
    b.anims["skill_roll"] = roll(b)
    loop = power_cycle(b, "skill_power_up_loop", shift=0.0, with_setup=False)
    b.anims["skill_power_up"] = power_cycle(b, "skill_power_up", shift=1.0, with_setup=True)
    b.anims["skill_power_up_loop"] = loop


# ---------------------------------------------------------------- fan throw

RELEASE = 3.0
BOOM = 7.0
END_FAN = 13.0


def fan_throw(b) -> Anim:
    a = Anim("skill_fan_throw")
    # Anticipation (lean back, fan flicks open), a held wind-up, a whip-fast
    # thrust with overshoot, a beat while the fan is out of sight, a flinch at
    # the distant blast, then an eased settle onto the reference pose.
    a.rotate("body", [(0, 0, "sine"), (1, -2.5, "out"), (2, -4.5, "sine"), (2.6, -5.2, "in"), (3, 1.5, "out"), (3.6, 6.5, "outcubic"),
                      (4.4, 8.0, "sine"), (5.5, 6.5, "sine"), (7, 5.5, "outcubic"), (7.6, 3.0, "sine"), (9, 4.0, "inout"),
                      (11, -1.2, "sine"), (12, 0.35, "sine"), (END_FAN, 0)])
    a.scale("body", [(0, 1, 1, "sine"), (1, 1.012, 0.985, "out"), (2.6, 1.022, 0.968, "in"), (3, 0.99, 1.012, "out"), (3.6, 0.982, 1.024, "outcubic"),
                     (4.6, 1.0, 1.0, "sine"), (7, 1.0, 1.0, "out"), (7.5, 1.012, 0.986, "sine"), (8.5, 1.0, 1.0, "sine"),
                     (11, 1.012, 0.988, "sine"), (12, 0.998, 1.002, "sine"), (END_FAN, 1, 1)])
    a.translate("body", [(0, 0, 0, "sine"), (2.6, 3, -2, "in"), (3, -1, 0, "out"), (3.6, -5, 0, "outcubic"), (4.6, -7, 0, "sine"),
                         (7, -6, 0, "outcubic"), (7.6, -3, 0, "sine"), (9, -4, 0, "inout"), (11.5, 0.5, -0.5, "sine"), (END_FAN, 0, 0)])
    a.rotate("head", [(0, 0, "sine"), (1, -2.0, "sine"), (2.6, -4.0, "in"), (3.6, 3.5, "outcubic"), (5, 2.0, "sine"),
                      (7, 1.5, "outexpo"), (7.4, -4.0, "sine"), (8.6, -1.0, "inout"), (10, 1.2, "sine"), (11.5, -0.6, "sine"), (END_FAN, 0)])

    # Near arm. Positive is counter-clockwise, so negative swings the elbow forward.
    # The fan leaves the hand on the forward swing (frame 3), at full speed;
    # the arm reaches full extension half a frame later and overshoots.
    a.rotate("arm_upper", [(0, 0, "sine"), (1, 9, "out"), (2, 17, "sine"), (2.6, 19, "in"), (3, -16, "out"), (3.6, -42, "outcubic"),
                           (4.4, -47, "sine"), (6, -44, "sine"), (7, -44, "outexpo"), (7.5, -40, "sine"), (8.5, -38, "inout"),
                           (10.5, -4, "sine"), (11.3, 2.2, "sine"), (12.2, -0.4, "sine"), (END_FAN, 0)])
    a.rotate("arm_fore", [(0, 0, "sine"), (1, -32, "out"), (2, -56, "sine"), (2.6, -60, "in"), (3, -12, "out"), (3.6, 22, "outcubic"),
                          (4.4, 25, "sine"), (6, 23, "sine"), (8.5, 22, "inout"), (10.5, 2, "sine"), (11.3, -2.5, "sine"),
                          (12.2, 0.4, "sine"), (END_FAN, 0)])
    a.rotate("hand_near", [(0, 0, "sine"), (1, -10, "out"), (2.6, -22, "in"), (3, -4, "out"), (3.6, 10, "outcubic"), (4.4, 4, "sine"),
                           (8.5, 2, "inout"), (10.5, -6, "sine"), (11.5, 1.5, "sine"), (END_FAN, 0)])
    a.translate("hand_far", [(0, 0, 0, "sine"), (2.6, 1, 1, "snap"), (3.4, -1.5, 0, "sine"), (8.5, -1, 0, "inout"), (END_FAN, 0, 0)])

    # The fan flicks open in the hand during the wind-up.
    a.attach("fan_held", [(0.0, None), (0.8, "fan"), (RELEASE, None)])
    a.scale("fan_held", [(0.8, 0.12, 0.85, "outback"), (1.7, 1.0, 1.0, "sine"), (2.6, 1.0, 1.0)])
    a.rotate("fan_held", [(0.8, 30, "outcubic"), (2.0, 48, "sine"), (2.6, 52, "snap"), (RELEASE, 90)])

    pose = world_pose(b.rig, a, RELEASE)
    ox, oy = b.extra["fan_centroid_from_rivet"]
    m = pose["fan_held"]
    fx_w, fy_w = apply(m, ox, oy)
    rot_w = math.degrees(math.atan2(m[2], m[0]))
    hand_canvas = (fx_w + 195.0, 540.0 - fy_w)
    b.extra["fan_release_canvas"] = hand_canvas
    sx, sy = b.rig.get("fan_fly").world
    # Straight line: constant height, x only decreases, constant speed.
    speed = 52.0
    line = []
    for f in np.arange(RELEASE, BOOM + 0.01, 0.5):
        line.append((float(f), fx_w - speed * (f - RELEASE) - sx, fy_w - sy))
    a.translate("fan_fly", [(f, x, y, "linear") for f, x, y in line])
    a.translate("fan_trail", [(f, x, y, "linear") for f, x, y in line])
    a.rotate("fan_fly", [(RELEASE, rot_w, "linear"), (BOOM, rot_w + 4 * 165.0)])
    a.attach("fan_fly", [(0.0, None), (RELEASE, "fan"), (RELEASE + 0.5, "fan_spin"), (6.5, None)])
    a.attach("fan_trail", [(0.0, None), (RELEASE, "fan_trail"), (6.6, None)])
    # The streak only reaches back to where the fan left the hand.
    trail_len = b.regions["fan_trail"].w - 6
    a.scale("fan_trail", [(f, max(0.02, min(1.0, (speed * (f - RELEASE) + 8) / trail_len)), 0.8 + 0.2 * min(1.0, (f - RELEASE) / 1.5), "linear")
                          for f, _, _ in line])
    a.alpha("fan_trail", [(RELEASE, 0.9), (5.0, 0.85, "in"), (6.5, 0.0)])

    # Smear along the hand's arc on the thrust.
    hand_w = apply(pose["hand_near"], -28, 0)
    sw_x, sw_y = b.rig.get("swoosh").world
    a.translate("swoosh", [(2.6, hand_w[0] + 26 - sw_x, hand_w[1] + 22 - sw_y, "linear")])
    a.attach("swoosh", [(0.0, None), (2.7, "swoosh"), (4.4, None)])
    a.alpha("swoosh", [(2.7, 0.0, "outcubic"), (3.0, 0.95, "in"), (4.4, 0.0)])
    a.scale("swoosh", [(2.7, 0.85, 0.85, "outcubic"), (4.4, 1.08, 1.0)])

    # The burst is on the flight line, past the left edge of the frame.
    bx, by = b.rig.get("boom").world
    boom_x, boom_y = to_world(-22.0, 0)[0], fy_w
    a.translate("boom", [(0, boom_x - bx, boom_y - by, "step")])
    burst(a)
    a.event(RELEASE, "fan_release")
    a.event(BOOM, "fan_explode")
    a.rotate("root", [(0, 0), (END_FAN, 0)])
    return a


def burst(a: Anim):
    rng = np.random.default_rng(11)
    a.attach("boom_flash", [(0.0, None), (BOOM - 0.2, "boom_flash"), (9.0, None)])
    a.scale("boom_flash", [(BOOM - 0.2, 0.5, 0.5, "outexpo"), (BOOM, 1.2, 1.2, "out"), (8.0, 1.55, 1.55, "out"), (9.0, 1.7, 1.7)])
    a.alpha("boom_flash", [(BOOM - 0.2, 1.0), (7.4, 1.0, "in"), (8.0, 0.55, "out"), (9.0, 0.0)])

    a.attach("boom_fire", [(0.0, None), (BOOM, "boom_fire_0"), (7.8, "boom_fire_1"), (8.8, "boom_fire_2"), (9.9, "boom_fire_3"), (11.2, None)])
    a.scale("boom_fire", [(BOOM, 0.62, 0.62, "outexpo"), (8.0, 1.08, 1.05, "out"), (9.0, 1.22, 1.18, "sine"), (11.2, 1.38, 1.3)])
    a.translate("boom_fire", [(BOOM, 0, 0, "sine"), (11.2, 6, 26)])
    a.rotate("boom_fire", [(BOOM, 0, "linear"), (11.2, 24)])
    a.alpha("boom_fire", [(BOOM, 1.0), (9.0, 1.0, "in"), (11.2, 0.0)])

    a.attach("boom_ring", [(0.0, None), (7.2, "boom_ring"), (9.4, None)])
    a.scale("boom_ring", [(7.2, 0.35, 0.35, "outcubic"), (9.4, 1.9, 1.75)])
    a.alpha("boom_ring", [(7.2, 1.0), (8.0, 0.8, "in"), (9.4, 0.0)])

    # Sparks: ballistic, mostly thrown back toward the frame.
    for i in range(N_SPARKS):
        ang = math.radians(-70 + 140 * i / (N_SPARKS - 1) + rng.uniform(-6, 6))
        if i % 4 == 3:
            ang = math.radians(180 + rng.uniform(-50, 50))
        v = rng.uniform(95, 150)  # px per frame at launch
        drag = 0.55
        g = 9.0
        keys_t, keys_r = [], []
        start = BOOM + rng.uniform(0.0, 0.3)
        life = rng.uniform(2.0, 3.2)
        for f in np.arange(0, life + 0.01, 0.25):
            dist = v * (1 - math.exp(-drag * f)) / drag
            x = math.cos(ang) * (14 + dist)
            y = math.sin(ang) * (14 + dist) - 0.5 * g * f * f
            vx = math.cos(ang) * v * math.exp(-drag * f)
            vy = math.sin(ang) * v * math.exp(-drag * f) - g * f
            keys_t.append((start + f, x, y, "linear"))
            keys_r.append((start + f, math.degrees(math.atan2(vy, vx)), "linear"))
        name = f"spark_{i}"
        a.translate(name, keys_t)
        a.rotate(name, keys_r)
        a.attach(name, [(0.0, None), (start, "spark"), (start + life, None)])
        a.alpha(name, [(start, 1.0), (start + life * 0.45, 1.0, "in"), (start + life, 0.0)])
        a.scale(name, [(start, 1.4, 1.0, "out"), (start + life, 0.5, 0.6)])

    # Shreds of the fan flutter down near the frame edge. They stay clear of the character.
    for i in range(N_DEBRIS):
        start = BOOM + 0.5 + rng.uniform(0, 0.6)
        vx, vy = rng.uniform(26, 44), rng.uniform(14, 34)
        x = y = 0.0
        keys_t, keys_r = [], []
        rot, spin = rng.uniform(0, 360), rng.uniform(-70, 70)
        f = 0.0
        end = END_FAN - 0.3
        while start + f <= end + 1e-6:
            keys_t.append((start + f, x, y, "linear"))
            keys_r.append((start + f, rot, "linear"))
            dt = 0.25
            vx *= 0.72 if f < 1.0 else 0.9
            vy = vy * 0.7 - 2.2 * dt * 4
            vy = max(vy, -9.0)
            x += vx * dt + math.sin((f + i) * 2.4) * 2.0
            y += vy * dt
            rot += spin * dt
            f += dt
        name = f"debris_{i}"
        a.translate(name, keys_t)
        a.rotate(name, keys_r)
        a.attach(name, [(0.0, None), (start, "debris"), (end, None)])
        a.alpha(name, [(start, 1.0), (end - 1.2, 1.0, "in"), (end, 0.0)])
        a.scale(name, [(start, 1.6, 1.6)])

    for i in range(N_SMOKE):
        start = 7.8 + i * 0.25
        dx, dy = rng.uniform(-40, 30), rng.uniform(-10, 30)
        rise = rng.uniform(80, 150)
        name = f"smoke_{i}"
        a.attach(name, [(0.0, None), (start, "smoke"), (END_FAN - 0.25, None)])
        a.translate(name, [(start, dx, dy, "out"), (END_FAN - 0.25, dx + rng.uniform(15, 45), dy + rise)])
        a.scale(name, [(start, 0.4, 0.4, "outcubic"), (END_FAN - 0.25, 1.35 + 0.3 * rng.random(), 1.25 + 0.3 * rng.random())])
        a.rotate(name, [(start, rng.uniform(-40, 40), "linear"), (END_FAN - 0.25, rng.uniform(-40, 40))])
        a.alpha(name, [(start, 0.0, "out"), (start + 0.5, 0.72), (9.6, 0.62, "in"), (END_FAN - 0.25, 0.0)])


# ---------------------------------------------------------------- roll

ROLL_END = 12.0
TUCK_IN, BALL_IN, BALL_OUT, RIG_BACK = 2.0, 3.0, 10.0, 11.5


def ball_contact(b, rot: float, sx: float, sy: float) -> float:
    """Lowest point of the scaled, rotated ball, relative to the ball bone."""
    pts = b.extra.setdefault("_ball_pts", None)
    if pts is None:
        layer, box = b.curl["ball"]
        ys, xs = np.where(layer[:, :, 3] > 128)
        bx, by = b.rig.get("ball").pivot
        pts = np.stack([xs + 0.5 - bx, by - (ys + 0.5)], axis=1)
        b.extra["_ball_pts"] = pts
    r = math.radians(rot)
    y = math.sin(r) * pts[:, 0] * sx + math.cos(r) * pts[:, 1] * sy
    return float(y.min())


def tuck_left(b, rot: float, sx: float, sy: float) -> float:
    layer, box = b.curl["tuck"]
    ys, xs = np.where(layer[:, :, 3] > 40)
    bx, by = b.rig.get("tuck").pivot
    px, py = (xs - bx) * sx, (by - ys) * sy
    r = math.radians(rot)
    return float((math.cos(r) * px - math.sin(r) * py).min() + bx)


def roll(b) -> Anim:
    a = Anim("skill_roll")
    ground = 540.0 - 535.0  # world y of the soles
    # Root carries the travel (root motion). It accelerates out of the crouch and
    # brakes as the frog lands, then the last step settles onto the setup pose.
    travel = {0: 0, 1.2: 0, 2: -1.5, 2.5: -4, 3: -10, 4: -23, 5: -36, 6: -49, 7: -61, 8: -71, 9: -80, 10: -91, 11: -101, 11.5: -106, 12: -110}
    frames = np.arange(0, ROLL_END + 0.001, 0.5)
    xs = spline(travel, frames)
    a.translate("root", [(float(f), float(x), 0.0, "linear") for f, x in zip(frames, xs)])

    # Crouch on the rig: hips drop and squash, chin tucks, arms hug the belly.
    a.translate("body", [(0, 0, 0, "sine"), (1, -3, -19, "sine"), (1.7, -5, -30, "step"), (RIG_BACK, 2, -13, "outcubic"), (ROLL_END, 0, 0)])
    a.scale("body", [(0, 1, 1, "sine"), (1, 1.045, 0.935, "sine"), (1.7, 1.065, 0.905, "step"), (RIG_BACK, 1.04, 0.955, "outcubic"), (ROLL_END, 1, 1)])
    a.rotate("body", [(0, 0, "sine"), (1, 4.5, "sine"), (1.7, 7.5, "step"), (RIG_BACK, -3.0, "outcubic"), (ROLL_END, 0)])
    a.rotate("head", [(0, 0, "sine"), (1, 5, "sine"), (1.7, 9, "step"), (RIG_BACK, -2.5, "outcubic"), (ROLL_END, 0)])
    a.rotate("arm_upper", [(0, 0, "sine"), (1.7, 4, "step"), (RIG_BACK, 2, "outcubic"), (ROLL_END, 0)])
    a.rotate("arm_fore", [(0, 0, "sine"), (1.7, -6, "step"), (RIG_BACK, -2, "outcubic"), (ROLL_END, 0)])
    a.translate("hand_far", [(0, 0, 0, "sine"), (1.7, 3, -1, "step"), (RIG_BACK, 1, 0, "outcubic"), (ROLL_END, 0, 0)])
    for leg in ("leg_far", "leg_near"):
        a.scale(leg, [(0, 1, 1, "sine"), (1, 1.02, 0.9, "sine"), (1.7, 1.03, 0.84, "step"), (RIG_BACK, 1.02, 0.92, "outcubic"), (ROLL_END, 1, 1)])

    for slot in RIG_VISUAL:
        a.attach(slot, [(0.0, slot), (TUCK_IN, None), (RIG_BACK, slot)])
    a.attach("mouth", [(0.0, "mouth"), (TUCK_IN, None), (RIG_BACK, "mouth")])
    # The shadow stays on the ground under whichever pose is showing.
    a.attach("shadow", [(0.0, "shadow")])
    a.translate("shadow", [(0, 0, 0, "sine"), (1.7, 4, 0, "sine"), (BALL_IN, 26, 2, "sine"), (BALL_OUT, 26, 2, "sine"), (11, 24, 1, "sine"), (RIG_BACK, 10, 0, "sine"), (ROLL_END, 0, 0)])
    a.scale("shadow", [(0, 1, 1, "sine"), (1.7, 1.03, 1.0, "sine"), (BALL_IN, 0.78, 0.9, "sine"), (6.5, 0.74, 0.9, "sine"), (BALL_OUT, 0.8, 0.92, "sine"), (11, 0.84, 0.95, "sine"), (RIG_BACK, 0.95, 0.98, "sine"), (ROLL_END, 1, 1)])

    # Tuck: load the spring, tip forward.
    a.attach("tuck", [(0.0, None), (TUCK_IN, "tuck"), (BALL_IN, None), (BALL_OUT, "tuck"), (RIG_BACK, None)])
    a.attach("mouth_tuck", [(0.0, None), (TUCK_IN, "mouth"), (BALL_IN, None), (BALL_OUT, "mouth"), (RIG_BACK, None)])
    a.scale("tuck", [(TUCK_IN, 1.02, 0.98, "sine"), (2.5, 1.06, 0.9, "in"), (2.99, 1.07, 0.88, "step"),
                     (BALL_OUT, 1.07, 0.9, "outcubic"), (10.5, 1.02, 0.97, "sine"), (11, 0.98, 1.04, "sine"), (RIG_BACK, 0.99, 1.02)])
    a.rotate("tuck", [(TUCK_IN, 3, "sine"), (2.5, 9, "in"), (2.99, 14, "step"), (BALL_OUT, 4, "outcubic"), (10.5, 1.5, "sine"), (11, -2.5, "sine"), (RIG_BACK, -1)])
    a.translate("tuck", [(TUCK_IN, 0, 0, "sine"), (2.99, -3, 0, "step"), (BALL_OUT, 0, 0)])

    # Ball: one full forward turn (counter-clockwise while travelling left).
    a.attach("ball", [(0.0, None), (BALL_IN, "ball"), (BALL_OUT, None)])
    a.attach("mouth_ball", [(0.0, None), (BALL_IN, "mouth"), (BALL_OUT, None)])
    rot_keys = {BALL_IN: 34, 4: 88, 5: 144, 6: 200, 7: 252, 8: 300, 9: 338, BALL_OUT: 362}
    scale_keys = {BALL_IN: 1.02, 3.5: 1.0, 9: 1.0, BALL_OUT: 1.0}
    squash_keys = {BALL_IN: 1.0, 5.5: 1.0, 6: 0.97, 6.5: 1.0, 9: 1.0, 9.5: 0.97, BALL_OUT: 0.96}
    bf = np.arange(BALL_IN, BALL_OUT - 0.49, 0.5)
    rots = spline(rot_keys, bf)
    scs = spline(scale_keys, bf)
    sqs = spline(squash_keys, bf)
    by = b.rig.get("ball").world[1]
    rk, tk, sk = [], [], []
    for f, r, s, q in zip(bf, rots, scs, sqs):
        sx, sy = s * (2 - q), s * q
        low = ball_contact(b, r, sx, sy)
        rk.append((float(f), float(r), "linear"))
        sk.append((float(f), float(sx), float(sy), "linear"))
        tk.append((float(f), 0.0, float(ground - by - low), "linear"))
    a.rotate("ball", rk)
    a.scale("ball", sk)
    a.translate("ball", tk)
    a.event(TUCK_IN, "roll_start")
    a.event(BALL_OUT, "roll_land")
    b.extra["roll_travel"] = travel[ROLL_END]
    return a


# ---------------------------------------------------------------- power-up


def power_cycle(b, name: str, shift: float, with_setup: bool) -> Anim:
    """A one-second burn that loops. `shift` frames of setup pose come first."""
    a = Anim(name)
    info = b.extra["fire"]
    step = 0.5
    loop_frames = np.arange(0, FPS + 0.001, step)  # frames within the loop, 0..12

    def T(f):  # loop frame -> animation frame
        return float(f + shift)

    def tau(f):
        return f / FPS

    def vis(slot: str, keys):
        if with_setup:
            keys = [(0.0, None)] + [(k + shift, n) for k, n in keys]
        a.attach(slot, keys)

    def flipbook(slot: str, offset: int):
        keys = []
        for k in range(FPS):
            keys.append((float(k), f"f{(k + offset) % 6}"))
        vis(slot, keys)

    for f in info["big"]:
        flipbook(f["slot"], f["offset"])
        ph = f["phase"]
        a.scale(f["bone"], [(T(k), 0.9 * (1 + 0.06 * math.sin(math.tau * (tau(k) + ph))), f["scale"] * (1 + 0.09 * math.sin(math.tau * (2 * tau(k) + ph)) + 0.04 * math.sin(math.tau * (3 * tau(k) + 2 * ph))), "linear") for k in loop_frames])
        a.rotate(f["bone"], [(T(k), f["rot"] + 5 * math.sin(math.tau * (tau(k) + ph)), "linear") for k in loop_frames])
        a.alpha(f["slot"], [(T(k), 0.82 + 0.12 * math.sin(math.tau * (2 * tau(k) + ph)), "linear") for k in loop_frames])

    for f in info["flames"]:
        flipbook(f["slot"], f["offset"])
        ph, ph2 = f["phase"], (f["phase"] * 3.7) % 1
        a.scale(f["bone"], [(T(k), f["width"] * (1 - 0.05 * math.sin(math.tau * (2 * tau(k) + ph))),
                             f["scale"] * (1 + 0.13 * math.sin(math.tau * (2 * tau(k) + ph)) + 0.07 * math.sin(math.tau * (3 * tau(k) + ph2))), "linear") for k in loop_frames])
        a.rotate(f["bone"], [(T(k), f["rot"] + 4 * math.sin(math.tau * (tau(k) + ph2)) + 2 * math.sin(math.tau * (3 * tau(k) + ph)), "linear") for k in loop_frames])

    # Licks: three per second peel off the tips, shrink, and fade.
    for f in info["licks"]:
        keys_t, keys_s, keys_a = [], [], []
        for k in loop_frames:
            age = (3 * tau(k) + f["phase"]) % 1.0
            keys_t.append((T(k), math.sin(math.tau * age) * 3, f["rise"] * age, "linear"))
            keys_s.append((T(k), 0.42 * (1 - 0.5 * age), 0.5 * (1 - 0.6 * age), "linear"))
            keys_a.append((T(k), (math.sin(math.pi * min(age * 1.4, 1.0)) ** 0.8) * (1 - age), "linear"))
        # The wrap from age 1 back to 0 happens at zero alpha.
        flipbook(f["slot"], int(f["phase"] * 6))
        a.translate(f["bone"], keys_t)
        a.scale(f["bone"], keys_s)
        a.rotate(f["bone"], [(T(0), f["rot"])])
        a.alpha(f["slot"], keys_a)

    for e in info["embers"]:
        keys_t, keys_a, keys_s = [], [], []
        for k in loop_frames:
            age = (tau(k) - e["phase"]) % 1.0
            if age < e["life"]:
                u = age / e["life"]
                al = math.sin(math.pi * u) ** 0.6
            else:
                u, al = 0.0, 0.0
            x = e["drift"] * u + e["sway"] * math.sin(e["wob"] + u * 7.0) * u
            y = e["rise"] * (u ** 0.85)
            keys_t.append((T(k), x, y, "linear"))
            keys_a.append((T(k), al, "linear"))
            s = e["size"] * (1 - 0.45 * u)
            keys_s.append((T(k), s, s, "linear"))
        vis(e["slot"], [(0.0, "ember")])
        a.translate(e["bone"], keys_t)
        a.alpha(e["slot"], keys_a)
        a.scale(e["bone"], keys_s)

    vis("fire_aura", [(float(k), f"a{k % 6}") for k in range(FPS)])
    a.alpha("fire_aura", [(T(k), 0.9 + 0.1 * math.sin(math.tau * (3 * tau(k) + 0.1)), "linear") for k in loop_frames])
    vis("fire_glow", [(0.0, "fire_glow")])
    a.scale("fire_glow", [(T(k), 1 + 0.025 * math.sin(math.tau * 2 * tau(k)), 1 + 0.04 * math.sin(math.tau * (2 * tau(k) + 0.15)), "linear") for k in loop_frames])
    a.alpha("fire_glow", [(T(k), 0.8 + 0.2 * math.sin(math.tau * (2 * tau(k) + 0.3)), "linear") for k in loop_frames])
    vis("fire_pool", [(0.0, "fire_pool")])
    a.scale("fire_pool", [(T(k), 1 + 0.04 * math.sin(math.tau * (3 * tau(k))), 1 + 0.08 * math.sin(math.tau * (3 * tau(k) + 0.2)), "linear") for k in loop_frames])
    a.alpha("fire_pool", [(T(k), 0.85 + 0.15 * math.sin(math.tau * (3 * tau(k) + 0.5)), "linear") for k in loop_frames])
    if with_setup:
        # The clip holds its last burn frame for one frame, like the frame clip.
        a.rotate("fire", [(0.0, 0.0), (T(FPS), 0.0)])
    return a
