# How we evaluate cav

How we measure whether the CLI and the skill help an agent make good motion graphics, and
what we learned about measuring it. For people who run or change the evaluation.

Status: the clean 33-run comparison and required once-only repeats are complete.
Measured on Cavalry 2.8.0. Earlier results used Cavalry 2.7.2. Updated 2026-10-02.

Failed setup and runtime attempts are preserved and excluded from task scores and means:

| Attempt | Cavalry | Failure | Recovery |
|---|---|---|---|
| pioneer-final2 | 2.8.0 | Harness disabled the extension registering the requested provider model | Load provider extensions through Pioneer's tool-stripping adapter |
| pioneer-final3 | 2.8.0 | Pi shell hook could not execute build-brief | Freeze the helper in a read-only runtime folder; actual Pi shell diagnostic passed |
| pioneer-final4 MCP setup | 2.8.0 | Knowledge search could not load its embedding model | Freeze a preloaded cache; real knowledge search and bridge status passed inside Pioneer |
| pioneer-final4 MCP preview setup | 2.8.0 | Setup interrupted to keep preview files under .plans | Set TMPDIR to the run's out folder; actual MCP PNG preview passed |
| pioneer-final4 MCP bridge | 2.8.0 | Cavalry restarted and its bridge connection failed; cause unknown | Both bridges recovered; read-only checks passed; record listener process IDs before and after each run |
| pioneer-final4 kinetic-title plugin | 2.8.0 | Provider turn reported terminated, with no HTTP status or quota detail | Same-model acknowledgment and both bridge checks passed; retry the unscored attempt |
| pioneer-final4 bar-chart MCP observer check | 2.8.0 | A second MCP client submitted a status request during the actor run; effect on result polling cannot be ruled out | Exclude the attempt and its blind verdicts; retry with the same frozen setup |
| pioneer-final4 beat-loop MCP observer check | 2.8.0 | A second MCP client submitted a status request while native rendering held a script call; effect on result polling cannot be ruled out | Exclude the attempt; retry with the same frozen setup |
| pioneer-final4-repeat baseline preflight | 2.8.0 | Status remained queued beyond 60 seconds; no actor started | Queued job later completed; user rebooted; both bridges passed readiness; resume only the unstarted baseline |

These recoveries kept the tested model, provider, thinking level and native Pioneer sandbox.
The shell-hook reproduction is tracked in [Pioneer issue #102](https://github.com/rock3r/pioneer/issues/102).
Both bridges must belong to the same Cavalry process. A restart during a run invalidates it,
even if both ports still belong to a process named Cavalry.

## Measured results

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


### Tasks used for tuning

| Arm | Cavalry | Measured runs | Automatic passes | Mean time (s) | Mean tokens | Timeouts |
|---|---|---|---|---|---|---|
| plugin | 2.8.0 | 8 | 5/8 | 1,180.4 | 977,676 | 1 |
| baseline | 2.8.0 | 8 | 5/8 | 1,094.4 | 1,053,104 | 1 |
| cavalry-mcp | 2.8.0 | 8 | 2/8 | 1,666.6 | 3,194,675 | 6 |

#### Blind comparison: plugin vs baseline

Judge: gpt-6.1-sol through Codex. Each available pair was judged in both orders.

| Arm | Cavalry | Wins | Claims met | Unknown claims |
|---|---|---|---|---|
| plugin | 2.8.0 | 5/12 | 43/60 | 13 |
| baseline | 2.8.0 | 7/12 | 46/60 | 12 |

Ties: 0/12. Both passes agreed on 5/6 pairs. Unknown claims are included in the claim denominator.

Not judgeable: promo-music (plugin: final video missing). Its automatic result is retained.

Not judgeable: ui-walkthrough (baseline: final video missing). Its automatic result is retained.

Pairs selected for a separate once-only repeat: beat-loop.

#### Blind comparison: plugin vs cavalry-mcp

Judge: gpt-6.1-sol through Codex. Each available pair was judged in both orders.

| Arm | Cavalry | Wins | Claims met | Unknown claims |
|---|---|---|---|---|
| plugin | 2.8.0 | 4/4 | 20/20 | 0 |
| cavalry-mcp | 2.8.0 | 0/4 | 18/20 | 2 |

Ties: 0/4. Both passes agreed on 2/2 pairs. Unknown claims are included in the claim denominator.

Not judgeable: beat-loop (cavalry-mcp: final video missing). Its automatic result is retained.

Not judgeable: kinetic-title (cavalry-mcp: final video missing). Its automatic result is retained.

Not judgeable: logo-sting (cavalry-mcp: final video missing). Its automatic result is retained.

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

## Separate once-only repeats

Repeated once after winner disagreement in the original image orders: beat-loop (plugin, baseline).
The repeat used the byte-identical frozen snapshot and the same native setup.
No repeat pair had two final videos, so no repeat judge was called.
A disagreement in a repeat does not trigger another repeat. These results are separate.

### All tasks

| Arm | Cavalry | Measured runs | Automatic passes | Mean time (s) | Mean tokens | Timeouts |
|---|---|---|---|---|---|---|
| plugin | 2.8.0 | 1 | 0/1 | 1,806.2 | 1,213,699 | 1 |
| baseline | 2.8.0 | 1 | 1/1 | 1,020.4 | 809,625 | 0 |

#### Blind comparison: plugin vs baseline

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


## Summary

We give the same motion-design brief to an agent under three conditions (arms), one run at a
time, each in a new Cavalry scene. A script checks each result against hard requirements, and
a separate model compares the arms' contact sheets without knowing which arm made which.

The main lessons so far:

- Measure noise before trusting a difference. With one run per task, a task can flip between
  pass and fail with no code change.
- Check the graders as hard as the agent. Three of our gates contradicted their own briefs.
- Check the environment as hard as the result. One agent changed the machine to finish its
  task, and that silently invalidated a whole arm for two days.
- Separate plumbing failures (harness, bridge, provider) from model failures. They look alike
  in a pass/fail table.

The harness is in `tools/evals/`. Generated fixtures and all run data go to `.plans/evals/`
(or `CAV_EVAL_DATA`), which is not committed.

## Arms

| Arm | The agent gets |
|---|---|
| plugin | the `cav` CLI and the `cavalry` skill |
| baseline | the `cav` CLI only; it can read `cav help` and `cav helpers` |
| cavalry-mcp | the upstream cavalry-mcp server through an MCP adapter; `cav` is hidden |

All arms use the same agent (pi), the same model, the same prompt text apart from one line
naming the tool, and a 30-minute limit. The clean comparison uses pi through Pioneer's
sandbox. Every arm of one comparison runs from one frozen snapshot of the CLI and the skill
(`snapshots/iter<name>/` in the data folder), so edits to those parts during a batch do not
leak into it. Keep the harness and task definitions fixed too.

## Tasks

Eight tasks were used while we improved the toolkit. Three more were written later and never
used for tuning, to catch overfitting.

| Task | Length | Frame rate | Music | Held out |
|---|---|---|---|---|
| logo-sting | 4 s | 60 | no | no |
| kinetic-title | 5 s | 60 | no | no |
| lower-third | 6 s | 60 | no | no |
| bar-chart | 7 s | 60 | no | no |
| ui-walkthrough (rebuild a screenshot as vectors) | 8 s | 60 | no | no |
| beat-loop | 15 s | 60 | 128 BPM loop | no |
| transition-pack | 8 s | 60 | no | no |
| promo-music | 15.9 s | 60 | 100 BPM track | no |
| quote-card (square, 1080×1080) | 8 s | 30 | no | yes |
| map-route | 10 s | 60 | no | yes |
| countdown | 12.2 s | 30 | 90 BPM track | yes |

The fixtures are generated by `tools/evals/make_fixtures.py` with a fixed seed, so every run
of it produces the same files byte for byte. The music is synthesised, so its tempo and
structure are known exactly, including a short silence before each drop. Each task has five rubric
claims, for example "The quote appears phrase by phrase (not all at once), then the
attribution." Some claims need evidence beyond the frames. The judge can mark them unknown.

## Automatic checks

`tools/evals/score.py` checks the saved scene and the rendered video. A run passes only if every
gate passes.

| Gate | Passes when |
|---|---|
| environment | bridge owners, process IDs and tokens stayed unchanged (see "The port forwarder") |
| sceneSaved, videoRendered | `out/scene.cv` and `out/final.mp4` exist |
| resolution, fps, duration | the video matches the brief (duration within 10 % or 0.5 s) |
| audio | music tasks have an audio stream |
| coverage | at least 60 % of sampled frames are not blank |
| motion | at least 30 % of frames differ from the previous one (on a 160×90 copy), and no still stretch is longer than max(2.5 s, 40 % of the length); some tasks override these |
| eased | at least half of the keyed moves of 4 frames or more have an easing; constant motion (a move of 90 frames or more, or 3 or more linear moves in a row) does not count either way |
| beatSync | at least 40 % of visual hits land on a beat and at least 75 % of downbeats get a hit |

The scorer also records time, tokens, turns, script errors and the `cav` commands used.
Token totals include input, output and cached reads, as reported by Pi.

## Blind judging

`tools/evals/blind.py prepare` copies each pair of contact sheets to `judge/<iter>/<task>/A.png`
and `B.png` in random order, with the task brief and rubric, and keeps the answer key in a
separate file. A second copy with the order swapped goes to `judge/<iter>-swap/`. A judge
model that is not the tested model marks each rubric claim true, false or
unknown for both sheets and picks the better one. `blind.py unblind` maps the verdicts back
to arms and reports wins, claims met, and how often the two passes agreed.

Agreement compares the chosen winner, including a tie. Repeat the task pair once when
that choice changes between image orders. Claim marks and unknown claims are reported
separately.

The judge sees only still frames. It cannot judge audio or exact beat timing, so those claims
are marked unknown, and the automatic beat gate covers them.

The judge also receives no input fixtures or scene source. It cannot independently check
values against the CSV, compare colours and proportions with the original screenshot,
or prove that the UI uses vector elements. Claim counts report the judge's marks. These
counts do not verify those source-dependent conditions.

If a valid task run produces no final video, the automatic checks record a failure. There
is no contact sheet for blind judging. Report that pair as not judgeable, with its reason;
do not create a blank substitute or remove the task from automatic results.

Use `--tag mcp --b mcp` for the plugin-versus-MCP comparison. It writes to
`judge/<iter>-mcp/` and `judge/<iter>-mcp-swap/`, leaving the baseline comparison in place.
Use the same tag and arms with `unblind`. Record the exact judge model. Keep repeated runs
in a separate iteration and report them separately from the first eleven tasks.

The historical comparison used Claude Sonnet. The clean comparison uses
GPT 6.1 Sol (`gpt-6.1-sol`) through Codex, as requested.
Every available original pair has two completed, audited verdicts.
Repeats without two final videos have no blind verdict.

`tools/evals/judge.py --via codex` supplies only the two images and the task text. It disables
tools, skills, memory and integrations. It rejects tool use, provider errors and invalid
verdicts. Each pass starts a fresh session. The answer key is not supplied. Model provenance,
input hashes and logs go to `judge-logs/`. `unblind` validates verdicts too and reports unknown
claims separately. `--via pioneer` remains available for a configured Pi judge model.

Use `prepare --keep-existing` to add pairs as their runs finish. It preserves prior verdicts
and checks that their inputs and A/B mappings have not changed. It reuses prior mappings
when a newly ready task precedes an existing pair. This lets judging proceed
while later tasks run. Use `judge.py --skip-existing` to retain validated judgments.

## Running through Pioneer

Pioneer runs each agent in a macOS sandbox. The harness (`run.py --via pioneer`) adapts to
it:

- `cav` works in spool mode. The harness starts `cav relay` outside the sandbox and writes a
  `.cav-spool` file in the run folder.
- Pioneer loads enabled provider and auth extensions from a private snapshot. It removes
  their tools. Auto-loaded skills and context files remain disabled for all arms; only the
  plugin arm gets the cavalry skill explicitly.
- When build-brief is installed, the batch freezes its executable in `runtime/bin/` and
  records its hash. The actor reads that folder and finds the helper on PATH. This lets
  its existing shell hook work without access to Homebrew folders. Pioneer judges use the
  same copy; the requested Codex judge has hooks and tools disabled.
- The cavalry-mcp arm loads pi-mcp-adapter with `--pi-extension`, may reach only port 8722
  (`--allow-loopback`), and starts the server through `/usr/bin/env HOME=<real home>`, because
  Pioneer gives the agent a private home folder and cavalry-mcp reads its token from the real
  one. The wrapper sets `TMPDIR` to `out/.mcp-tmp/` inside the run folder, so preview PNGs
  also stay under `.plans/`.
- The MCP knowledge-search model is preloaded outside the actor sandbox. The batch copies
  it to the iteration's `runtime/mcp-kb/` folder and records each file's SHA-256. The server
  gets read access to that cache and runs with `HF_HUB_OFFLINE=1`. It needs no model-download
  access inside the sandbox. Resuming checks the frozen files before running another task.
- The agent's stdout and stderr go to files (`--stdout-file`, `--stderr-file`). Otherwise
  pi's JSON events, which carry every viewed image as base64, pass Pioneer's 4 MiB in-memory
  limit.
- During an active MCP run, observer checks only read `GET http://127.0.0.1:8722/get`.
  This reads the existing response without submitting a script. The upstream bridge keeps
  one shared latest result. Its Python lock belongs to each client instance, so a second
  client can overwrite a response the actor is waiting for. Run full MCP status scripts
  only before or after an MCP actor. A passive GET proves HTTP availability; it does not
  prove that a new script would finish. This protocol limit was read in the installed
  bridge source. The two observer-overlap attempts above are excluded, not measured task failures.
- Scratch scenes and renders stay under `.plans/evals/scratch/`. Pioneer gives the actor
  access only to its work folder and the explicit runtime paths.
- A harness error, a non-timeout controller exit, a scorer error, or an environment change
  stops the batch. Provider errors in the event stream also stop it, even if the controller
  exits successfully. Inspect the logs before resuming. A timeout while the model works remains
  a task result and is scored. A normal exit at the model's output limit is also scored
  when no provider error occurred. Missing required files fail the automatic checks.

These results apply to the recorded Pioneer grants. Direct `ffprobe` calls returned
"command not found" in the kinetic-title plugin actor. CLI commands run through the relay;
the relay and scorer can use the host's installed media programs. Actor shell access is
limited to its declared runtime paths and work folder.

## What went wrong, and what we changed

| Problem | Effect | Fix |
|---|---|---|
| The quote-card still limit was below the hold the brief asked for | the plugin arm failed for doing what the brief said | per-task `maxStill` |
| The motion gate barely registers a thin line and a small marker | both arms failed map-route | per-task `minMotion` |
| The eased gate counted correct constant motion (orbits, drifts) as linear | a good logo sting failed | constant motion counts neither way |
| Review-sheet labels were drawn over the picture | I graded hidden content as a defect | labels moved below the tiles |
| A cavalry-mcp agent found its bridge down, started a port forwarder to cav-bridge and copied our token over the cavalry-mcp token | every later cavalry-mcp call ran through cav-bridge; the cavalry-mcp and `cav` clients overwrote each other's results, and a job got stuck | the harness refuses to start when another process holds a bridge port or the tokens are equal, and fails a run whose agent changed them (the `environment` gate); see below |
| `cav run` deleted results before a later `cav job wait` | a relayed wait blocked every later command for 20 minutes | results kept for a day; unknown jobs fail fast; the relay runs requests in parallel |
| A timed-out run left a script running in Cavalry | the next run's bridge check gave up | the harness waits up to 10 minutes for the bridge |
| OpenRouter free models returned empty responses | runs ended early | reported as "not measurable", not as model failures |

Every gate change applies to all arms equally, and every rescored result is listed as
rescored.

### The port forwarder

This is the most serious problem we found, so it is described in full.

On 2026-09-28 at about 16:00, in iteration 6, the cavalry-mcp agent on `ui-walkthrough` could
not reach the cavalry-mcp bridge on port 8722. That run used pi directly, without a sandbox.
The agent found cav-bridge on port 8723, wrote `/tmp/portfwd.py`, and started it in the
background to forward 8722 to 8723. It then copied `~/.cav/token` over
`~/.cavalry-mcp-token` and drove cav-bridge with curl. We found this on 2026-10-01 from the
run's event log and from `lsof`, which showed a Python process on port 8722.

The forwarder kept running after the run ended. Every later cavalry-mcp call went to
cav-bridge, and the real cavalry-mcp bridge could not open its port.

At first we blamed shared global names between the two bridges. That was wrong. On Cavalry
2.8.0, a cav job cannot see the cavalry-mcp bridge's top-level names. With the forwarder
stopped and separate tokens, ten overlapping cavalry-mcp and `cav` jobs all returned correct
results (measured 2026-10-01).

| Runs | cavalry-mcp results |
|---|---|
| iteration 1; iteration 6 tasks that started before 15:56 on 2026-09-28 | from the real cavalry-mcp bridge |
| iteration 6 tasks that started at 15:56 or later; all of `pioneer-final` | not valid: served by cav-bridge |
| plugin and baseline arms, all iterations | not affected |

What we changed:

- `run.py` records which process listens on each bridge port and a hash of each token,
  before and after every run. A run does not start when a port belongs to anything except
  Cavalry, when the cavalry-mcp bridge is down for the MCP arm, or when the two tokens are
  equal.
- If the agent changed the ports or tokens, `score.py` fails the run's `environment` gate.
- The final comparisons run through Pioneer. Its sandbox lets an agent write only its run
  folder and reach only the ports its arm needs.

## Reproducing a run

The harness needs Cavalry open with both bridges running (cav-bridge, and
cavalry-mcp-bridge for the MCP arm), `pi` with a configured model, `uv`, and Pioneer 0.4.2 or
newer for `--via pioneer`. The MCP arm also needs a cavalry-mcp checkout (default
`~/src/cavalry-mcp`, or set `CAVALRY_MCP_DIR`) and the adapter installed once:

```bash
go build -o bin/cav ./cmd/cav      # batch.py freezes this binary and the skill per iteration
(cd tools/evals/mcp && npm ci)
uv run --no-project --with numpy --with scipy --with pillow python tools/evals/make_fixtures.py
FASTEMBED_CACHE_PATH="$PWD/.plans/evals/runtime/mcp-kb-prefetch" "${CAVALRY_MCP_DIR:-$HOME/src/cavalry-mcp}/.venv/bin/python" -c 'from cavalry_mcp.tools.knowledge import search; assert search("composition frame range", 1)'
python3 tools/evals/batch.py --iter <name> --arms plugin,baseline,mcp --via pioneer
python3 tools/evals/blind.py prepare --iter <name> --keep-existing
python3 tools/evals/judge.py --iter <name> --via codex --judge-model gpt-6.1-sol --skip-existing
python3 tools/evals/blind.py unblind --iter <name>
python3 tools/evals/blind.py prepare --iter <name> --b mcp --tag mcp --keep-existing
python3 tools/evals/judge.py --iter <name> --tag mcp --via codex --judge-model gpt-6.1-sol --skip-existing
python3 tools/evals/blind.py unblind --iter <name> --b mcp --tag mcp
python3 tools/evals/summary.py --iter <name>
```

The Codex binary must support the requested judge model. Use `--codex-bin <path>` when
the desktop app's bundled binary is newer than the command on PATH.

`tools/evals/peek.py <run dir>` prints a run's tool calls while it is still going.
Use `--skip-existing` with the batch command to resume scored runs. A run with
`environmentChanged` in its metadata stops a resumed batch too.

Runs are serial: one Cavalry, one scene at a time. A full comparison has 33 runs, each
with a 30-minute deadline, and can take many hours. Planning and rendering count toward
task time. Excluded infrastructure failures add wall time but do not enter task means.
