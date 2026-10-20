#!/usr/bin/env python3
"""Dev helper: rebuild the skeleton and tile selected frames of one animation.

    python3 tools/skeletal/devshow.py skill_fan_throw 0 1 2 2.6 3 4 --out /tmp/x.png [--crop x0,y0,x1,y1] [--zoom 1.5]
"""

from __future__ import annotations

import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
sys.path.insert(0, str(HERE.parent))

from PIL import Image, ImageDraw  # noqa: E402

import build  # noqa: E402
from anims import author  # noqa: E402
from output import on_white  # noqa: E402
from rig import FPS  # noqa: E402


def main(argv):
    out = Path("/tmp/devshow.png")
    crop = None
    zoom = 1.0
    cols = 0
    args = []
    it = iter(argv[1:])
    for a in it:
        if a == "--out":
            out = Path(next(it))
        elif a == "--crop":
            crop = tuple(int(v) for v in next(it).split(","))
        elif a == "--zoom":
            zoom = float(next(it))
        elif a == "--cols":
            cols = int(next(it))
        else:
            args.append(a)
    name, frames = args[0], [float(f) for f in args[1:]]
    b = build.Build()
    b.build_rig()
    b.build_parts()
    author(b)
    b.write()
    poses = build.spine_poses({name: [round(f / FPS, 6) for f in frames]})
    raster = build.Raster(b.regions)
    tiles = []
    for f, fr in zip(frames, poses["animations"][name]["frames"]):
        im = on_white(raster.render(fr))
        if crop:
            im = im.crop(crop)
        im = im.resize((int(im.width * zoom), int(im.height * zoom)), Image.Resampling.LANCZOS)
        ImageDraw.Draw(im).text((4, 4), f"{f:g}", fill=(200, 0, 0))
        tiles.append(im)
    cols = cols or len(tiles)
    tw, th = tiles[0].width + 4, tiles[0].height + 4
    rows = (len(tiles) + cols - 1) // cols
    sheet = Image.new("RGB", (tw * cols, th * rows), (200, 200, 205))
    for i, t in enumerate(tiles):
        sheet.paste(t, ((i % cols) * tw, (i // cols) * th))
    sheet.save(out)
    print(out)


if __name__ == "__main__":
    main(sys.argv)
