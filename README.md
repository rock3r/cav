# cavalry-skill

Tools that let coding agents make motion graphics in [Cavalry](https://cavalry.studio), the 2D
motion design app. There are three parts:

| Part | What it is |
|---|---|
| `cav` | A command-line tool. It runs JavaScript inside a running Cavalry, renders frames, contact sheets and videos, inspects scenes, finds music beats, and searches the API and docs offline. `cav guide` prints the workflow, the traps and motion design recipes. |
| The `cavalry` skill | A short entry point for agents: it checks the setup and sends the agent to `cav guide`. |
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
| `cav guide [topic]` | How to work: workflow, design recipes, music sync, traps, native features |
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
- The bridge publishes job state and stores results. When Cavalry is busy, `cav` waits
  and reports that it is not answering.
- Round trip: about 60 ms for a small job (measured on an M-series Mac, Cavalry 2.7.2).
- The helper library is sent once per bridge session and reloaded when its version changes.

## What was verified

Tested on macOS with Cavalry 2.7.2. On 2026-10-01, live tests also passed on Cavalry 2.8.0:
65 core helper checks and 14 native helper checks (measured). Windows paths and scripts are
written but **not tested** on Windows. `cav guide traps` and `cav guide native` list the
verified traps and recipes.

## Evaluation

Measured on Cavalry **2.8.0**, with `zai/glm-5.3-flash`, medium thinking, through
Pioneer's native sandbox. Each run had a new scene and a 30-minute deadline. The CLI
and skill were frozen at `cadd3c6`. Pioneer was 0.4.4 and Pi was 0.84.2.

There are 11 original tasks per arm: eight used for tuning and three held out
(quote-card, map-route and countdown). The plugin arm gets the CLI and skill;
baseline gets the CLI alone; cavalry-mcp gets the upstream MCP server, with the CLI hidden.
Infrastructure and provider failures were excluded and replaced with the same setup.
Normal task deadlines remain in the sample. Missing deliverables fail automatic checks.

GPT 6.1 Sol (`gpt-6.1-sol`) through Codex CLI 0.159.2 judged every available contact-sheet
pair in two fresh sessions, with swapped order and no tools or answer-key access.
Wins count judgments, rather than tasks. Claims are the judge's marks; unknown claims
remain in the denominator. Sheets cannot establish audio, exact beat timing, fidelity
to input fixtures or vector construction. Automatic audio and beat checks are separate.
Pairs without final videos have no blind verdict; their automatic failures remain counted.

All figures below are **measured**. Mean tokens include input, output and cached reads.
Mean times include the normal deadlines. Original and repeat runs are never pooled.

### All tasks

| Arm | Cavalry | Measured runs | Automatic passes | Mean time (s) | Mean tokens | Timeouts |
|---|---|---|---|---|---|---|
| plugin | 2.8.0 | 11 | 8/11 | 1,176.7 | 1,219,232 | 1 |
| baseline | 2.8.0 | 11 | 8/11 | 1,025.3 | 982,121 | 1 |
| cavalry-mcp | 2.8.0 | 11 | 3/11 | 1,604.0 | 3,221,162 | 8 |

#### Blind comparison: plugin vs baseline

Judge: gpt-6.1-sol through Codex. Each available pair was judged in both orders.

| Arm | Cavalry | Wins | Claims met | Unknown claims |
|---|---|---|---|---|
| plugin | 2.8.0 | 9/18 | 63/90 | 22 |
| baseline | 2.8.0 | 9/18 | 66/90 | 22 |

Ties: 0/18. Both passes agreed on 8/9 pairs. Unknown claims are included in the claim denominator.

Not judgeable: promo-music (plugin: final video missing). Its automatic result is retained.

Not judgeable: ui-walkthrough (baseline: final video missing). Its automatic result is retained.

Pairs selected for a separate once-only repeat: beat-loop.

#### Blind comparison: plugin vs cavalry-mcp

Judge: gpt-6.1-sol through Codex. Each available pair was judged in both orders.

| Arm | Cavalry | Wins | Claims met | Unknown claims |
|---|---|---|---|---|
| plugin | 2.8.0 | 6/6 | 30/30 | 0 |
| cavalry-mcp | 2.8.0 | 0/6 | 28/30 | 2 |

Ties: 0/6. Both passes agreed on 3/3 pairs. Unknown claims are included in the claim denominator.

Not judgeable: beat-loop (cavalry-mcp: final video missing). Its automatic result is retained.

Not judgeable: countdown (cavalry-mcp: final video missing). Its automatic result is retained.

Not judgeable: kinetic-title (cavalry-mcp: final video missing). Its automatic result is retained.

Not judgeable: logo-sting (cavalry-mcp: final video missing). Its automatic result is retained.

Not judgeable: map-route (cavalry-mcp: final video missing). Its automatic result is retained.

Not judgeable: promo-music (plugin: final video missing; cavalry-mcp: final video missing). Its automatic result is retained.

Not judgeable: transition-pack (cavalry-mcp: final video missing). Its automatic result is retained.

Not judgeable: ui-walkthrough (cavalry-mcp: final video missing). Its automatic result is retained.


### Held-out tasks

| Arm | Cavalry | Measured runs | Automatic passes | Mean time (s) | Mean tokens | Timeouts |
|---|---|---|---|---|---|---|
| plugin | 2.8.0 | 3 | 3/3 | 1,166.7 | 1,863,384 | 0 |
| baseline | 2.8.0 | 3 | 3/3 | 841.2 | 792,833 | 0 |
| cavalry-mcp | 2.8.0 | 3 | 1/3 | 1,437.0 | 3,291,794 | 2 |

#### Blind comparison: plugin vs baseline

Judge: gpt-6.1-sol through Codex. Each available pair was judged in both orders.

| Arm | Cavalry | Wins | Claims met | Unknown claims |
|---|---|---|---|---|
| plugin | 2.8.0 | 4/6 | 20/30 | 9 |
| baseline | 2.8.0 | 2/6 | 20/30 | 10 |

Ties: 0/6. Both passes agreed on 3/3 pairs. Unknown claims are included in the claim denominator.

#### Blind comparison: plugin vs cavalry-mcp

Judge: gpt-6.1-sol through Codex. Each available pair was judged in both orders.

| Arm | Cavalry | Wins | Claims met | Unknown claims |
|---|---|---|---|---|
| plugin | 2.8.0 | 2/2 | 10/10 | 0 |
| cavalry-mcp | 2.8.0 | 0/2 | 10/10 | 0 |

Ties: 0/2. Both passes agreed on 1/1 pairs. Unknown claims are included in the claim denominator.

Not judgeable: countdown (cavalry-mcp: final video missing). Its automatic result is retained.

Not judgeable: map-route (cavalry-mcp: final video missing). Its automatic result is retained.

### Separate once-only repeats

Repeated once after winner disagreement in the original image orders: beat-loop (plugin, baseline).
The repeat used the byte-identical frozen snapshot and the same native setup.
No repeat pair had two final videos, so no repeat judge was called.
A disagreement in a repeat does not trigger another repeat. These results are separate.

#### All tasks

| Arm | Cavalry | Measured runs | Automatic passes | Mean time (s) | Mean tokens | Timeouts |
|---|---|---|---|---|---|---|
| plugin | 2.8.0 | 1 | 0/1 | 1,806.2 | 1,213,699 | 1 |
| baseline | 2.8.0 | 1 | 1/1 | 1,020.4 | 809,625 | 0 |

##### Blind comparison: plugin vs baseline

No judge was called for this group because no contact-sheet pair was available.

| Arm | Cavalry | Wins | Claims met | Unknown claims |
|---|---|---|---|---|
| plugin | 2.8.0 | not measured | not measured | not measured |
| baseline | 2.8.0 | not measured | not measured | not measured |

No completed contact-sheet pairs were judgeable in this group.

Not judgeable: beat-loop (plugin: final video missing). Its automatic result is retained.

These figures replace the historical Cavalry 2.7.2 plugin/baseline figures:
12/14 and 14/14 automatic passes; 10 and 16 blind wins; 118/130 and 111/130 claims;
833 s and 864 s mean time; 1.17 M and 1.01 M mean tokens. Those figures pooled
originals and repeats and used Claude Sonnet. The new originals use 11 runs per arm,
a different judge and Pioneer, so the difference is not an isolated skill effect.
The invalid MCP 1/8 figure, all of `iterpioneer-final`, and iteration 6 tasks started
at or after 15:56 on 2026-09-28 remain excluded.

See [the evaluation method and full tuning tables](docs/evaluation.md).

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
