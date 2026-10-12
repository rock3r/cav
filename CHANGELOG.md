# Changelog

## Unreleased

- The `cavalry` plugin now works as an Antigravity plugin. Install it in the Antigravity CLI
  with `agy plugin install ./plugins/cavalry` from a clone of this repository.
- Antigravity starts `cav mcp` from the new `mcp_config.json`. It has the same content as
  `.mcp.json`, and a test keeps the two files the same.
- `cav review hook --host antigravity --event PreInvocation|Stop` is the review hook for
  Antigravity. It reads Antigravity's events and looks for review files in each workspace
  folder. Before a model call, it adds a message about new notes. When the agent wants to
  stop, it asks the agent to continue and read them. It tells the agent about each send
  once. So it does not repeat the notes before every model call, and it does not keep the
  agent running.
- The plugin's root `hooks.json` installs this hook in Antigravity. Only the Antigravity CLI
  runs plugin hooks. The Antigravity app and IDE do not.
- We have not checked whether Antigravity draws MCP Apps. If it does not, `show_review`
  still gives the agent the open notes as text, and `cav review` shows the page in a
  browser.

## 1.4.0

- `cav music gen --service yue2` makes instrumental takes with YuE2, an open music model,
  on a server you run. The server is new: `tools/yue2-server` loads YuE2 once and keeps it
  in memory. It runs on an Apple silicon Mac and on an NVIDIA GPU with 12 GB or more. Set
  it with `cav config endpoint yue2 http://127.0.0.1:8002`. The server's routes and fields
  are a subset of YuE2-Turbo's API. Its instrumental mode is an extension that Turbo does
  not have.
- YuE2 makes songs with vocals. For cav, the server plans a score, moves the vocal notes
  to an instrument, and generates again, as the instrumental helper in the YuE2
  repository does.
- YuE2 has no length setting, but its audio follows the score it plans. The server cuts
  that score to about `--seconds` at a phrase (every four bars) or a section, and keeps
  the outro. cav says how long the take came out, and when YuE2 stopped part of a take at
  its token limit.
- cav converts YuE2's FLAC to 24-bit WAV.
- cav records YuE2's real terms with each take: the weights are CC BY-NC 4.0, and an
  individual creator or musician may still use and sell the music. A company needs a
  commercial licence from the YuE2 authors. cav cannot tell the two apart, so it treats a
  take as non-commercial. A project set to `cav credits licence nc-ok` accepts it.
  Otherwise `cav music gen` warns, and `cav credits check` lists the take.
- With a reference track (`--ref`), YuE2 makes an instrumental cover. cav uploads the
  track to the server's new `POST /v1/covers` route, which has the same fields as
  YuE2-Turbo's. SheetSage2, a transcription model, writes down the track's melody. YuE2
  then plays that melody on an instrument, in the style of the prompt. It does not copy
  the singer or the arrangement.
- SheetSage2 runs in its own Python environment next to YuE2, on CUDA, on an Apple
  silicon Mac (MPS) or on the CPU. Without it, the server's covers are off, and cav
  describes the track in words instead, as for Lyria and ElevenLabs. cav says why.
- A cover plays someone else's song. cav records the reference track with the take, next
  to YuE2's licence, and `cav credits` lists a credit line for the song. You need the
  songwriter's permission or a cover licence to use a cover.
- `cav music gen --lyrics song.txt` makes a song with vocals. Mark the sections of the file
  with tags such as `[Verse]` and `[Chorus]`. Only YuE2 and ACE-Step can sing. The other
  services refuse `--lyrics` with an error that names the two that can. Without
  `--service`, cav tries only YuE2 and ACE-Step for a song. YuE2 takes up to 16000
  characters of lyrics and ACE-Step up to 4096.
- `.cav/manifest.json` records the lyrics of each take with lyrics, next to its prompt.
- With YuE2, the lyrics set the length of a song, and cav does not send `--seconds`. The
  server cannot cut a sung score without stopping in the middle of a line, because the
  score has the vocal notes but not the words. ACE-Step keeps `--seconds`, so lyrics that
  are too long can stop in the middle of a line.
- With `--lyrics` and `--ref`, YuE2 makes a sung cover: it sings the new lyrics to the
  melody of the track.
- Music is instrumental unless you give `--lyrics`. The help, `cav guide production`, the
  user guide and the skill now say so.
- The `yue2` service also works with a YuE2-Turbo server. Turbo is NOIZ's fast server for
  YuE2. With vLLM on a large NVIDIA GPU it uses about 18 GB at rest. Without vLLM, and with
  the low-memory settings in `cav guide production`, it also runs on a 12 GB GPU and on an
  Apple silicon Mac. Turbo has no instrumental mode and no length cut, so cav does that work
  itself. One job plans a score. cav moves the vocal notes to an instrument and cuts the
  score to about `--seconds`. A second job then makes the audio from that score. A cover
  works the same way: the cover job's transcribed melody takes the place of the planned
  score. A song with `--lyrics` needs none of this, so it is one job on Turbo too.
- cav needs Turbo's API key. Give it with `YUE2_API_KEY` or `cav config set-key yue2`.
  cav never prints the key.
- `cav config check` now tells cav's server and Turbo apart, and checks Turbo's key. It no
  longer says that a Turbo server cannot make instrumentals.
- cav was tested against a real Turbo server (commit 7c88813) on an RTX 3080 Ti, with an
  instrumental, a song with lyrics and a cover, and on an M1 Max with an instrumental. A
  large GPU with vLLM was not tested. `cav guide production` has the times.
- When cav or its server cuts a YuE2 score, a note held across the cut now keeps its part
  after the cut. Before, it was dropped, and left a rest at the start of the outro.
- A cut never keeps only silent bars, even when they are closest to `--seconds`.
- In the `cav review` page, the cursor in a note you are writing no longer jumps to the
  end of the text by itself. The page moved the note row on every layout, even when it
  was already in place. Now it moves the row only when the playhead passes a note, and
  keeps the cursor where it was.

## 1.3.0

- In `cav review` and the chat review, each note has an Edit action. You edit the text in
  place and save with Cmd/Ctrl+Enter or Save. The server refuses to empty the text of a
  note that has no drawing. Deleting a note keeps the sheet's scroll position, and a long
  note count in a narrow sheet ends with an ellipsis.
- A note's text can be at most 65536 characters, both when you create it and when you
  edit it. Before, a long note could be created and then fail to save.
- In the chat review page, the armed delete button uses the host's danger background
  (`--color-background-danger`) with its danger text colour (`--color-text-danger`) on
  top. Before, it used the danger text colour as the fill, so its label and icon could
  lose contrast. The button follows the host only when the host sends both colours. The
  `cav review` page in the browser looks the same as before.
- In ChatGPT and Codex, the chat review page opens full screen. Its resource names the
  display modes in `_meta["openai/ui"]`, so the host can choose one before the page
  starts. The page's own full screen button is hidden there, because those hosts have
  their own control.
- The chat review page opens inline and has a full screen button when the host offers
  full screen. In full screen the stage and the note sheet fill the window, and the page
  keeps clear of the chat composer. The page also takes the host's theme: it has no
  border or background of its own, and it uses the host's colours and font for its chrome.
  The stage stays black and the playhead stays orange. If you pick a theme in the page
  that differs from the chat's, the page paints its own background.
- `cav music gen --ref <track>` (repeatable) makes variations of a reference track, with
  `--takes` takes for each one. Stable Audio gets the track through audio-to-audio, and
  `--keep` (0-1) sets how much of it stays. ACE-Step gets it as a style reference. Lyria
  and ElevenLabs cannot take audio, so the audio model describes the track in words and
  cav adds that to the prompt. cav reports which way each reference went, and the
  manifest records the reference with each take. For Stable Audio, cav converts a
  reference that is not MP3 or WAV and sends at most its first 190 seconds.
- `cav music gen` lowers the level of a take whose true peak is above -1 dBTP, so it does
  not clip (gain only). `--raw-level` keeps the level the service sent.
- `cav music gen` with ElevenLabs sends only the shot ids as section names. The shot
  descriptions named products, and ElevenLabs refused the whole plan for that.
- Lyria takes now come as WAV: cav asks for `audio/wav` in the response format. Before,
  Lyria sent MP3.
- `cav config check` says when an ElevenLabs key is on the free plan, which cannot make
  music through the API.
- `cav guide production` has a table of what each music service does differently.
- Redesign the review page as an animator's exposure sheet. Notes sit in rows by frame,
  beside the stage, with snapshots at one scale. Frames with no note show as ruled paper
  whose height grows with the run. You write a note in the orange playhead row, which
  moves with the frame. The page draws its own icons, puts labels and shortcuts in
  tooltips, follows the system's light or dark theme, and fits from a phone to a desktop
  and in a chat frame. Replies are written inline, and deleting takes a second click
  instead of a confirm box, so both work in chat frames.
- `cav review` finds a note's snapshot by name in the review's own folder, so a review
  opened from another folder (the chat's server, for one) shows its snapshots, and a
  review file cannot make the server send a file from elsewhere.
- Add `cav mcp`: a small MCP server (stdio) that shows the `cav review` page inside the
  chat as an MCP App, in hosts that draw MCP Apps, such as the chat of the Claude desktop
  app (its Code tab does not). The page plays a 640-pixel preview sent inline, and its
  notes go to the same review file. `review_notes` reads the sent notes with their
  snapshot images, for chats without a shell. The cavalry plugin starts it in Claude Code and Codex; without the plugin,
  register it with `claude mcp add cav -- cav mcp` or `codex mcp add cav -- cav mcp`.
- In `cav review` and `cav mcp`, a note now stays on the render the page showed when you
  made it, also when a re-render goes live at the same moment. The page sends the version
  it shows with each note. In the chat, a re-render goes live only when the page asks for
  the review state.
- In the chat review, a re-render whose preview cannot load (for example over 24 MB) no
  longer leaves the earlier render on the stage. The page stops adding notes until a
  render has loaded.
- When cav cannot reach cav-bridge, `cav doctor` and the errors of every other command
  now say whether Cavalry itself is running. "Cavalry is not running (it may have
  crashed)" names the newest Cavalry crash report on macOS, if there is one. "Cavalry is
  running but cav-bridge is not" asks you to start only the bridge.
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
  `cav score render` to render it from the command line. Tested with REAPER 7.82 on macOS
  and Windows. The video track is silent, so the render's own soundtrack stays out of the
  music WAV. When REAPER has no audio device yet, `cav score render` selects REAPER's
  default first, so REAPER does not stop to ask.
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
