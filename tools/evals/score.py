"""Score one eval run automatically and make review images for vision grading.

  uv run --no-project --with numpy --with pillow python score.py <run dir>

Writes <run>/score.json, <run>/review.png (16 labelled frames) and <run>/frames/*.png.
"""

import collections
import json
import subprocess
import sys
from pathlib import Path

import numpy as np
from PIL import Image, ImageDraw

from evalpaths import CAV, FIXTURES, TASKS
SMALL_W, SMALL_H = 160, 90


def ffprobe(path):
    out = subprocess.run(["ffprobe", "-v", "error", "-show_streams", "-show_format", "-of", "json", str(path)],
                         capture_output=True, text=True)
    return json.loads(out.stdout or "{}")


def small_frames(path):
    raw = subprocess.run(["ffmpeg", "-v", "error", "-i", str(path), "-vf", f"scale={SMALL_W}:{SMALL_H}",
                          "-f", "rawvideo", "-pix_fmt", "gray", "-"], capture_output=True).stdout
    n = len(raw) // (SMALL_W * SMALL_H)
    return np.frombuffer(raw[: n * SMALL_W * SMALL_H], np.uint8).reshape(n, SMALL_H, SMALL_W).astype(np.float32)


def onsets(diff, fps):
    """Frames where the picture changes much more than usual (visual hits and cuts)."""
    if len(diff) < 3:
        return []
    med = np.median(diff)
    mad = np.median(np.abs(diff - med)) + 1e-6
    thr = max(med + 3 * mad, 1.0)
    peaks = []
    gap = max(3, int(fps * 0.1))
    for i in range(1, len(diff) - 1):
        if diff[i] >= thr and diff[i] >= diff[i - 1] and diff[i] >= diff[i + 1]:
            if peaks and i - peaks[-1] < gap:
                if diff[i] > diff[peaks[-1]]:
                    peaks[-1] = i
                continue
            peaks.append(i)
    # diff[i] is the change between frame i and i+1: the hit shows on frame i+1
    return [p + 1 for p in peaks]


def beat_grid(music, fps):
    r = subprocess.run([str(CAV), "beats", str(music), "--fps", str(fps), "--json"], capture_output=True, text=True)
    g = json.loads(r.stdout)
    return g["beatFrames"], g["downbeatFrames"]


def scene_stats(cv):
    d = json.loads(cv.read_text())
    nodes = {n["nodeId"]: n for n in d["nodes"]}
    types = collections.Counter(n["nodeType"] for n in d["nodes"])
    curves = collections.defaultdict(list)
    for c in d.get("connections", []):
        frm, to = c.get("from", ""), c.get("to", "")
        if frm.startswith("keyframe#") and ".keyframes." in to:
            curves[to.split(".")[0]].append(frm.split(".")[0])
    segs = eased = 0
    key_frames = []
    for curve, keys in curves.items():
        if nodes.get(curve, {}).get("nodeType") != "animationCurve":
            continue
        ks = []
        for k in keys:
            a = nodes.get(k, {}).get("attributes", {})
            if "timeOffset" not in a:
                continue
            data = (a.get("data") or {}).get("value") or {}
            ks.append((a["timeOffset"]["value"], data.get("numValue"), "easing" in data or "exprEasing" in data))
        ks.sort()
        pairs = list(zip(ks, ks[1:]))
        moving = [v0 is not None and v1 is not None and abs(v1 - v0) > 1e-6 for (t0, v0, _), (t1, v1, _) in pairs]
        # Constant motion is linear on purpose: a long drift (90 frames or more) or a chain of
        # three or more linear segments in a row (a keyed orbit or path). It counts neither way.
        linear_run = [0] * len(pairs)
        i = 0
        while i < len(pairs):
            j = i
            while j < len(pairs) and moving[j] and not pairs[j][0][2]:
                j += 1
            for k in range(i, j):
                linear_run[k] = j - i
            i = j + 1 if j == i else j
        for idx, ((t0, v0, e0), (t1, v1, _)) in enumerate(pairs):
            # Segments shorter than 4 frames are baked motion (keys every frame or two): the
            # curve comes from the baking, not from easing, so they do not count either way.
            if not moving[idx] or t1 - t0 < 4:
                continue
            if not e0 and (t1 - t0 >= 90 or linear_run[idx] >= 3):
                continue
            segs += 1
            eased += 1 if e0 else 0
            key_frames += [int(round(t0)), int(round(t1))]
    layer_types = {"basicShape", "textShape", "group", "rectangleShape", "ellipseShape", "polygonShape", "editableShape",
                   "duplicator", "imageShape", "compositionReference", "footageShape", "starShape"}
    native = {k: types[k] for k in ("duplicator", "stagger", "subMesh", "oscillator", "noise", "gradientShader",
                                     "stringGenerator", "compositionReference", "planarCamera", "forgeDynamicsShape",
                                     "particleShape", "pathfinder", "wave") if types.get(k)}
    return {
        "nodes": len(d["nodes"]),
        "layersApprox": sum(v for k, v in types.items() if k in layer_types or k.endswith("Shape")),
        "animatedSegments": segs,
        "easedSegments": eased,
        "easedRatio": round(eased / segs, 3) if segs else 0.0,
        "keyFrames": sorted(set(key_frames)),
        "native": native,
        "filters": sum(v for k, v in types.items() if "Filter" in k or k.startswith("sceneGroup::")),
    }


def near_any(frames, targets, tol):
    t = np.array(targets)
    if not len(t):
        return 0.0
    hits = sum(1 for f in frames if np.min(np.abs(t - f)) <= tol)
    return hits / len(frames) if frames else 0.0


def covered(targets, frames, tol):
    f = np.array(frames)
    if not len(targets):
        return 0.0
    if not len(f):
        return 0.0
    return sum(1 for t in targets if np.min(np.abs(f - t)) <= tol) / len(targets)


def review_sheet(video, dur, out, fps):
    fdir = out.parent / "frames"
    fdir.mkdir(exist_ok=True)
    n = 16
    tiles = []
    for i in range(n):
        t = min(dur - 0.5 / fps, dur * i / (n - 1)) if dur > 0 else 0
        p = fdir / f"t{i:02d}.png"
        subprocess.run(["ffmpeg", "-v", "error", "-y", "-ss", f"{t:.3f}", "-i", str(video), "-frames:v", "1",
                        "-vf", "scale=480:-2", str(p)])
        if p.exists():
            tiles.append((t, Image.open(p).convert("RGB")))
    for frac, name in ((0.3, "full_30"), (0.65, "full_65"), (0.98, "full_end")):
        subprocess.run(["ffmpeg", "-v", "error", "-y", "-ss", f"{dur * frac:.3f}", "-i", str(video), "-frames:v", "1",
                        str(fdir / f"{name}.png")])
    if not tiles:
        return
    w, h = tiles[0][1].size
    cols = 4
    rows = (len(tiles) + cols - 1) // cols
    lab = 16  # label strip under each tile, so labels never cover the picture
    sheet = Image.new("RGB", (cols * w + (cols + 1) * 6, rows * (h + lab) + (rows + 1) * 6), (40, 40, 44))
    d = ImageDraw.Draw(sheet)
    for i, (t, im) in enumerate(tiles):
        x, y = 6 + (i % cols) * (w + 6), 6 + (i // cols) * (h + lab + 6)
        sheet.paste(im, (x, y))
        label = f"{t:.2f}s f{int(round(t * fps))}"
        d.text((x + 3, y + h + 2), label, fill=(230, 230, 230))
    for f in fdir.glob("t*.png"):
        f.unlink()
    sheet.save(out)


def main():
    run = Path(sys.argv[1]).resolve()
    meta = json.loads((run / "meta.json").read_text())
    spec = json.loads(TASKS.read_text())
    task = next(t for t in spec["tasks"] if t["id"] == meta["task"])
    exp = task["expect"]
    work = run / "work"
    video, scene = work / "out" / "final.mp4", work / "out" / "scene.cv"
    s = {"task": meta["task"], "arm": meta["arm"], "model": meta["model"], "gates": {}, "metrics": {}}

    # cav usage
    calls = errors = job_errors = 0
    cmds = collections.Counter()
    log = run / "cav.jsonl"
    if log.exists():
        for line in log.read_text().splitlines():
            try:
                r = json.loads(line)
            except ValueError:
                continue
            calls += 1
            argv = [a for a in r.get("argv", []) if not a.startswith("-")]
            cmds[argv[0] if argv else "?"] += 1
            if r.get("exit", 0) not in (0,):
                errors += 1
            if r.get("jobOk") is False:
                job_errors += 1
    s["metrics"].update({"cavCalls": calls, "cavErrors": errors, "scriptErrors": job_errors, "cavCommands": dict(cmds),
                         "wallSeconds": meta.get("wallSeconds"), "tokens": meta.get("tokens"), "turns": meta.get("turns"),
                         "toolCalls": meta.get("toolCalls"), "toolErrors": meta.get("toolErrors"), "timedOut": meta.get("timedOut")})

    # A run whose agent changed the bridge ports or tokens did not test its own arm (see run.py).
    s["gates"]["environment"] = "environmentChanged" not in meta
    s["gates"]["sceneSaved"] = scene.exists()
    s["gates"]["videoRendered"] = video.exists()
    fps = exp["fps"]
    if video.exists():
        info = ffprobe(video)
        vs = [x for x in info.get("streams", []) if x.get("codec_type") == "video"]
        aud = [x for x in info.get("streams", []) if x.get("codec_type") == "audio"]
        dur = float(info.get("format", {}).get("duration", 0))
        if vs:
            num, den = (vs[0].get("r_frame_rate", "0/1").split("/") + ["1"])[:2]
            vfps = float(num) / float(den or 1)
            s["metrics"].update({"duration": round(dur, 2), "fps": round(vfps, 2), "width": vs[0].get("width"), "height": vs[0].get("height")})
            s["gates"]["resolution"] = vs[0].get("width") == exp.get("width", 1920) and vs[0].get("height") == exp.get("height", 1080)
            s["gates"]["fps"] = abs(vfps - fps) < 0.5
        s["gates"]["duration"] = abs(dur - exp["seconds"]) <= max(0.5, 0.1 * exp["seconds"])
        if exp.get("audio"):
            s["gates"]["audio"] = bool(aud)
        fr = small_frames(video)
        if len(fr):
            std = fr.reshape(len(fr), -1).std(axis=1)
            coverage = float((std > 4).mean())
            diff = np.abs(np.diff(fr, axis=0)).mean(axis=(1, 2)) if len(fr) > 1 else np.array([0.0])
            moving = diff > 0.05
            longest, cur = 0, 0
            for m in moving:
                cur = 0 if m else cur + 1
                longest = max(longest, cur)
            s["metrics"].update({"coverage": round(coverage, 3), "motionRatio": round(float(moving.mean()), 3),
                                 "longestStillSeconds": round(longest / fps, 2)})
            s["gates"]["coverage"] = coverage >= 0.6
            # Holds are fine (a lockup can rest for a second); a mostly frozen video is not.
            s["gates"]["motion"] = float(moving.mean()) >= exp.get("minMotion", 0.3) and longest / fps <= exp.get("maxStill", max(2.5, 0.4 * exp["seconds"]))
            ons = onsets(diff, fps)
            s["metrics"]["visualOnsets"] = len(ons)
            if exp.get("music"):
                beats, downs = beat_grid(FIXTURES / exp["music"], fps)
                beats = [b for b in beats if b < len(fr)]
                downs = [b for b in downs if b < len(fr)]
                per = 60 / json.loads(subprocess.run([str(CAV), "beats", str(FIXTURES / exp["music"]), "--json"],
                                                     capture_output=True, text=True).stdout)["bpm"] * fps
                chance = min(1.0, 5 / per)
                on_beat = near_any(ons, beats, 2)
                down_hit = covered(downs[1:], ons, 3)
                s["metrics"].update({"onsetsOnBeat": round(on_beat, 3), "chanceOnBeat": round(chance, 3),
                                     "downbeatsHit": round(down_hit, 3)})
                s["gates"]["beatSync"] = on_beat >= 0.4 and down_hit >= 0.75
        review_sheet(video, dur, run / "review.png", fps)
    if scene.exists():
        try:
            st = scene_stats(scene)
            kf = st.pop("keyFrames")
            s["metrics"]["scene"] = st
            s["gates"]["eased"] = st["easedRatio"] >= 0.5 and st["animatedSegments"] >= 4
            if exp.get("music"):
                beats, _ = beat_grid(FIXTURES / exp["music"], fps)
                s["metrics"]["scene"]["keysOnBeat"] = round(near_any(kf, beats, 1), 3)
        except Exception as e:  # a broken scene file is a finding, not a crash
            s["metrics"]["sceneError"] = str(e)
            s["gates"]["eased"] = False
    s["autoPass"] = all(s["gates"].values()) and bool(s["gates"])
    (run / "score.json").write_text(json.dumps(s, indent=2))
    print(json.dumps({"task": s["task"], "arm": s["arm"], "autoPass": s["autoPass"],
                      "failed": [k for k, v in s["gates"].items() if not v]}))


if __name__ == "__main__":
    main()
