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
| `cav job wait [<id>]` | Wait for a long job (default: the last one); a timeout means "still running", not "failed" |
| `cav tree` / `cav layer <id>` | Layer tree; one layer's parent, bounding box, transform, animated attributes and keys |
| `cav frame [n...]` | Render frames to PNG |
| `cav sheet [frames]` | Render frames into one labelled contact sheet (optionally with beat numbers) |
| `cav check` | Find common problems before rendering: long still stretches, tiny or clipped text, off-frame layers, empty start or end |
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
Windows. The helper library has unit tests (Node, mock API) and live tests (74 checks inside
Cavalry). The skill's `references/` folder lists the verified traps and recipes.

## Evaluation

We measured the skill on 8 motion-design tasks: a logo sting, a kinetic title, a lower third,
a bar chart, a UI walkthrough, a beat-synced loop, a transition pack and a promo cut to music.
Each run starts from a new scene. The model is GLM-5.3 Flash (`zai/glm-5.3-flash`, run through
the pi agent) with a 30-minute limit. Three arms get the same task text:

| Arm | What the agent gets |
|---|---|
| plugin | the `cav` CLI and this skill |
| baseline | the `cav` CLI only (it can read `cav help`) |
| cavalry-mcp | the upstream cavalry-mcp server |

A run passes when a script confirms the scene was saved and the video has the right size,
frame rate, duration, audio, frame coverage, motion, easing and (for music tasks) beat sync.
We also graded every contact sheet by eye (1 to 5).

Results of iteration 6, the last one with all three arms on the same code (measured):

| Arm | Passed | Mean grade | Mean time | Mean tokens | Script errors |
|---|---|---|---|---|---|
| plugin | 6 of 8 | 3.7 | 820 s | 0.70 M | 1 |
| baseline | 5 of 8 | 3.6 | 1149 s | 1.54 M | 9 |
| cavalry-mcp | 1 of 8 | 2.5 | 1494 s | 3.88 M | 0 |

The plugin arm of iteration 7 passed 7 of 8 tasks (mean grade 3.7, 742 s, 0.75 M tokens).

What this shows:

- The skill makes the agent faster and cheaper: about 30 % less time and half the tokens of the
  baseline, with far fewer script errors.
- The CLI alone already helps a lot: the baseline passes most tasks. The skill mostly adds
  polish (easing, beat sync, readable sizes, living holds) and fewer mistakes.
- Every failed run in the last iterations was a 30-minute timeout. In each one the model spent
  9 to 10 minutes planning before its first command. Slow scripts in heavy scenes did the rest.
- Each cell is one run per task, so a single task can flip between iterations. Treat
  differences of one task as noise.
- The scoring rules changed between the early iterations, so compare arms inside one
  iteration, not across iterations.

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
