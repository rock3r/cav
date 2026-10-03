# Making motion graphics with cav

You drive a running Cavalry app with `cav`. Your JavaScript runs inside Cavalry. You cannot
see the viewport: render frames and look at them to know what you made.

Other guides: `cav guide design` (motion recipes), `cav guide music` (beat sync),
`cav guide traps` (silent failures), `cav guide native` (duplicators, filters, 3D, particles).

## 1. Check the setup

Run `cav doctor`. If it reports `FAIL bridge`, ask the user to open Cavalry and click
**Scripts > cav-bridge**, and keep its window open. Do not try other ways to reach Cavalry.
If it reports `blocked: a sandbox`, your sandbox stops `cav`: show the user the fix it prints.

## 2. The workflow (follow it in order)

1. **Plan first.** Write the timeline as a table: time (s), frame, what happens. With music,
   plan in beats. Pick 2-4 colours and 1-2 fonts.
2. **Start a new scene**: `cav scene new --width 1920 --height 1080 --fps 60 --seconds 8 --bg '#0b0d12'`.
   It refuses when the open scene has unsaved changes. Never use `--force` on the user's own
   work: ask first.
3. **Write the build as script files** (for example `build/01_title.js`), one section per
   file, and run them with `cav run build/01_title.js`. Files can be fixed and run again.
4. **Review after every file**: `cav sheet` renders 12 frames into `renders/sheet.png`. Open
   the image and look at it. `cav sheet 0-240:20` for a range, `cav sheet 30,60,90` for a
   list, `--bpm 120` to see beat numbers.
5. **Fix numbers, not guesses**: `cav tree` (ids and structure), `cav layer <id>` (position,
   bounding box, keys).
6. **Check**: start with `cav check --quick --json` for structural performance warnings.
   `cav check` adds bounded visual checks; `--profile` optionally measures a small frame set.
   Read failures and skipped coverage, and use a sheet to review blank starts/ends. Fix
   each finding or explain why it is intended. Warnings describe risk, not measured blame.
7. **Save**: `cav scene save scenes/<name>.cv`.
8. **Render**: `cav render -o renders/final.mp4 [--audio music.wav]`. Check the frame count
   it prints.

## 3. Writing scripts

The helper library is the global `cav` in every `cav run`. `cav helpers` prints its
reference; `cav helpers <word>` prints one part. Prefer helpers to raw `api.*` calls: they
avoid the traps in `cav guide traps` and give clear errors.

```js
// build/01_title.js: a title that drops in on the beat, then leaves.
var c = cav.comp()                         // {width, height, fps, left, right, top, bottom}
var beat = cav.beats(120)                  // beat.beat(n) is the frame of beat n (beat 0 is frame 0)
cav.plane('bg', '#0b0d12')
var title = cav.text('title', 'LAUNCH DAY', 140, { font: 'Inter', style: 'Black', color: '#f5f7fa', y: 40 })
var bar = cav.rect('underline', 520, 10, { fill: '#ff4d2e', y: -60, radius: 5 })

cav.slideIn(title, beat.beat(0), { dy: -80, dur: 20, ease: 'outBack' })
cav.wipeIn(bar, beat.beat(1), { dur: 18, from: 'left' })
for (var b = 2; b <= 6; b++) cav.punch(title, beat.beat(b))   // a small hit on every beat
cav.slideOut(title, beat.beat(7), { dy: 60, dur: 16 })        // wind up (inBack), then exit
cav.fadeOut(bar, beat.beat(7), 12)
return { title: title, bar: bar }
```

Rules that keep scripts working:

- (0, 0) is the **centre** of the comp, and **+y is up**. The top edge is `c.top`, not 0.
- Frames are whole numbers. Scale is a multiplier (1 = 100 %). Opacity is 0-100. Rotation is
  in degrees.
- Keys: `cav.key(layer, 'position', [[0, [x, y], 'outExpo'], [30, [x2, y2]]])`. The ease on a
  key shapes the move that **starts** at that key. A move without an ease is linear.
- `cav.tween(layer, attr, f0, f1, from, to, ease)` places both keys. Two tweens on one
  attribute hold still between them.
- Build each panel (dialog, card, button) as a group, and create everything in it with
  `parent: thatGroup`. A layer left outside moves and stacks on its own, so it can end up
  behind the panel.
- Text is centred on its `x` by default (`cav.text`, `cav.glyphs`). Pass `align: 'left'` when
  `x` is the left edge, for example for text typed into a field.
- Variables do not survive between `cav run` calls. Store ids on `globalThis`, or look layers
  up with `cav.find('name')`.
- Look up API names with `cav api <word>` and node attributes with `cav docs <node name>` and
  `cav layer <id> --attrs`. Do not guess attribute names.
- A long build is not a failure: if `cav run` exits with code 3, the job is still running.
  Use `cav operation status <id>` and `cav operation resume <id>`. Do not retry the
  original command. Raw `cav job wait <job-id>` finishes only one job, including metadata.

## 4. Make it look professional

Read `cav guide design` before a creative task. The short version:

- **Every move has an ease.** Entrances `outExpo`/`outBack`, exits `inBack`/`inCubic`, loops
  `inOut`. Linear only for constant drifts.
- **Overlap and stagger**: elements arrive 2-6 frames apart, not all at once (`cav.stagger`,
  `cav.cascade`).
- **Anticipation and overshoot**: wind up before a big move (`inBack`), land with a small
  overshoot (`outBack`, `spring`).
- **Hierarchy**: one hero element at a time. Big headline, small kicker, lots of empty space.
  Keep all text inside 90 % of the frame, including text in buttons and pills (make the pill
  wider than its text).
- **Readable sizes at 1080p**: headlines 100-180 px, names 56-72 px, labels 30-40 px, nothing
  under 28 px. Secondary text in light grey (`#c3cad8`) on dark, never mid grey.
- **Keep it moving**: during a hold, something large still moves a little (a slow drift, a
  gentle scale breathe, `cav.oscillate`/`cav.wiggle`, staggered accents). Only the text stays
  still enough to read. Every card, word and panel gets an entrance, not just a cut.
- **Start fast**: something clearly visible within the first half second.
- **Use the whole length**: end on a finished frame, not on seconds of empty screen.
- **Timing at 60 fps**: small moves 12-20 frames, big moves 20-40, holds long enough to read
  (about 1 s per 3 words).
- **Impacts**: `cav.flash`, `cav.ring`, `cav.burst`, `cav.shake` on a top-level rig group, all
  on the same frame. Keep flashes short and partial (`peak` 40-70, `dur` 4-8).
- **Transitions**: zoom-through (`cav.zoomThrough`), wipes, masks, a flash on the cut.
- For many similar items, use native features (duplicators, stagger): `cav guide native`.

## 5. Music

Get the grid with `cav beats music.wav --fps 60 -o build/grid.json`. Put cuts and big hits on
downbeats and small accents on beats; a move that hits a beat must **end** on the beat frame.
Details: `cav guide music`.

## 6. Done checklist

- `cav sheet` reviewed after the last change: nothing missing, clipped, overlapping by
  mistake, or off-screen.
- `cav check` shows no finding that you have not fixed or explained.
- Every moving element has an ease; hits land on beat frames when there is music.
- Scene saved with `cav scene save`; final MP4 rendered with `cav render`, and the frame
  count matches.
- Tell the user the file paths, and what you could not check (for example, you cannot hear
  the music).

## Operation recovery

`render`, `check` and `run` record their phases and job IDs before submission. A render's
--timeout includes metadata, native rendering, postprocessing and validation. Status does
not queue scene work. If the outcome is unknown, inspect the same operation; never stack
retries. Resume preserves completed work and refuses to overwrite an existing final file.
The updated bridge detects changed sessions; older bridges require you to confirm that the
original scene/session is still active. Keep inputs and the CLI build unchanged while
recovering. Native calls cannot be interrupted by a heartbeat or CLI timeout.
