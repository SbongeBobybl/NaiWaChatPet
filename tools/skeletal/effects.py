"""Fan throw effect bones and slots: projectile, trail, smear, and the burst."""

from __future__ import annotations

import fx

N_SPARKS = 12
N_DEBRIS = 6
N_SMOKE = 5


def layout_projectile(b):
    """Front layer: smear, streak, and the flying fan."""
    b.slot("swoosh", "swoosh")
    b.attach("swoosh", "swoosh", "swoosh", 0, 0)

    b.slot("fan_fly", "fan_fly")
    gx, gy = fx.fan_centroid(b.regions["fan"].rgba)
    b.attach("fan_fly", "fan", "fan", fx.FAN_W / 2 - gx, gy - fx.FAN_H / 2)
    b.attach("fan_fly", "fan_spin", "fan_spin", 0, 0)
    b.extra["fan_centroid_from_rivet"] = (gx - fx.FAN_RIVET[0], fx.FAN_RIVET[1] - gy)


def trail_slot(b):
    """Streak behind the fan. It draws under the near arm so it never crosses the hand."""
    b.slot("fan_trail", "fan_trail")
    r = b.regions["fan_trail"]
    # The streak trails to the right of the fan, which flies left.
    b.attach("fan_trail", "fan_trail", "fan_trail", r.w / 2 - 6, 0)


def layout_boom(b):
    """The burst is far away down the flight line, so it draws behind the character."""
    rig = b.rig
    bx, by = rig.get("boom").pivot
    for i in range(N_SMOKE):
        name = f"smoke_{i}"
        rig.add(name, "boom", (bx, by))
        b.slot(name, name)
        b.attach(name, "smoke", f"smoke_{i % 3}", 0, 0)
    rig.add("boom_flash", "boom", (bx, by))
    b.slot("boom_flash", "boom_flash")
    b.attach("boom_flash", "boom_flash", "boom_flash", 0, 0)
    rig.add("boom_fire", "boom", (bx, by))
    b.slot("boom_fire", "boom_fire")
    for s in range(4):
        b.attach("boom_fire", f"boom_fire_{s}", f"boom_fire_{s}", 0, 0)
    rig.add("boom_ring", "boom", (bx, by))
    b.slot("boom_ring", "boom_ring")
    b.attach("boom_ring", "boom_ring", "boom_ring", 0, 0)
    sr = b.regions["spark"]
    for i in range(N_SPARKS):
        name = f"spark_{i}"
        rig.add(name, "boom", (bx, by))
        b.slot(name, name)
        # Bright head on the bone, tail behind it along -x.
        b.attach(name, "spark", "spark", -sr.w / 2 + 3, 0)
    for i in range(N_DEBRIS):
        name = f"debris_{i}"
        rig.add(name, "boom", (bx, by))
        b.slot(name, name)
        b.attach(name, "debris", f"debris_{i % 3}", 0, 0)
