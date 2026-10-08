# Changelog

## Unreleased

- Read `keychain:service/account` keys from Windows Credential Manager on Windows. The
  target name is the service and the user name is the account. A missing entry and a
  refused read are reported the same way as on macOS, where a refused read is now told
  apart from a missing entry.
- Add `cav review`: a local review page for a render. It steps exact frames, shuttles with
  J/K/L, sets ranges, and draws arrows, boxes, ellipses and freehand. Notes go to
  `<video>.review.json` with a snapshot of the frame and drawing. "Send to agent" releases a
  waiting `cav review wait`; `export`, `resolve` and `reopen` close the loop.
- Add `cav config` and `cav doctor --services`: one service per production job, in an order
  you choose, with keys read from environment variables, the macOS keychain, 1Password
  (`op://`) or an OAuth login. Keys are never printed or stored.
- Add `cav sfx` and `cav ref`: search Freesound, Openverse, Pexels, Unsplash, Wikimedia
  Commons and folders you index, download with the licence and credit recorded, and
  generate sound effects (`cav sfx gen`, ElevenLabs).
- Add `cav credits`: a per-project licence setting (`nc-ok` or `commercial`), credit lines,
  and licence checks, which `cav check` also reports.
- Add `cav board`: storyboards timed in beats, frames per shot (greybox cards, or Gemini,
  OpenAI, OpenRouter, Recraft), a board sheet, an animatic cut on the beat, and mood boards.
- Add `cav gen`: generated stills, transparent PNGs and SVG, and PNG-to-SVG tracing.
- Add `cav listen`: loudness, structure, storyboard cuts against downbeats, a picture, and an
  audio model's critique of a track. Add `cav music gen` (ElevenLabs Music, Stable Audio).
- Add `cav music render`: play a written score (notes and hits on a beat grid) with
  built-in synths and drums, samples, or VST3/AU instruments, mixed and mastered locally.
  Plugin tracks take a `preset` and `params`. The limiter measures true (inter-sample)
  peaks and limits a hit at 0 s. `master.reference` (Matchering) now keeps the loudness
  target, the true peak and 48 kHz. The command names missing score files up front, and
  warns when a mix is too sparse to reach its loudness target.
- Add local services: mflux (open image models on Apple silicon), ComfyUI (a workflow you
  choose), and ACE-Step 1.5 (an open music model on a server you run).
- `cav listen --score` rates takes with local models (Audiobox Aesthetics, CLAP), and several
  tracks are compared side by side.
- Add `cav score`: a REAPER project with the render, a marker per shot and the takes, and
  `cav score render` to render it from the command line.
- Add `cav board place`: each shot's frame becomes an image layer named by its shot id in the
  open composition, shown for its beats, so the build starts from the storyboard.
- Add `cav ref arena`: copy an Are.na channel into the mood board as reference-only assets.
- `cav config check --live` (and `cav doctor --services --live`) makes one small paid call
  per keyed service, after asking.
- The review page shows the audio waveform, reloads a re-rendered file, and compares it with
  the earlier render (A/B wipe, difference) or as an onion skin. It follows the system's
  light or dark theme, with a manual override.
- Add `cav guide production`.
- The cavalry plugin adds a command hook for Claude Code and Codex (`cav review hook`): when
  the reviewer sends notes that no `cav review wait` picked up, the agent hears about it once,
  on the next prompt or at the end of its turn.
- Add the cav-review plugin for Claude Code: the open review notes above the prompt, a link
  to the page, a toast when notes arrive, and an optional turn that handles them. It reads
  the review through `cav review export` when Claude Code reports a change to the review
  files; it does not poll.

- `cav beats` with an audio file also reports sections, rises, falls, silences, the strongest
  accents with their frequency band, and the energy of each bar.
- Add `cav spectrogram`: draw a track as one labelled image (spectrogram, loudness, bars,
  sections and per-band onsets), so an agent can look at music it cannot hear.
- Add `cav sync`: find the cuts and visual hits in a rendered video, measure them against the
  beat grid, and list strong musical moments that no visual hit answers.
- Add the `cav.sound` helper: drive an attribute from Cavalry's Sound behaviour.
- `cav check` reports text layers whose font is not installed.
- Add fal.ai as an image service: one key (`FAL_KEY`) for FLUX.2, Seedream, Ideogram, Grok
  Imagine and any other fal model id you set with `cav config model fal`.
- Add a `video` job and `cav board motion`: Veo 3.1 (Lite by default) turns chosen shots'
  frames into moving clips, and `cav board animatic` plays them, cut to each shot.
- Add Lyria (`lyria-3.5`) as a music service. Veo and Lyria use the Gemini key unless they
  have their own.
- Add `cav onion`: blend a few frames into one image that shows a motion path, the spacing
  of its easing and any overshoot, and report holds and jumps between neighbouring frames.

## 1.1.2

- Display the plugin as Cav in Claude, Codex and marketplace metadata. State that it is
  not affiliated with Cavalry, Scene Group or Canva.
- Add website, GitHub Issues support and privacy-policy links to the plugin listing.
  The privacy policy states that the plugin collects no data.
- Include a plugin README covering setup, local execution, downloads, support and license
  for directory submission.

## 1.1.1

- Inspect hand-off seams using bounded frame pairs from a full-comp video or native
  renders. Report changed pixels, bounding boxes, intended cuts and omitted coverage.
- Add chronological sparse previews with `frames --keep-every` and composition selection
  for frame, sheet and check. Restore both composition and playhead after native errors.
- Render saved scenes in validated chunks with source/segment hashes, simulation replay,
  retained partial attempts and explicit reconciliation after a native bridge restart.
  Add `render --save` to save the named scene before submission.
- Flag SkSL/input redeclarations, pre-comp size, dense keys and group filters as structural
  risks. Add bounded scene node/key counts and key-volume warnings.
- Add SkSL, pre-comp, variable-font axes, two-key drivers, typed per-copy drivers, connected
  Look At and explicit above/below helpers. Font axes use ordered OpenType metadata.
- Add inclusive scene ranges and fps presets. Warn about installed/running bridge mismatch
  in version/status while keeping the plain version command offline.

- Prefer a complete page title at the start of an offline docs query, so contextual queries
  such as `Look At duplicator` find the requested node before unrelated body matches.
- Document per-copy JavaScript, SkSL input declarations, mode-dependent group opacity,
  variable-font axis indexing and animation hand-off traps from launch-video feedback.

## 1.1.0

- Recover scene opening through native loading and follow-up metadata with a configurable
  total timeout and scene input hash; resume without reopening the scene.
- Track frame/sheet exports with stable staging, resumable publication and playhead
  restoration after native PNG failures.
- Inspect referenced pre-comps without switching compositions, preserve composition
  paths in performance findings, and report incomplete membership scans explicitly.
- Select profile frames with bounded chronological simulation warm-up, low-resolution
  rendering and separate warm-up timings/progress. Explicitly overwrite Cavalry progress
  files so live status advances beyond the first checkpoint, and report refused writes
  once while allowing later checkpoints to recover.

- Persist render/check/run operations with phase-aware recovery, total wait budgets,
  duplicate prevention, read-only status and explicit unknown-outcome reconciliation.
- End waits promptly on refused bridge connections or changed sessions; expose unvalidated
  render artifacts and preserve partial output without retrying native work.
- Add cheap structural performance warnings, bounded checks, chronological measured
  profiling, partial progress and truthful failed/skipped coverage.
- Bind resumed visual checks to the original scene/comp, skip stillness conclusions for
  incomplete motion coverage, and preserve timeout recovery when ffmpeg/ffprobe is killed.
- Gate new submissions during resume against other pending operations, and derive total
  deadlines from parsed command options, including single-dash timeout flags.
- Bound script input preparation (including pipes) without abandoned reader goroutines,
  and never evaluate Duplicator counts after dependency inspection fails.
- Bound streaming audio hashes by the operation deadline and persist checkpoint renames
  and newly created directories before allowing native submission.
- Preserve signed macOS Cav.app bundles across install/update; require Developer ID,
  Hardened Runtime, timestamps, notarization, stapling and final artifact verification
  for release builds. Keep development credential-free and other platform layouts intact.

## 1.0.2

- The project is now called `cav`, and the repository moved to
  [rock3r/cav](https://github.com/rock3r/cav). Old links redirect, and `cav update` from 1.0.0
  and 1.0.1 still works.
- The plugin marketplace is now called `cav`: install the plugin with
  `/plugin install cavalry@cav` (Claude Code) or `codex plugin add cavalry@cav` (Codex). If
  you added the old `cavalry-skill` marketplace, remove it and add `rock3r/cav`.

## 1.0.1

Fixes found by installing 1.0.0 from the release and using it through the Claude Code and
Codex plugins.

- Inside an agent's sandbox, `cav` said the bridge was "not running" when the sandbox had
  blocked the connection. It now says `blocked: a sandbox` and explains the fix: allow
  `127.0.0.1:8723` and `~/.cav`, or use `cav relay`. `cav doctor` also checks that it can
  write its jobs folder.
- Through `cav relay`, the "job running" line came after the result, so agents waited for
  jobs that had already finished. The line now comes first, as in a direct run.
- `cav doctor` exited 0 even when it reported a problem. It now exits 2 when only the bridge is
  not running and 1 for other problems. With `--json` it prints one object.
- The install from the release, `cav setup` and the offline commands were tested on Windows 11
  (x64). Driving Cavalry on Windows is still not tested.

## 1.0.0

The first public release. It lets you, or a coding agent, build, check and render motion
graphics in Cavalry from a terminal.

### What is in it

| Part | What it does |
|---|---|
| `cav` CLI | Runs JavaScript inside a running Cavalry. Renders frames, contact sheets and MP4 videos (with audio). Inspects scenes and layers. Finds the tempo and beats of a music track. Searches the Cavalry API and docs offline. Prints guides on how to work (`cav guide`). Every command has `--json` output and fixed exit codes. |
| cav-bridge | A Cavalry UI script that the CLI talks to on `127.0.0.1:8723`. Requests need a secret token from `~/.cav/token`. |
| Helper library | The global `cav` inside scripts: shapes, text, keys and easing, entrances and exits, impacts, kinetic type, transitions, beat grids, and native features (duplicators, stagger, filters, 2.5D, particles). |
| `cav check` | Finds problems a viewer would notice before you render: long still stretches, small, clipped or hidden text, text that spills out of its button, off-frame layers, and empty frames at the start or end. |
| `cavalry` skill and plugins | A short entry point for agents. Packaged for Claude Code, Agent Plugins and Codex, with marketplace entries. |
| Sandbox relay | `cav relay` forwards commands through a folder, for agents whose sandbox blocks `127.0.0.1`. |
| Installers | One command on macOS and Windows. They check each download against the release checksums. `cav update` asks before it installs a new release. |

### Requirements

- Cavalry 2.4 or newer. Tested on 2.7.2 and 2.8.0.
- macOS. Windows builds and installers are included but **not tested** on Windows.
- ffmpeg, for videos with audio and for beat detection.
- Some native features need a Cavalry Pro licence. `cav.pro(type)` tells you.

### Measured results

In a blind evaluation on Cavalry 2.8.0 with a mid-size model (GLM-5.3 Flash), agents that
used `cav` passed 8 of 11 tasks, and agents that used the upstream cavalry-mcp server passed
3 of 11. The `cav` agents used about a third of the tokens. The skill did not improve on the
CLI alone, so this release moves its guidance into `cav guide`. Details: `docs/evaluation.md`.

### Known limits

- The bridge runs on Cavalry's single script thread. One slow script delays every other
  request.
- Starting or restarting the bridge needs the Cavalry window (Scripts menu).
- On Cavalry 2.8.0, inside the script run that creates a layer, a child's world bounding box
  does not follow an animated parent. Read it again in a later `cav run`.
- Connecting the Spring behaviour from a script had no visible effect in our tests. Use
  `spring` or `outBack` easing instead.
