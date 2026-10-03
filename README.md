# cav

Make motion graphics in [Cavalry](https://cavalry.studio), the 2D motion design app, from a
terminal. You, or a coding agent working for you, write short scripts. `cav` runs them inside
the Cavalry window that is open on your desktop, shows you contact sheets, checks the result
for common mistakes, and renders the video, with music if you want.

| Part | What it is |
|---|---|
| `cav` | A command-line tool for macOS and Windows. It runs JavaScript in Cavalry, renders frames, contact sheets and MP4 videos, inspects scenes, finds the beats in a music track, and searches the Cavalry API and docs offline. `cav guide` explains the workflow and has motion design recipes. |
| cav-bridge | A small Cavalry script that `cav` talks to. `cav setup` installs it. |
| `cavalry` skill | A short entry point for coding agents. It checks the setup and sends the agent to `cav guide`. Packaged as a Claude Code plugin and a Codex plugin. |

There is no MCP server. Cavalry always runs on a desktop, so a shell is always there, and a
command-line tool costs an agent no context until it asks for help.

## Requirements

- Cavalry 2.4 or newer. Tested on 2.7.2 and 2.8.0.
- macOS or Windows. See [What was tested](#what-was-tested) for the details on Windows.
- ffmpeg, for videos with audio and for beat detection (`brew install ffmpeg`, or
  `winget install Gyan.FFmpeg`).

## Install

**1. Install `cav`.** On macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/rock3r/cav/main/install.sh | sh
```

On Windows, in PowerShell:

```powershell
irm https://raw.githubusercontent.com/rock3r/cav/main/install.ps1 | iex
```

The installer checks the download against the release checksums, then runs `cav setup`.

**2. Start the bridge.** In Cavalry, click **Scripts > cav-bridge** and keep its small window
open. Do this once each time you start Cavalry. Then check everything:

```bash
cav doctor
```

**3. Optional: add the agent plugin.** In Claude Code:

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

Other agents: copy `plugins/cavalry/skills/cavalry` into the agent's skills folder.

**Agents in a sandbox.** Some agent sandboxes block connections to `127.0.0.1` or writes
outside the project. `cav doctor` then reports `blocked: a sandbox`. Either allow `cav` to
reach `127.0.0.1:8723`, or run `cav relay` outside the sandbox and let `cav` work through a
folder: see [Agents in a sandbox](docs/user-guide.md#agents-in-a-sandbox).

## Try it

```bash
cav scene new --width 1920 --height 1080 --fps 60 --seconds 4
cav guide            # the workflow, with a first script to copy
cav run title.js     # run your script inside Cavalry
cav sheet            # look at 12 frames in renders/sheet.png
cav check            # find problems a viewer would notice
cav render -o out/title.mp4
```

The [user guide](docs/user-guide.md) walks through a first animation, music sync and
troubleshooting.

## Commands

| Command | What it does |
|---|---|
| `cav setup` / `cav doctor` | Install or check everything: token, bridge script, Cavalry, ffmpeg, docs, running bridge |
| `cav guide [topic]` | How to work: workflow, design recipes, music sync, traps, native features |
| `cav status` | Existing bridge state; no scene work is queued |
| `cav scene new\|comp\|open\|save` | New scene (refuses to discard unsaved work), change the comp, open, save |
| `cav run <file.js>... \| -e <code>` | Run JavaScript in Cavalry with the helper library loaded; prints the return value |
| `cav job wait [<id>]` | Wait for one raw job without continuing command phases |
| `cav operation status/resume <id>` | Inspect or continue a complete scene open, frame, sheet, render, check or run after a timeout |
| `cav tree` / `cav layer <id>` | The layer tree; one layer's position, bounding box and keys |
| `cav frame [n...]` / `cav sheet [frames]` | Render frames to PNG, or a labelled contact sheet |
| `cav check [--quick] [--profile]` | Structural performance warnings and bounded visual/measured checks |
| `cav render [-o out.mp4] [--audio track]` | Render to MP4, check the frame count, add the music |
| `cav beats <audio>` | Tempo, beats and downbeats as frame numbers |
| `cav api` / `cav docs` / `cav types` / `cav helpers` | Offline search: API, Cavalry docs, layer types, the helper library |
| `cav relay --spool <dir>` | Forward commands for agents whose sandbox blocks `cav` |
| `cav version [--check]` / `cav update` | Versions, and updates that ask before they install |

Every command accepts `--json`. Exit codes: 0 ok, 1 error, 2 bridge not reachable, 3 job still
running, 4 bridge lost.

## Update

```bash
cav update
```

It shows the new release, asks, checks the download, and runs `cav setup`. If `cav doctor`
then says the running bridge is older, close the cav-bridge window in Cavalry and start it
again. Update the plugin with `/plugin marketplace update cav` in Claude Code.

## How well it works

We gave the same 11 motion design briefs to an agent with a mid-size model (GLM-5.3 Flash) in
three ways, and compared the results with automatic checks and a blind judge (measured on
Cavalry 2.8.0):

| Agent used | Tasks passed | Mean time | Mean tokens |
|---|---|---|---|
| `cav` and the skill | 8 of 11 | 1,177 s | 1.22 M |
| `cav` only | 8 of 11 | 1,025 s | 0.98 M |
| the upstream cavalry-mcp server | 3 of 11 | 1,604 s | 3.22 M |

`cav` did most of the work: with or without the skill, the agent passed far more tasks with a
third of the tokens. The skill did not add a measurable gain, so 1.0 moved its guidance into
`cav guide`, where every agent can read it. The method, the full tables and the problems we
found are in [the evaluation](docs/evaluation.md).

## What was tested

| What | macOS | Windows |
|---|---|---|
| Install from the release, `cav setup`, offline commands | tested | tested (Windows 11, x64) |
| `cav update` from 1.0.0 to 1.0.1 | tested | tested |
| Driving Cavalry: jobs, sheets, checks, renders | tested (65 helper and 14 native-feature checks live) | not tested |
| Claude Code and Codex plugins, with a sandboxed agent through `cav relay` | tested | not tested |

## Documentation

- [User guide](docs/user-guide.md): install, the first animation, music, agents, troubleshooting.
- [Architecture](docs/architecture.md): how the parts fit together, for people who change the code.
- [Evaluation](docs/evaluation.md): how we measure it, and the results.
- [Changelog](CHANGELOG.md) and [releasing](RELEASING.md).

## Development

```bash
go test ./...
node --test assets/helpers/test/*.test.js assets/bridge/test/*.test.js
cav scene new --force --width 1280 --height 720 --fps 30 --seconds 3   # a throwaway scene
cav run assets/helpers/test/live.js                                    # live checks in Cavalry
claude plugin validate . --strict
```

## Credits

- [cavalry-mcp](https://github.com/m18h/cavalry-mcp) by Michael Essandoh (MIT): the bridge
  script is derived from its bridge, and the token handshake, API reference and docs search
  ideas come from it. See `NOTICE`.
- [cavalry-types](https://github.com/scenery-io/cavalry-types) by Remco Janssen (MIT): API names
  and signatures.
- Cavalry is made by Scene Group (now part of Canva). This project is not affiliated with them.
  The Cavalry docs are not included; `cav docs update` downloads them to your own computer.

Licence: MIT.

## Recovery, diagnostics and signed macOS bundles

`scene open`, `frame`, `sheet`, `render`, `check` and `run` return an operation ID. After a timeout, use
`cav operation status <id>` and `cav operation resume <id> --timeout 90m` instead of
retrying the command. `cav check --quick --json` avoids playhead changes, bounds and renders;
`--profile` adds a small measured frame set. Quick inspection follows referenced pre-comps
without switching the active comp; findings include their composition paths. Select later
frames with `--profile-frames 840,1560,1908`. Simulations require chronological PNG warm-up;
explicitly allow those extra frames with `--max-warmup 2000` and an appropriate timeout. Failures and skipped coverage are explicit.
A between-sample profile budget limit remains skipped coverage, including when no sample
finishes; it is not an API failure.
See [operation recovery](docs/recovery.md) for states, concurrency and remaining limits.

macOS releases preserve a signed, notarized and stapled `Cav.app`; the installer exposes
its executable on PATH through a symlink. Users of an already-shipped older updater should
run the current installer once to migrate. Development builds stay credential-free; real
release builds require signing. See [notarization](docs/NOTARIZATION.md).
