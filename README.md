# cavalry-skill

Tools that let coding agents make motion graphics in [Cavalry](https://cavalry.studio), the 2D
motion design app. There are three parts:

| Part | What it is |
|---|---|
| `cav` | A command-line tool. It runs JavaScript inside a running Cavalry, renders frames, contact sheets and videos, inspects scenes, finds music beats, and searches the API and docs offline. |
| The `cavalry` skill | Instructions for agents: the workflow, the traps, and motion design recipes. |
| Plugin packaging | The skill as a Claude Code plugin, an [Agent Plugins](https://agent-plugins.org) plugin and a Codex plugin, with marketplace entries. |

There is no MCP server. Driving Cavalry always needs a desktop with Cavalry running, so a shell
is always there. A CLI costs the agent no context until it asks for help.

## Install

**1. The CLI.** macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/rock3r/cavalry-skill/main/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/rock3r/cavalry-skill/main/install.ps1 | iex
```

The installer checks the download against `checksums.txt`, then runs `cav setup`. Setup is safe
to run again. It creates a secret token, copies the bridge script into Cavalry's Scripts folder,
checks Cavalry and ffmpeg, and builds the offline docs index.

**2. Start the bridge.** In Cavalry, click **Scripts > cav-bridge** and keep its small window open.
The first time, Cavalry asks whether you trust the script. Then check everything:

```bash
cav doctor
```

**3. The plugin (optional).** In Claude Code:

```text
/plugin marketplace add rock3r/cavalry-skill
/plugin install cavalry@cavalry-skill
```

Other agents: copy `plugins/cavalry/skills/cavalry` into the agent's skills folder
(OpenCode reads `.opencode/skills/`, `.claude/skills/` and `.agents/skills/`).

## Update

```bash
cav version --check    # reports a newer release, changes nothing
cav update             # shows the release, asks, verifies the checksum, then runs cav setup
```

cav never updates itself without asking. After an update, if `cav doctor` says the running bridge
is older, close the cav-bridge window in Cavalry and start it again. The plugin updates through
the marketplace: `/plugin marketplace update cavalry-skill`.

## Commands

| Command | What it does |
|---|---|
| `cav setup` / `cav doctor` | Install or check everything (token, bridge script, Cavalry, ffmpeg, docs, running bridge) |
| `cav status` | Bridge version, scene path, unsaved changes, comp size, fps and frame range |
| `cav scene new\|comp\|open\|save` | New scene with comp settings (refuses to discard unsaved work), change comp, open, save |
| `cav run <file.js>... \| -e <code> \| -` | Run JavaScript in Cavalry with the helper library preloaded; prints the return value |
| `cav job wait <id>` | Wait for a long job; a timeout means "still running", not "failed" |
| `cav tree` / `cav layer <id>` | Layer tree; one layer's parent, bounding box, transform, animated attributes and keys |
| `cav frame [n...]` | Render frames to PNG |
| `cav sheet [frames]` | Render frames into one labelled contact sheet (optionally with beat numbers) |
| `cav render [-o out.mp4] [--audio track]` | Render the comp to MP4, check the frame count, mux audio with ffmpeg |
| `cav beats <audio>` | Tempo, beats and downbeats as frame numbers |
| `cav api <words>` / `cav docs <words>` / `cav types <word>` | Offline search: API signatures, Cavalry docs, layer type ids |
| `cav helpers [word]` | Reference for the helper library (the global `cav` inside scripts) |
| `cav relay --spool <dir>` | Forward jobs for agents whose sandbox blocks 127.0.0.1 |
| `cav version [--check]` / `cav update` | Versions and updates |

Every command accepts `--json`. Exit codes: 0 ok, 1 error, 2 bridge not reachable, 3 job still
running, 4 bridge lost. Set `CAV_LOG=file.jsonl` to log every call.

## How it works

```
agent shell ── cav ── HTTP 127.0.0.1:8723 (token) ──> cav-bridge.js (UI script in Cavalry)
                 │                                       runs the job, publishes the result,
                 └─ reads ~/.cav/jobs/<id>.json <───────  and writes it to ~/.cav/jobs/<id>.json
```

- Jobs are files: the CLI writes the script to `~/.cav/jobs/`, and the bridge reads it. Results
  also go to a file, so a long job survives a client timeout (`cav job wait`).
- The bridge answers status requests while a job runs, so cav can tell "running" from "gone".
- Round trip: about 60 ms for a small job (measured on an M-series Mac, Cavalry 2.7.2).
- The helper library is sent once per bridge session and reloaded when its version changes.

## What was verified

Tested on macOS with Cavalry 2.7.2. Windows paths and scripts are written but **not tested** on
Windows. The helper library has unit tests (Node, mock API) and live tests (58 checks inside
Cavalry). The skill's `references/` folder lists the verified traps and recipes.

## Credits

- [cavalry-mcp](https://github.com/m18h/cavalry-mcp) by Michael Essandoh (MIT): the bridge script
  is derived from its bridge, and the token handshake, API reference and docs search ideas come
  from it. See `NOTICE`.
- [cavalry-types](https://github.com/scenery-io/cavalry-types) by Remco Janssen (MIT): API names
  and signatures.
- Cavalry is made by Scene Group (now part of Canva). This project is not affiliated with them.
  The Cavalry docs are not included; `cav docs update` downloads them to your own computer.

## Development

```bash
go test ./...                    # Go tests (search, beats, packaging contracts)
node --test assets/helpers/test  # helper unit tests
cav scene new --force --width 1280 --height 720 --fps 30 --seconds 3   # throwaway scene!
cav run assets/helpers/test/live.js                                    # live helper tests
claude plugin validate . --strict
```

Releases: see [RELEASING.md](RELEASING.md). Licence: MIT.
