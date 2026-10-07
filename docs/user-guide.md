# cav user guide

`cav` lets you, or a coding agent working for you, build and render motion graphics in
[Cavalry](https://cavalry.studio) from a terminal. You write short JavaScript files; `cav` runs
them inside the Cavalry app that is open on your desktop, shows you contact sheets of the
result, checks it for common mistakes, and renders the final video.

You need macOS (tested) or Windows (install and offline commands tested; driving Cavalry not tested yet), Cavalry 2.4 or newer (tested on 2.7.2 and 2.8.0),
and ffmpeg. Some features, such as 2.5D cameras, physics and particles, need a Cavalry Pro
licence.

## Install

**1. Install the CLI.** On macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/rock3r/cav/main/install.sh | sh
```

On Windows, in PowerShell:

```powershell
irm https://raw.githubusercontent.com/rock3r/cav/main/install.ps1 | iex
```

On macOS the installer preserves the full signed Cav.app under `~/.local/bin/.cav-bundles`
and exposes its executable with the `~/.local/bin/cav` symlink. See
[macOS distribution](NOTARIZATION.md) for migration from old updaters.

The installer puts `cav` in `~/.local/bin` (Windows: `%LOCALAPPDATA%\cav\bin`), checks the
download against the release checksums, and runs `cav setup`. Setup creates a secret token
in `~/.cav/token`, copies the bridge script into Cavalry's Scripts folder, and checks that
Cavalry and ffmpeg are installed. Running it again is safe.

If ffmpeg is missing, install it with `brew install ffmpeg` (macOS) or
`winget install Gyan.FFmpeg` (Windows).

**2. Start the bridge.** Open Cavalry and click **Scripts > cav-bridge**. A small window
appears. Keep it open while you use `cav`. The first time, Cavalry asks whether you trust
the script. You need to start the bridge again each time you restart Cavalry.

**3. Check everything.**

```bash
cav doctor
```

Every line should say `ok`. A line marked `FAIL` has a `fix:` line under it.

**4. Optional: build the docs index.** `cav docs update` downloads the Cavalry documentation
(about 520 pages) to your computer, so `cav docs <words>` can search it offline. Setup does
this for you unless you ran it with `--no-docs`.

## Your first animation

Make a new scene. `cav` refuses to replace a scene with unsaved changes, so it never throws
away your work.

```bash
cav scene new --width 1920 --height 1080 --fps 60 --seconds 4
```

Save this as `title.js`:

```js
var c = cav.comp()
var beat = cav.beats(120)                 // beat.beat(n) is the frame of beat n (beat 0 is frame 0)
cav.plane('bg', '#0b0d12')
var title = cav.text('title', 'LAUNCH DAY', 140, { font: 'Inter', style: 'Black', color: '#f5f7fa', y: 40 })
var bar = cav.rect('underline', 520, 10, { fill: '#ff4d2e', y: -60, radius: 5 })

cav.slideIn(title, beat.beat(0), { dy: -80, dur: 20, ease: 'outBack' })
cav.wipeIn(bar, beat.beat(1), { dur: 18, from: 'left' })
for (var b = 2; b <= 6; b++) cav.punch(title, beat.beat(b))   // a small hit on every beat
cav.slideOut(title, beat.beat(7), { dy: 60, dur: 16 })
cav.fadeOut(bar, beat.beat(7), 12)
return { title: title, bar: bar }
```

Run it, look at it, check it, render it:

```bash
cav run title.js          # prints the returned layer ids
cav sheet                 # renders 12 frames into renders/sheet.png
cav check                 # lists problems a viewer would notice
cav scene save out/title.cv
cav render -o out/title.mp4
```

For this short example, `cav check` still reports that nothing moves in about half of the
piece. Treat its fixes as suggestions: here, a slow drift on the underline would answer it.

Open `renders/sheet.png` after every change. It is the quickest way to see a missing layer,
a wrong position or a wrong colour.

## How scripts work

Your code runs inside a function in Cavalry, so `return` sends a value back to the terminal.
Two libraries are available:

- `api` is Cavalry's own scripting API. `cav api <words>` searches it offline, for example
  `cav api keyframe`.
- `cav` is the helper library. `cav helpers` prints all of it; `cav helpers slideIn` prints
  one entry.

Four rules prevent most surprises:

- `(0, 0)` is the centre of the comp, and `+y` points **up**. The top edge is `cav.comp().top`.
- Frames are whole numbers, opacity is 0 to 100, scale is a multiplier (1 = 100 %), rotation
  is in degrees.
- Variables do not survive between `cav run` calls. Look layers up again with
  `cav.find('name')`, or store ids on `globalThis`.
- Prefer the helpers to raw `api` calls. They avoid several Cavalry behaviours that fail
  silently, such as colour keys given as hex strings or keys on `rotation` instead of
  `rotation.z`.

A long script can outlive the CLI's wait budget. `scene open`, `frame`, `sheet`, `run`, `render` and `check` print an
operation ID. Inspect and resume the complete command without submitting it again:

```sh
cav operation status <operation-id> --json
cav operation resume <operation-id> --timeout 30m --json
```

`cav job wait <job-id>` still waits for one raw job. Waiting for preparatory metadata does
not resume a render. Keep the original scene and inputs unchanged during recovery. See
[operation recovery](recovery.md) for unknown outcomes, partial checks and stale records.

## Everyday commands

| Task | Command |
|---|---|
| Inspect existing bridge state | `cav status` |
| Retrieve scene metadata and structure | `cav scene info`, `cav tree` |
| Inspect a layer (position, bounding box, keys) | `cav layer <id>`, `cav layer <id> --attrs` |
| Change the comp size, length or background | `cav scene comp --seconds 8 --bg '#101014'` |
| Render one frame | `cav frame 90 -o renders/f90.png` |
| Review chosen frames | `cav sheet 0,30,60,90`, or every 30th frame: `cav sheet 0-600:30` |
| See how one move travels (path, easing, overshoot) | `cav onion 0-60` writes `renders/onion.png` |
| Find problems before rendering | `cav check` |
| Render with music | `cav render -o out/final.mp4 --audio music.wav` |
| Look at a track, or check a render against it | `cav spectrogram music.wav`, `cav sync out/final.mp4` |
| Find a layer type id | `cav types gradient` |
| Search the docs | `cav docs stagger` |
| Read how to work, or motion recipes | `cav guide`, `cav guide design` |

Every command accepts `--json` for machine-readable output.

## Syncing to music

`cav beats` finds the tempo, the beats and the downbeats (the first beat of each bar) of a
track, and prints them as frame numbers:

```bash
cav beats music.wav --fps 60 -o grid.json
```

Put cuts and big hits on downbeats and smaller accents on the other beats. Then check that
the hits land where you meant them:

```bash
cav sheet 0-960:36 --bpm 100
```

With `--bpm`, each tile shows its beat number.

Detection sometimes picks half or double the real tempo. If the BPM looks wrong, run it
again with `--bpm <what you expect>`. For a track without a file, `cav beats --bpm 120
--seconds 16` makes an exact grid.

With a file, `cav beats` also lists the sections of the track (where the music changes), sudden
rises and falls in loudness, silences, and the strongest accents with their frequency band.

Two more commands help an agent that cannot hear the music:

```bash
cav spectrogram music.wav --fps 60            # renders/spectrogram.png
cav sync renders/final.mp4                    # renders/sync.png and a report
```

`cav spectrogram` draws the track as one image: a spectrogram with bar lines and section
lines, a loudness curve, and the onsets of the low, mid and high bands. `cav sync` reads a
rendered video, finds its cuts and visual hits, and measures each one against the beats in
frames. It also lists the strong musical moments that no visual hit answers. Its picture
adds the visual change per frame under the spectrogram view.

For movement that follows the music all the time, the `cav.sound` helper connects Cavalry's
own Sound behaviour to an attribute: `cav.sound('/abs/music.wav', layer, 'scale.y', {min: 1,
max: 1.4, low: 20, high: 150})`.

## Planning, assets, music and review

These commands work before and after the Cavalry scene, and none of them needs the bridge.
`cav guide production` is the full walkthrough; this is the short version.

**Services and keys.** Each job (frames, transparent assets, SVG, reference images, sound
search, music, and the critique of a track) has an order of services. cav uses the first one
it can use now, so everything works without keys and gets better with them:

```bash
cav config                                          # the order per job, and key sources
cav config set-key gemini op://Private/Gemini/api key
cav config check                                    # one free call per service
```

A key source is `env:NAME`, `keychain:service/account` (macOS), `op://vault/item/field`
(1Password, read with `op read` when needed) or `oauth` (`cav config login freesound`). cav
never prints or stores a key.

**Licences.** `cav credits licence nc-ok` or `cav credits licence commercial` tells cav what
the project may use. Everything cav downloads or generates is recorded in
`.cav/manifest.json`. `cav credits` prints the credit lines and `cav credits check` lists
licence problems; `cav check` reports them too.

**Storyboard and animatic.**

```bash
cav ref search "neon city night" && cav ref get openverse:<id>   # into moodboard/
cav board mood                                   # renders/moodboard.png
cav board init --bpm 120 --seconds 16 --shots 6  # then describe each shot in storyboard.json
cav board frames                                 # greybox cards, or generated with a key
cav board sheet                                  # renders/board.png
cav board animatic --audio music.wav             # renders/animatic.mp4, cut on the beat
```

**Assets.** `cav gen image "<prompt>"` makes a still; `--alpha` a transparent PNG, `--vector`
an SVG. `cav gen vector logo.png` traces a PNG.

**Music and sound.** `cav sfx search whoosh` finds free sounds, `cav sfx get <ref>` downloads
one with its credit, `cav sfx index <folder>` adds a library you own (for example a Sonniss
GDC bundle), and `cav sfx gen "<prompt>"` generates one. With a key, `cav music gen` makes
takes whose sections follow the storyboard's shots. `cav listen <track> --board
storyboard.json` reports length, tempo, loudness, the cuts against the downbeats, a picture
and, with a Gemini key, a written critique.

**Review.** `cav review renders/final.mp4` serves a page on http://127.0.0.1:8790. Step
frames with the arrow keys, shuttle with J/K/L, set a range with I and O, draw on the frame,
and add notes. "Send to agent" hands the open notes to an agent waiting in
`cav review wait`. Notes live in `renders/final.review.json`, each with a snapshot of the
frame and the drawing.

## Using cav with a coding agent

`cav guide` prints the workflow, the traps, and recipes for motion design (entrances,
impacts, kinetic type, transitions, beat sync), so any agent with a shell can learn `cav` from
`cav` itself. The `cavalry` skill is a short entry point: it tells the agent when to use
`cav`, how to check the setup, and to read `cav guide`. Install it in Claude Code:

```text
/plugin marketplace add rock3r/cav
/plugin install cavalry@cav
```

In Codex:

```bash
codex plugin marketplace add rock3r/cav
```

```bash
codex plugin add cavalry@cav
```

For other agents, copy `plugins/cavalry/skills/cavalry` into the agent's skills folder.

In our tests, a mid-size model did as well with the CLI alone as with the CLI and the old,
longer skill. That is why the skill is now short. See the README for the measured results.

### Agents in a sandbox

Some agent sandboxes block connections to `127.0.0.1` and allow writes only inside the
project folder. `cav` can work through a folder instead:

1. Outside the sandbox, start a relay on a folder the agent can write:

   ```bash
   cav relay --spool /path/to/project/.cav/spool
   ```

2. Inside the project, create a file named `.cav-spool` that contains that path, or set
   `CAV_SPOOL=/path/to/project/.cav/spool` for the agent.

The agent then uses `cav` normally. Each command is written to the folder, and the relay
runs it outside the sandbox and writes the output back. Add `--restricted` to the relay to
block scripts from starting other programs. This catches mistakes; it is not a security
boundary, because scripts can still read and write files through Cavalry's API.

## Troubleshooting

| What you see | Why | What to do |
|---|---|---|
| `FAIL bridge  not running on 127.0.0.1:8723` | The bridge script is not open in Cavalry. | In Cavalry: **Scripts > cav-bridge**. Keep its window open. |
| `listening ... but not answering` | Cavalry is busy with a long script or render, or a dialog box is open. | Wait for the job, or close the dialog in Cavalry, then try again. |
| `running v1.0.0; v1.1.0 is installed` | You updated `cav` but Cavalry still runs the old bridge. It still works. | Close the cav-bridge window and start it again from the Scripts menu. |
| `cav and cav-bridge do not match` or `speaks protocol` | The running bridge and `cav` use different request formats. `cav` will not send jobs. | Run `cav setup`, then close the cav-bridge window and start it again from the Scripts menu. |
| Exit code 3, pending/unknown | The job took longer than `--timeout`. | `cav operation status <id>`, then `cav operation resume <id>`. Do not retry the original command. |
| Exit code 4, bridge lost/session changed | The original result is unresolved. | Stop automatic waiting, preserve partial output, and inspect the operation. File growth is not completion; reconcile before a new render. |
| `unknown job` | The id is wrong, or the result is older than a day. | Check the id. `cav job wait` without an id waits for the most recent job. |
| "Cavalry is busy (not answering)" during a job | A Cavalry operation is blocking the app, for example deleting hundreds of layers. | Keep waiting. `cav` gives up only after 120 s of silence. |
| A layer appears in the wrong place | `+y` is up, and `(0, 0)` is the centre. | Check positions with `cav layer <id>`. |
| Text at the wrong position after a slide | Text is centred on its `x` by default. | Pass `align: 'left'` when `x` is the left edge. |
| Physics or particles do not move in a sheet | They simulate only when frames render in order. | Use a step-1 range: `cav sheet 0-59:1`. |
| `blocked: a sandbox` | An agent's sandbox stops `cav` from reaching `127.0.0.1` or writing `~/.cav`. | Allow `cav` to reach `127.0.0.1:8723` and write `~/.cav`, or use a relay (see "Agents in a sandbox"). |
| `waiting for cav relay` | The agent is in spool mode but no relay is running. | Start `cav relay --spool <folder>` outside the sandbox. |
| `cav rect` or `cav text` is an unknown command | Helpers are used inside scripts, not in the terminal. | Write the call in a `.js` file and run it with `cav run`. |

If you run a script from Cavalry's own JavaScript Editor at the same time as `cav`, both
share Cavalry's single script thread. Long scripts from either side delay the other.

## Updating

```bash
cav version --check       # tells you whether a newer release exists, changes nothing
cav update                # shows the release, asks, installs, then runs cav setup
```

`cav` never updates itself without asking. After an update, restart the cav-bridge window in
Cavalry if `cav doctor` says the running bridge is older. Update the Claude Code plugin with
`/plugin marketplace update cav`.

## Uninstalling

1. Delete the `cav` binary: `~/.local/bin/cav` on macOS, `%LOCALAPPDATA%\cav\bin\cav.exe` on
   Windows (or the folder you chose with `CAV_BIN_DIR`).
2. Delete `~/.cav` (the token, job results and docs cache).
3. Delete `cav-bridge.js` from Cavalry's Scripts folder: macOS
   `~/Library/Application Support/Cavalry/Scripts`, Windows `%APPDATA%\Cavalry\Scripts`.
4. In Claude Code, remove the plugin with `/plugin uninstall cavalry@cav`.

## Reference

Exit codes:

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Error (for example, the script threw) |
| 2 | The bridge cannot be reached |
| 3 | Wait budget expired; native outcome pending or unknown |
| 4 | Bridge lost/refused or operation session changed; preserve partial output and inspect |

Environment variables:

| Variable | Effect |
|---|---|
| `CAV_HOME` | Where `cav` keeps its token, job results and caches (default `~/.cav`). |
| `CAV_SPOOL` | Work through a spool folder and `cav relay`. |
| `CAV_LOG` | Append a JSON line for every `cav` call to this file. |
| `CAV_OUT_DIR` | Default folder for sheets and frames (default `renders`). |
| `CAV_BRIDGE_PORT` | The bridge port (default 8723). |
| `CAV_SCRIPTS_DIR` | Cavalry's Scripts folder, if it is not in the usual place. |

Use `cav check --quick --json` for cheap structural performance warnings. Add `--profile`
for measured sampling, or `--profile-render` to time low-resolution PNG calls too. Default
checks are bounded; inspect `failures`, `skipped`, `complete` and `clean` before treating a
scene as reviewed. Blank heads/tails still need a sheet review.

Quick structural checks include referenced pre-comps and report their composition paths.
For slow later regions, use `cav check --profile-frames 840,1560,1908 --max-warmup 2000
--timeout 30m --json`. Simulation targets automatically render at 10 percent after
chronological warm-up from the comp start. Warm-up cost is separate from sample timings;
the extra-frame allowance defaults to 600. Keep default profiling for a small initial
sample, and use repeated measurements before claiming a performance improvement.

Docs search prefers the longest complete page title at the start of a query. For example,
`cav docs "Look At duplicator"` prioritizes Look At sections, then ranks sections by their
relevance to the query. API search keeps its function-name ranking.


## Long scenes and launch-video checks

```sh
cav scene new --range 0-3599 --fps-preset smooth
cav scene info
cav check --quick --comp section
cav frames 0-599 --keep-every 10 --comp section -o out/preview
cav render --save --chunk-frames 600 --audio track.wav -o out/final.mp4
cav seams --video out/final.mp4 --allow-cuts 600,1200
```

`scene info` reports scene layer/comp nodes and key counts. `countsComplete` says whether
its bounded metadata scan finished. Key-volume and shader warnings are risks to measure,
not evidence that a particular node caused a slow render.

`--comp` on frame, sheet, frames, check and seams accepts a node ID or a unique comp name.
It restores the selected comp's playhead and the original comp/playhead on success or
native failure. Duplicate names fail. Switching comps can mark the scene unsaved in
Cavalry; save explicitly before chunk rendering.

`frames` sorts requests, renders intervening frames from comp start and keeps every Nth
requested frame. Its default budget is 10000 evaluations and at most 1000 retained PNGs.
Unretained frames overwrite a single warm-up PNG. Increase `--max-evaluated` deliberately
for longer previews. Ordinary sparse `frame` and `sheet` calls still skip intervening frames.

`seams` compares f-1/f at in/out and opacity-key boundaries. A full-comp finished video
preserves rendered simulation state; its frame zero must correspond to comp start.
Native simulation sampling evaluates chronologically. Changed-pixel fractions and boxes
are review signals, not proof of bugs. Intended cuts stay visible with `review:false`.
Metadata omissions and the boundary cap are reported. Inspect referenced sections with
`--comp`; reference time remapping is not inferred. Pixel boxes refer to sampled images.

Chunk rendering needs a saved named scene, ffmpeg and ffprobe. Every new chunk replays
from comp start, bounded by `--max-warmup`. Completed segments pass frame-count/hash
checks before concatenation, audio muxing and final publication. The source file is hashed
again before each chunk. Staging stays beside the final destination as recovery evidence.

After a timeout, use ordinary `operation resume ID`; do not start another render. If
Cavalry or the bridge has restarted, first inspect the old outcome and reopen the same
saved scene. Then use `operation resume ID --restart-chunk --acknowledge-unknown-outcome`.
This requires a different idle bridge session. It retains uncertain job/artifact evidence,
reuses validated segments and gives reconciled chunk work a fresh ID. It never cancels
or automatically resubmits native work. At most 1000 chunks are allowed.

See `cav helpers` and `cav guide native` for guarded SkSL, pre-comp, real font-axis,
two-key and typed per-copy drivers, connected Look At, and explicit above/below helpers.
`cav version` checks the installed bridge file offline; `cav status` adds warnings about
the running bridge from its existing probe.
