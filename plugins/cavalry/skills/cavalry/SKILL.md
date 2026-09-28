---
name: cavalry
description: Build professional motion graphics in Cavalry (the 2D motion design app) from the shell with the `cav` CLI - logo stings, kinetic type, lower thirds, charts, UI walkthroughs, transitions, and animations synced to music beats. Use when the user asks to create, animate, edit, inspect or render anything in Cavalry, or mentions .cv scenes.
license: MIT
compatibility: Needs Cavalry 2.4+ (tested on 2.7.2) on macOS or Windows, the `cav` CLI, and ffmpeg. Cavalry must be running with the cav-bridge script open.
metadata:
  version: "0.1.0"
---

# Cavalry with `cav`

You drive a running Cavalry app through the `cav` CLI. Your JavaScript runs inside Cavalry.
You cannot see the viewport: you **must render frames and look at them** to know what you made.

## 1. Check the setup first

```bash
cav doctor
```

- `cav: command not found` -> install it: see `references/install.md`. Ask the user before installing.
- `FAIL bridge` -> ask the user to open Cavalry and click **Scripts > cav-bridge** (keep its window open). Wait for them, then run `cav doctor` again. Do not try other ways to reach Cavalry.
- Every command accepts `--json`. Exit codes: 0 ok, 1 error, 2 bridge not reachable, 3 still running (not a failure: run `cav job wait <id>`), 4 bridge lost.

## 2. The workflow (follow it in order)

1. **Plan on paper first.** Write the timeline as a table: time (s), frame, what happens. At music tempo, plan in beats (see section 5). Pick 2-4 colours and 1-2 fonts.
2. **Start a fresh scene**: `cav scene new --width 1920 --height 1080 --fps 60 --seconds 8 --bg '#0b0d12'`.
   It refuses if the open scene has unsaved changes. Never use `--force` on the user's own work: ask first.
3. **Write the build as script files** in the project (e.g. `build/01_title.js`), one scene section per file, and run them with `cav run build/01_title.js`. Files can be fixed and re-run. Start each file with `cav.clear()` only if it builds the whole comp; otherwise delete just its own group.
4. **Review after every file**: `cav sheet` renders 12 frames spread over the comp into `renders/sheet.png`. Open the image and check it. `cav sheet 0-240:20` for a range, `cav sheet 30,60,90` for a list, `--bpm 120` to see beat numbers.
5. **Fix numbers, not guesses**: `cav tree` (layer ids and structure), `cav layer <id>` (position, bbox, keys).
6. **Check**: `cav check` lists still stretches, text too small to read, layers outside the frame and empty frames at the start or end. Fix every finding, or say why it is intended.
7. **Save**: `cav scene save scenes/<name>.cv`.
8. **Render**: `cav render -o renders/final.mp4 [--audio music.wav]`. Check the frame count it prints.

## 3. Writing scripts

The helper library is loaded as the global `cav` in every `cav run`. Print its reference with `cav helpers` (or `cav helpers <word>`). Prefer helpers over raw `api.*`: they avoid the traps in section 4 and give clear errors.

```js
// build/01_title.js - a title that drops in on the beat, then leaves.
var c = cav.comp()                         // {width, height, fps, left, right, top, bottom}
var beat = cav.beats(120)                  // beat.beat(n) -> frame of beat n (0-based)
var bg = cav.plane('bg', '#0b0d12')
var title = cav.text('title', 'LAUNCH DAY', 140, { font: 'Inter', style: 'Black', color: '#f5f7fa', y: 40 })
var bar = cav.rect('underline', 520, 10, { fill: '#ff4d2e', y: -60, radius: 5 })

cav.slideIn(title, beat.beat(1), { dy: -80, dur: 20, ease: 'outBack' })
cav.wipeIn(bar, beat.beat(2), { dur: 18, from: 'left' })
cav.punch(title, beat.beat(4))                          // small accent on the beat
cav.slideOut(title, beat.beat(7), { dy: 60, dur: 16 })  // anticipation (inBack) then exit
cav.fadeOut(bar, beat.beat(7), 12)
return { title: title, bar: bar }
```

Rules that keep scripts working:
- Coordinates: (0, 0) is the **centre** of the comp. **+y is UP.** The top edge is `c.top`, not 0.
- Frames are integers. Scale is a multiplier (1 = 100 %). Opacity is 0-100. Rotation is degrees.
- Keys: `cav.key(layer, 'position', [[0, [x, y], 'outExpo'], [30, [x2, y2]]])`. The ease on a key shapes the move that **starts** at that key. Without an ease the move is linear.
- `cav.tween(layer, attr, f0, f1, from, to, ease)` places both keys. Two tweens on one attribute hold still between them.
- Variables do not survive between `cav run` calls. Store ids on `globalThis` or look them up with `cav.find('name')`.
- Look up unknown API names with `cav api <word>` and node attributes with `cav docs <node name>` and `cav layer <id> --attrs`. Do not guess attribute names.
- Long builds are fine: if `cav run` exits with code 3, the job is still running. Run `cav job wait <id>`.

## 4. Traps that fail silently (all verified in Cavalry 2.7.2)

| Trap | What you see | Do this |
|---|---|---|
| `api.create`/`api.primitive` put the new layer next to the current selection | Layers end up inside the wrong group and never render | Use `cav.*` create helpers (they clear the selection) |
| `api.parent` keeps the world transform | Children get odd offsets and scales | Use `cav.set(layer, {parent: g, x, y})` or re-set position/scale after parenting |
| One number on a two-value attribute (`generator.radius`, `generator.dimensions`, `amount`) | Silently becomes (0, 0): the shape vanishes | Pass `[x, y]`; `cav.attr` expands single numbers for you |
| Hex strings in colour keyframes | No keys are made | `cav.key(l, 'fill', [[0, '#ff0000'], [10, '#0000ff']])` splits channels for you |
| String keyframes on `text` | Ignored | One layer per letter (`cav.glyphs`) or several text layers you fade between |
| Opacity is clamped to 0-100 | Overshoot easings do nothing on opacity | Use overshoot (`outBack`, `spring`) on position, scale, rotation |
| Stroke width scales with the layer scale | Rings get thick when scaled | Animate `generator.radius` (see `cav.ring`) |
| Keys interpolate across gaps | A layer drifts slowly between two moves | Use `cav.tween`, or add a hold key before the next move |
| `api.reorder(a, b)` | a goes **below** b | Think "a under b" |
| Raw keys on `'rotation'` (it has x, y, z) | No keys are made | `cav.key`/`cav.tween` on `'rotation'` key `rotation.z` for you |
| A behaviour connected to an attribute | It replaces the value instead of adding | Put the layer in a positioned group, animate the child |
| Comp motion blur alone | Renders sharp | `cav.motionBlur()` also switches it on per layer |
| Missing font | Cavalry silently uses another font | `cav.text` warns and falls back; check the warning |

More traps and details: `references/gotchas.md`.

## 5. Music and beat sync

- Given a track: `cav beats music.wav --fps 60 -o build/grid.json`. It prints BPM, beat frames and downbeat frames (first beat of each bar). If the BPM looks half or double, re-run with `--bpm <expected>`.
- Build a grid in scripts: `var b = cav.beats(bpm, {offset: <frame of first beat>})`; `b.beat(n)`, `b.bar(n)`.
- Put cuts and big hits **exactly on downbeats**, small accents on beats. Motion that "hits" a beat should **land** on the beat frame (the ease ends there), so start it a few frames earlier.
- Leave a short silence or calm moment before a big hit: hold, shrink to a dot, then explode on the downbeat.
- Render with `--audio music.wav` and check a few frames around hits with `cav sheet <frames> --bpm <bpm>`.
- Details and recipes: `references/music-sync.md`.

## 6. Make it look professional

Read `references/motion-recipes.md` before a creative task. Short version:
- **Every move has an ease.** Entrances `outExpo`/`outBack`, exits `inBack`/`inCubic`, loops `inOut`. Never linear, except for constant drifts.
- **Overlap and stagger**: elements arrive 2-6 frames apart, not all at once (`cav.stagger`, `cav.cascade`).
- **Anticipation and overshoot**: wind up before a big move (`inBack`), land with a small overshoot (`outBack`, `spring`).
- **Hierarchy**: one hero element per moment. Big bold headline, small kicker, lots of empty space. Keep text inside 90 % of the frame.
- **Readable sizes at 1080p**: headlines 100-180 px, names 56-72 px, labels 30-40 px, nothing under 28 px. Secondary text in light grey (`#c3cad8`) on dark, never mid grey.
- **Use the whole duration**: end on a finished frame (a held lockup or the last exit ending near the last frame), not on seconds of empty screen.
- **Timing at 60 fps**: small moves 12-20 frames, big moves 20-40, holds long enough to read (about 1 s per 3 words).
- **Impacts**: `cav.flash`, `cav.ring`, `cav.burst`, `cav.shake` on a top-level rig group, all on the same frame. Keep flashes short and partial (`peak` 40-70, `dur` 4-8): a long full-white flash reads as a mistake.
- **Transitions**: zoom-through (`cav.zoomThrough`), wipes, masks, a flash on the cut. Cut between sections with group in/out frames.
- Prefer native Cavalry features (duplicators, stagger, shaders) for many similar items: see `references/native-features.md`. Fewer layers make scripts faster.

## 7. Done checklist

- [ ] `cav sheet` reviewed after the last change: nothing missing, clipped, overlapping by mistake, or off-screen.
- [ ] `cav check` shows no findings you have not fixed or explained.
- [ ] Every moving element has easing; hits land on beat frames when there is music.
- [ ] Scene saved with `cav scene save`; final MP4 rendered with `cav render` and the frame count matches.
- [ ] Tell the user the file paths, and what you could not check (for example: you cannot hear the music).
