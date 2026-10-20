// Pose a Spine 4.2 skeleton with the official spine-core runtime and dump
// what a renderer needs: per slot, the region quad in world space, the slot
// color, and point attachments. Also dumps bone world matrices for tests.
//
// usage: node pose_dump.mjs <skeleton.json> <skeleton.atlas> <requests.json> <out.json>
// requests.json: {"<animation>": [t0, t1, ...], ...}
import fs from "node:fs";
import path from "node:path";
const spineDir = process.env.SPINE_CORE_DIR || "/tmp/spinejs/node_modules/@esotericsoftware/spine-core";
const spine = await import(path.join(spineDir, "dist", "index.js"));

const [skelPath, atlasPath, reqPath, outPath] = process.argv.slice(2);
const atlas = new spine.TextureAtlas(fs.readFileSync(atlasPath, "utf8"));
const loader = new spine.AtlasAttachmentLoader(atlas);
const json = new spine.SkeletonJson(loader);
const data = json.readSkeletonData(fs.readFileSync(skelPath, "utf8"));
const requests = JSON.parse(fs.readFileSync(reqPath, "utf8"));

const skeleton = new spine.Skeleton(data);
const verts = new Float32Array(8);
const out = { spine: data.version, animations: {} };

function snapshot() {
  const slots = [];
  for (const slot of skeleton.drawOrder) {
    const att = slot.getAttachment();
    if (!att) continue;
    const c = slot.color;
    if (att instanceof spine.RegionAttachment) {
      att.computeWorldVertices(slot, verts, 0, 2);
      const ac = att.color;
      slots.push({
        slot: slot.data.name,
        region: att.region.name,
        blend: slot.data.blendMode,
        color: [c.r * ac.r, c.g * ac.g, c.b * ac.b, c.a * ac.a],
        verts: Array.from(verts),
      });
    } else if (att instanceof spine.PointAttachment) {
      const p = att.computeWorldPosition(slot.bone, new spine.Vector2());
      slots.push({ slot: slot.data.name, point: att.name, x: p.x, y: p.y });
    }
  }
  const bones = {};
  for (const b of skeleton.bones) {
    bones[b.data.name] = [b.a, b.b, b.c, b.d, b.worldX, b.worldY];
  }
  return { slots, bones };
}

for (const [name, times] of Object.entries(requests)) {
  const anim = name === "setup" ? null : data.findAnimation(name);
  if (name !== "setup" && !anim) throw new Error(`no animation ${name}`);
  out.animations[name] = { duration: anim ? anim.duration : 0, frames: [] };
  for (const t of times) {
    skeleton.setToSetupPose();
    // Keys are stored as float32; sample a hair after the frame time so a key
    // on this exact frame is taken.
    if (anim) anim.apply(skeleton, 0, t + 1e-5, false, null, 1, spine.MixBlend.setup, spine.MixDirection.mixIn);
    skeleton.updateWorldTransform(spine.Physics.none);
    out.animations[name].frames.push({ t, ...snapshot() });
  }
}
fs.writeFileSync(outPath, JSON.stringify(out));
