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

Documentation: the [user guide](docs/user-guide.md) covers installing, the workflow, music
sync, agents and troubleshooting. [Architecture](docs/architecture.md) explains how the parts
fit together, for people who change the code, and [evaluation](docs/evaluation.md) describes
how we measure it.

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

Tested on macOS with Cavalry 2.7.2. On 2026-10-01, live tests also passed on Cavalry 2.8.0:
65 core helper checks and 14 native helper checks (measured). Windows paths and scripts are
written but **not tested** on Windows. The skill's `references/` folder lists the verified
traps and recipes.

## Evaluation

The clean 33-run comparison is running through Pioneer on Cavalry 2.8.0 as
`pioneer-final4`. It uses `zai/glm-5.3-flash`, medium thinking, and the same frozen CLI
and skill for all three arms. Setup, provider and bridge failures are excluded from task
results. The historical numbers below remain in place until the full comparison is audited.

The clean comparison uses GPT 6.1 Sol (`gpt-6.1-sol`) through Codex for blind judging.
Each available pair is judged in fresh sessions in both image orders. These verdicts
remain separate from the historical Sonnet results below.

We measured the toolkit on 11 motion-design tasks with GLM-5.3 Flash (`zai/glm-5.3-flash`, run
through the pi agent, 30-minute limit per run, one new scene per run). Eight tasks were used
while we improved the toolkit: a logo sting, a kinetic title, a lower third, a bar chart, a UI
walkthrough, a beat-synced loop, a transition pack and a promo cut to music. Three tasks were
written later and never used for tuning (held out): a square quote card, a map route and a
countdown to music.

| Arm | What the agent gets |
|---|---|
| plugin | the `cav` CLI and this skill |
| baseline | the `cav` CLI only (it can read `cav help` and `cav helpers`) |
| cavalry-mcp | the upstream cavalry-mcp server |

A script checks each run: the scene was saved, and the video has the right size, frame rate,
duration, audio, frame coverage, motion, easing and (for music) beat sync. For quality, a
separate model (Claude Sonnet) compared the two arms' contact sheets without knowing which arm
made which. It judged every pair twice, the second time with the order swapped.

Historical comparison, same code for both arms (measured on Cavalry 2.7.2):

| Arm | Cavalry | Automatic checks passed | Blind wins | Rubric claims met | Mean time | Mean tokens |
|---|---|---|---|---|---|---|
| plugin | 2.7.2 | 12 of 14 runs | 10 | 118 of 130 | 833 s | 1.17 M |
| baseline | 2.7.2 | 14 of 14 runs | 16 | 111 of 130 | 864 s | 1.01 M |

The upstream cavalry-mcp server passed 1 of 8 tasks in the last run that included it, but
that number is not valid. During that run, one cavalry-mcp agent found the cavalry-mcp bridge
down. It started a port forwarder from the cavalry-mcp port to cav-bridge and copied the cav
token over the cavalry-mcp token. From then on, every cavalry-mcp call went to cav-bridge. A
clean rerun is pending. The plugin and baseline numbers are not affected: their jobs always
ran through cav-bridge. See [docs/evaluation.md](docs/evaluation.md) for details.

What the historical runs showed:

- The CLI does most of the work. With `cav help`, the helper library and `cav check`, a
  mid-size model produces clean, animated, correct videos without the skill.
- The skill did not make the videos better in the final blind comparison. It won the repeated
  tasks (5 to 1) but lost the first pass (5 to 15), including all held-out tasks. Earlier
  iterations showed it using fewer tokens and making fewer script errors; the final run did not
  confirm that.
- Both plugin failures were a 30-minute timeout on the beat loop and a missed beat-sync check.
  In timeouts the model spent 9 to 10 minutes planning before its first command.
- The judge's two passes agreed on 9 of 13 pairs, and single runs flip between pass and fail.
  Differences of one or two tasks are noise.
- Free models: Nemotron 3.5 Lightning (OpenRouter, free) passed 0 of 8 with the plugin: it
  ignored the review loop and stacked everything in the middle of the frame. Space Bunny could
  not be measured: the free endpoint returned empty responses.

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
node --test assets/helpers/test/*.test.js  # helper unit tests
node --test assets/bridge/test/*.test.js   # the bridge adds no globals and restores api after a job
cav scene new --force --width 1280 --height 720 --fps 30 --seconds 3   # throwaway scene!
cav run assets/helpers/test/live.js                                    # live helper tests
claude plugin validate . --strict
```

Releases: see [RELEASING.md](RELEASING.md). Licence: MIT.
