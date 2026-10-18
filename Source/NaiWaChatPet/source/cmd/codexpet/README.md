# Windows desktop host

This is a maintainable reconstruction. It uses the `animation`, `compositor`, `safestore` and `winlayer` packages; it is not recovered V10 source.

`embedded_assets/runtime.json` currently has `release_gate.passed=false`. It is deliberately unable to open a pet window. The final build must replace this directory with the repaired assets and a verified manifest before compiling. The host embeds the directory, so the finished release can be one EXE. `-assets` overrides the embedded assets for debugging.

```
go build -ldflags "-H windowsgui" -o CodexPetRebuilt.exe ./cmd/codexpet
CodexPetRebuilt.exe -assets C:\pet\released-assets -storage C:\pet\ReceivedFiles
```

`-check` validates the manifest, every resource hash, complete clips, PNG decoding, the shared canvas and alpha-zero RGB before any native window is created. A passed configuration check is not evidence of GUI playback quality.

The window has a transparent layered canvas, topmost placement and no background brush. Drag with the left button, double-click for laughter, use the right menu for sizes and exit, or hold Ctrl while scrolling. The asset mask uses alpha > 40. Old opaque black/white masks are converted from luminance in `auto` mode; `alpha`, `luminance` and `body` can be selected explicitly. Hover exit keeps a 250 ms grace period.

The UI thread calls `runtime.LockOSThread`, uses a 16 ms timer and advances a single monotonic engine clock. Body, prop and shadow share one stable source canvas and one global scale. `GetDpiForWindow` and `WM_DPICHANGED` change the common DPI factor while keeping the bottom line stable. Resizing never restarts a clip. The GDI surface is resized and closed explicitly. File copying and peer I/O run in background workers; no worker accesses the surface.

The default receiving directory is `ReceivedFiles` beside the EXE. `-storage` sets another directory. Dragging files requests a safe transfer into a fresh name: the storage package verifies the destination before deleting the source, and does not overwrite another file. A verified destination produces an eating request even if source deletion reports an error; the message makes that partial outcome clear. Closed ordinary local files are supported; directory, reparse, hard-link, stream and open-file restrictions are reported by `safestore`. Successful storage notifications remain queued until the engine accepts them. Eating, digestion and dropping keep priority; repeated clicks/contact cannot reset those clips.

Instances of this host exchange protocol-tagged JSON in the owned `CodexPetReconstruction/peers` directory below the local temporary directory (`-peer-dir` overrides it). Each process owns a random filename. Heartbeats are written every 200 ms, contact is checked every 140 ms, future or > 5-second-old entries are ignored, and a latch emits one contact action until the pets separate. The host never reads or modifies V10's peer directory. Cleanup removes only this instance's exact heartbeat and temporary file.

`runtime.json` version 1 uses this schema. `duration_ns` means the duration **of one frame**, not an entire clip. Every one of the fourteen state keys in `animation.States` must be supplied; omitted keys or resources stop startup. The example below omits most clips and hashes for brevity and cannot be used as a release manifest.

```json
{
  "version": 1,
  "canvas": {"width": 420, "height": 560},
  "clips": {"idle": {"frames": 28, "duration_ns": 85000000}},
  "digest_delay_ns": 10000000000,
  "begin_drop_frame": 6,
  "body_dir": "body",
  "hover_mask": "hover_mask.png",
  "hover_mask_mode": "auto",
  "prop": "prop.png",
  "scale": {"default": 0.46, "min": 0.26, "max": 0.65, "step": 0.035, "menu": [0.35, 0.46, 0.55, 0.65]},
  "drop_geometry": {
    "x": 210, "start_y": 400, "ground_y": 480,
    "initial_velocity": 0, "gravity": 700, "restitution": 0.2,
    "settle_speed": 15, "reveal_ns": 130000000, "squash_ns": 90000000
  },
  "release_gate": {"passed": true, "reason": "Exact asset review evidence lives with the release pipeline."},
  "frames_sha256": {"body/idle_000.png": "64 hex digits for the exact PNG bytes"}
}
```

The drop coordinates, sprite and event marker must be calibrated to the actual repaired pixels. The schema example is not a calibration result. `body_dir` contains numbered files such as `idle_000.png`; `frames_sha256` includes every body frame, the prop, and hover mask using asset-relative slash paths. SHA checks and decoding use the same immutable in-memory bytes, so a directory change between verification and cache loading cannot replace a reviewed frame. Transparent RGB is validated independently of the release flag.

Pure-function tests cover the action queue, bridge completion, hover hold/mask, contact latch, peer staleness, shared DPI geometry, filename uniqueness, release gate, hashes and missing resources. Win32 declarations compile and pass `go vet`. No desktop window was started for this implementation, and dragging, menus, multi-monitor DPI, foreground behavior, real double-instance interaction and live file dropping have not been visually or interactively accepted.
