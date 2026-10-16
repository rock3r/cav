# Making motion graphics with cav

`cav` runs JavaScript inside an open Cavalry app. Render frames and inspect them to see
the result. The CLI cannot show the Cavalry viewport.

Read `cav guide design` for motion recipes and `cav guide music` for beat timing.
`cav guide traps` covers silent failures. `cav guide native` covers duplicators, filters,
3D and particles. `cav guide flow` lists the planning stages. `cav guide production`
covers assets, licences and review. `cav guide ui-showcase` covers software videos.

## 1. Check the setup

Run `cav doctor`. If it reports `FAIL bridge`, ask the user to open Cavalry and click
**Scripts > cav-bridge**. The bridge window must stay open. Use this bridge to reach Cavalry.
If it reports `blocked: a sandbox`, show the user the printed fix.

## 2. The build loop (follow it in order)

1. **Plan the piece.** Make a table with time in seconds, frame number and action. Plan
   in beats when the piece has music. Choose 2-4 colours and 1-2 fonts. For longer work,
   complete and review the brief, mood board, storyboard and animatic first. Read `cav guide flow`.
2. **Start a new scene**: `cav scene new --width 1920 --height 1080 --fps 60 --seconds 8 --bg '#0b0d12'`.
   This command refuses to replace unsaved work. Ask before using `--force` on the user's scene.
3. **Write script files** such as `build/01_title.js`, with one section per file. Run each
   file with `cav run build/01_title.js`. Edit and run it again when needed.
4. **Inspect every change.** `cav sheet` saves 12 frames to `renders/sheet.png`. Open the
   image. Use `cav sheet 0-240:20` for a range or `cav sheet 30,60,90` for chosen frames.
   Add `--bpm 120` to label beats. `cav onion 0-60` combines 8 frames of one move.
   It shows the path, easing and overshoot, and reports holds and jumps.
5. **Inspect values before editing.** Use `cav tree` for IDs and structure. Use
   `cav layer <id>` for position, bounding box and keys.
6. **Check**: start with `cav check --quick --json` for structural performance warnings.
   `cav check` adds bounded visual checks; `--profile` optionally measures a small frame set.
   It also compares frames at section boundaries. A "seam" finding can show a sudden
   appearance, flash or jump. Inspect its Before and After images. `--quick` skips this.
   Quick metadata includes referenced pre-comps. For later slow regions, select
   `--profile-frames F,...`; simulations render intervening frames from the comp start,
   so set `--max-warmup` explicitly when more than 600 extra frames are needed.
   Read warm-up time separately from sample time. Repeat measurements before claiming a
   speedup. Inspect failures and skipped checks. Use a sheet to check blank starts and ends.
   Fix each finding or explain its purpose. Structural warnings describe risks; profiling
   measures their effect.
7. **Save**: `cav scene save scenes/<name>.cv`.
8. **Render** with `cav render -o renders/final.mp4 [--audio music.wav]`. Check its reported
   frame count. Wait on the printed operation ID. If the bridge refuses or the session
   changes, exit 4 requires inspection and preservation of partial output. File size and
   age cannot establish completion or explain why Cavalry exited.

## 3. Writing scripts

Every `cav run` loads the global `cav` helper library. `cav helpers` prints its reference,
and `cav helpers <word>` prints a matching part. Use helpers where possible. They handle
the failures in `cav guide traps` and report clear errors.

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

Follow these script rules:

- (0, 0) is the **centre** of the comp, and **+y is up**. The top edge has coordinate `c.top`.
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
- If `cav run` exits with code 3, its native outcome is pending or unknown.
  Use `cav operation status <id>` and `cav operation resume <id>`. Do not retry the
  original command. Raw `cav job wait <job-id>` finishes only one job, including metadata.

## 4. Motion and layout

Read `cav guide design` before designing motion. Use these rules while building:

- **Set an ease for each move.** Use `outExpo`/`outBack` for entrances,
  `inBack`/`inCubic` for exits and `inOut` for loops. Use linear motion for constant drifts.
- **Overlap and stagger**: space element arrivals 2-6 frames apart (`cav.stagger`,
  `cav.cascade`).
- **Anticipation and overshoot**: wind up before a big move (`inBack`), land with a small
  overshoot (`outBack`, `spring`).
- **Hierarchy**: emphasize one element at a time. Use a large headline, a smaller label and empty space.
  Keep all text inside 90 % of the frame, including text in buttons and pills (make the pill
  wider than its text).
- **Readable sizes at 1080p**: headlines 100-180 px, names 56-72 px, labels 30-40 px, nothing
  under 28 px. Secondary text in light grey (`#c3cad8`) on dark, never mid grey.
- **Add motion during holds.** Move a large element slightly with position, scale,
  `cav.oscillate` or `cav.wiggle`. Keep text still enough to read. Give each card, word
  and panel an entrance.
- **Show content early.** Place a clear visual within the first half second.
- **Finish the piece.** End on a complete frame without trailing empty seconds.
- **Timing at 60 fps**: small moves 12-20 frames, big moves 20-40, holds long enough to read
  (about 1 s per 3 words).
- **Impacts**: `cav.flash`, `cav.ring`, `cav.burst`, `cav.shake` on a top-level rig group, all
  on the same frame. Keep flashes short and partial (`peak` 40-70, `dur` 4-8).
- **Transitions**: zoom-through (`cav.zoomThrough`), wipes, masks, a flash on the cut.
- **App screens and UI demos**: rebuild the UI as layers. Use a screenshot only as a guide to
  check the rebuild against (`cav guide design`, recipe 12). Read `cav guide ui-showcase` first.
- For many similar items, use native features (duplicators, stagger): `cav guide native`.

## 5. Music

Get the grid with `cav beats music.wav --fps 60 -o build/grid.json`. Put cuts and big hits on
downbeats and small accents on beats; a move that hits a beat must **end** on the beat frame.
Details: `cav guide music`.

## 6. Done checklist

- `cav sheet` reviewed after the last change: nothing missing, clipped, overlapping by
  mistake, or off-screen.
- Every `cav check` finding has been fixed or explained.
- Every moving element has an ease; hits land on beat frames when there is music.
- For a 9:16 or 1:1 version, run `cav scene reframe --to 9:16`.
  It leaves the original comp unchanged. Fix the listed issues and inspect it with `cav sheet --comp`.
- Scene saved with `cav scene save`; final MP4 rendered with `cav render`, and the frame
  count matches.
- Tell the user the file paths and any checks left incomplete (for example, audio that
  could not be heard; `cav listen` measures what it can).
- Offer a review: `cav review renders/final.mp4` in the background, then
  `cav review wait` for the user's notes.

## Operation recovery

`render`, `check` and `run` record their phases and job IDs before submission. A render's
`--timeout` covers metadata, native rendering, postprocessing and validation. Status does
not queue scene work. Inspect an unknown outcome through its existing operation ID.
Resume keeps completed work and refuses to overwrite an existing final file.
The updated bridge detects changed sessions; older bridges require confirmation that the
original scene/session is still active. Keep inputs and the CLI build unchanged while
recovering. Native calls cannot be interrupted by a heartbeat or CLI timeout.

Scene opening, frame export and sheets also return operation IDs. Use
`scene open --timeout 15m` for large scenes. The native scene can remain unready after
an open call returns. Resume that operation; opening the scene twice can conflict with
the first call. Frame and sheet jobs restore the playhead after recoverable errors and
retain staged output.
Keep the scene, inputs and CLI build unchanged until completion. Wait for an unresolved
native export before submitting another one.
