# How we evaluate cav

This document describes how the project tests agents making motion graphics with the
CLI, skill and cavalry-mcp. It records the results and limits for evaluation maintainers.

Status: The 33 original runs and the required single repeats are complete. The clean
comparison used Cavalry 2.8.0; earlier runs used 2.7.2. Updated 2026-10-02.

The project kept failed setup and runtime attempts. Task scores and means exclude them:

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

Recovery kept the model, provider, thinking level and native Pioneer sandbox fixed.
The shell-hook reproduction is tracked in [Pioneer issue #102](https://github.com/rock3r/pioneer/issues/102).
Both bridges must belong to one Cavalry process. A restart invalidates an active run,
even if the new process has the same name and ports.

## Measured results

The comparison used Cavalry **2.8.0**, `zai/glm-5.3-flash`, medium thinking and
Pioneer's native sandbox. Each run started in a new scene with a 30-minute limit.
The CLI and skill were frozen at `cadd3c6`. Pioneer was 0.4.4; Pi was 0.84.2.

Each arm has 11 original tasks. Eight informed toolkit changes; quote-card, map-route
and countdown were held out. The plugin arm used the CLI and skill. Baseline used the
CLI alone. The cavalry-mcp arm used the upstream server with the CLI hidden. The team
excluded infrastructure and provider failures, then reran those attempts with the
same setup. Normal deadlines remain in the data. Missing deliverables fail checks.

GPT 6.1 Sol (`gpt-6.1-sol`) through Codex CLI 0.159.2 judged each available pair twice.
Each judgment used a fresh session, reversed image order and no tools or answer key.
Wins count judgments. Claim totals reflect the judge's marks and include unknown claims.
Contact sheets cannot establish audio, exact beat timing, source fidelity or vector
construction. Automatic checks cover audio and beats separately. Missing final videos
prevent blind judgments. Automatic checks still fail for those tasks.

All figures below are **measured**. Token means include input, output and cached reads.
Time means include normal task deadlines. Original and repeat runs stay separate.

### All tasks

| Arm | Cavalry | Measured runs | Automatic passes | Mean time (s) | Mean tokens | Timeouts |
|---|---|---|---|---|---|---|
| plugin | 2.8.0 | 11 | 8/11 | 1,176.7 | 1,219,232 | 1 |
| baseline | 2.8.0 | 11 | 8/11 | 1,025.3 | 982,121 | 1 |
| cavalry-mcp | 2.8.0 | 11 | 3/11 | 1,604.0 | 3,221,162 | 8 |

#### Blind comparison: plugin vs baseline

GPT 6.1 Sol judged each available pair in both image orders through Codex.

| Arm | Cavalry | Wins | Claims met | Unknown claims |
|---|---|---|---|---|
| plugin | 2.8.0 | 9/18 | 63/90 | 22 |
| baseline | 2.8.0 | 9/18 | 66/90 | 22 |

The judge recorded 0/18 ties and agreed on 8/9 pairs across image orders. Unknown
claims remain in the denominator. Promo-music lacked the plugin video, and ui-walkthrough
lacked the baseline video. Both retain their automatic results without blind verdicts.
The team selected beat-loop for one separate repeat.

#### Blind comparison: plugin vs cavalry-mcp

GPT 6.1 Sol judged each available pair in both image orders through Codex.

| Arm | Cavalry | Wins | Claims met | Unknown claims |
|---|---|---|---|---|
| plugin | 2.8.0 | 6/6 | 30/30 | 0 |
| cavalry-mcp | 2.8.0 | 0/6 | 28/30 | 2 |

The judge recorded 0/6 ties and agreed on 3/3 pairs across image orders. Unknown
claims remain in the denominator. These tasks lacked a final video and had no blind
verdict. Their automatic results remain counted:

| Task | Missing final video |
|---|---|
| beat-loop | cavalry-mcp |
| countdown | cavalry-mcp |
| kinetic-title | cavalry-mcp |
| logo-sting | cavalry-mcp |
| map-route | cavalry-mcp |
| promo-music | plugin and cavalry-mcp |
| transition-pack | cavalry-mcp |
| ui-walkthrough | cavalry-mcp |


### Tasks used for tuning

| Arm | Cavalry | Measured runs | Automatic passes | Mean time (s) | Mean tokens | Timeouts |
|---|---|---|---|---|---|---|
| plugin | 2.8.0 | 8 | 5/8 | 1,180.4 | 977,676 | 1 |
| baseline | 2.8.0 | 8 | 5/8 | 1,094.4 | 1,053,104 | 1 |
| cavalry-mcp | 2.8.0 | 8 | 2/8 | 1,666.6 | 3,194,675 | 6 |

#### Blind comparison: plugin vs baseline

GPT 6.1 Sol judged each available pair in both image orders through Codex.

| Arm | Cavalry | Wins | Claims met | Unknown claims |
|---|---|---|---|---|
| plugin | 2.8.0 | 5/12 | 43/60 | 13 |
| baseline | 2.8.0 | 7/12 | 46/60 | 12 |

The judge recorded 0/12 ties and agreed on 5/6 pairs across image orders. Unknown
claims remain in the denominator. Promo-music lacked the plugin video, and ui-walkthrough
lacked the baseline video. Their automatic results remain counted without blind verdicts.
The team selected beat-loop for one separate repeat.

#### Blind comparison: plugin vs cavalry-mcp

GPT 6.1 Sol judged each available pair in both image orders through Codex.

| Arm | Cavalry | Wins | Claims met | Unknown claims |
|---|---|---|---|---|
| plugin | 2.8.0 | 4/4 | 20/20 | 0 |
| cavalry-mcp | 2.8.0 | 0/4 | 18/20 | 2 |

The judge recorded 0/4 ties and agreed on 2/2 pairs across image orders. Unknown
claims remain in the denominator. These tasks had no blind verdict because a final
video was missing. Their automatic results remain counted:

| Task | Missing final video |
|---|---|
| beat-loop | cavalry-mcp |
| kinetic-title | cavalry-mcp |
| logo-sting | cavalry-mcp |
| promo-music | plugin and cavalry-mcp |
| transition-pack | cavalry-mcp |
| ui-walkthrough | cavalry-mcp |


### Held-out tasks

| Arm | Cavalry | Measured runs | Automatic passes | Mean time (s) | Mean tokens | Timeouts |
|---|---|---|---|---|---|---|
| plugin | 2.8.0 | 3 | 3/3 | 1,166.7 | 1,863,384 | 0 |
| baseline | 2.8.0 | 3 | 3/3 | 841.2 | 792,833 | 0 |
| cavalry-mcp | 2.8.0 | 3 | 1/3 | 1,437.0 | 3,291,794 | 2 |

#### Blind comparison: plugin vs baseline

GPT 6.1 Sol judged each available pair in both image orders through Codex.

| Arm | Cavalry | Wins | Claims met | Unknown claims |
|---|---|---|---|---|
| plugin | 2.8.0 | 4/6 | 20/30 | 9 |
| baseline | 2.8.0 | 2/6 | 20/30 | 10 |

The judge recorded 0/6 ties and agreed on 3/3 pairs across image orders. Unknown
claims remain in the denominator.

#### Blind comparison: plugin vs cavalry-mcp

GPT 6.1 Sol judged each available pair in both image orders through Codex.

| Arm | Cavalry | Wins | Claims met | Unknown claims |
|---|---|---|---|---|
| plugin | 2.8.0 | 2/2 | 10/10 | 0 |
| cavalry-mcp | 2.8.0 | 0/2 | 10/10 | 0 |

The judge recorded 0/2 ties and agreed on the one pair across image orders. Unknown
claims remain in the denominator. Countdown and map-route lacked cavalry-mcp final
videos. Their automatic results remain counted without blind verdicts.

## Separate once-only repeats

The team repeated beat-loop once for plugin and baseline after the original image orders
disagreed on the winner. The repeat used the same native setup and a byte-identical
snapshot. No repeat pair had two final videos, so the judge received no repeat pair.
The process allows one repeat only. These results stay separate from original runs.

### All tasks

| Arm | Cavalry | Measured runs | Automatic passes | Mean time (s) | Mean tokens | Timeouts |
|---|---|---|---|---|---|---|
| plugin | 2.8.0 | 1 | 0/1 | 1,806.2 | 1,213,699 | 1 |
| baseline | 2.8.0 | 1 | 1/1 | 1,020.4 | 809,625 | 0 |

#### Blind comparison: plugin vs baseline

No pair of contact sheets was available for this group, so no judge ran.

| Arm | Cavalry | Wins | Claims met | Unknown claims |
|---|---|---|---|---|
| plugin | 2.8.0 | not measured | not measured | not measured |
| baseline | 2.8.0 | not measured | not measured | not measured |

Beat-loop lacked the plugin final video. Its automatic result remains counted without
a blind verdict.

These figures replace earlier Cavalry 2.7.2 plugin and baseline figures.
The earlier counts were 12/14 and 14/14 automatic passes, 10 and 16 blind wins,
and 118/130 and 111/130 claims. Mean times were 833 s and 864 s;
mean token counts were 1.17 M and 1.01 M. Those figures combined
original and repeat runs and used Claude Sonnet. The newer originals have 11 tasks
per arm, another judge and Pioneer. The difference cannot be assigned to the skill alone.
The invalid MCP 1/8 figure, all of `iterpioneer-final`, and iteration 6 tasks started
at or after 15:56 on 2026-09-28 remain excluded.


## Summary

Each arm receives the same motion-design brief in a fresh Cavalry scene. Runs proceed
one at a time. A script checks required outputs and behavior. A separate model compares
contact sheets without knowing which arm produced each one.

The earlier iterations exposed these measurement problems:

- A single task can change from pass to fail with no code change. One run per task
  therefore does not establish a stable difference.
- Three automatic gates contradicted their task briefs. The team corrected and reran
  those checks.
- One agent changed the machine during a run. That invalidated later results in one arm
  until the change was found two days later.
- Harness, bridge and provider failures can resemble task failures in pass/fail tables.
  The analysis records them separately.

The harness is in `tools/evals/`. It writes fixtures and run data to uncommitted
`.plans/evals/`, or to `CAV_EVAL_DATA` when set.

## Arms

| Arm | The agent gets |
|---|---|
| plugin | the `cav` CLI and the `cavalry` skill |
| baseline | the `cav` CLI only; it can read `cav help` and `cav helpers` |
| cavalry-mcp | the upstream cavalry-mcp server through an MCP adapter; `cav` is hidden |

All arms use Pi, the same model and a 30-minute limit. Prompt text differs only in
one line naming the tool. The clean comparison runs Pi inside Pioneer's sandbox.
One frozen CLI and skill snapshot serves every arm of a comparison at
`snapshots/iter<name>/` in the data folder. Keep the harness and task definitions
fixed during the batch.

## Tasks

Eight tasks informed toolkit changes. Three later tasks stayed outside tuning to
check whether those changes generalized.

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

`tools/evals/make_fixtures.py` uses a fixed seed, producing identical fixture bytes.
Synthesized music has known tempo and structure, including a brief silence before each
drop. Each task has five rubric claims. One asks whether a quote appears phrase by
phrase before its attribution. The judge marks claims unknown when frames lack evidence.

## Automatic checks

`tools/evals/score.py` checks the saved scene and video. Every gate must pass for the
run to pass.

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

The scorer records time, tokens, turns, script errors and cav commands. Pi's token
counts include input, output and cached reads.

## Blind judging

`tools/evals/blind.py prepare` copies contact sheets to
`judge/<iter>/<task>/A.png` and `B.png` in random order. It includes the task brief
and rubric. It keeps the answer key separately. A swapped pair goes to
`judge/<iter>-swap/`. A model separate from the tested agent marks each claim true,
false or unknown for both sheets and chooses a winner. `blind.py unblind` maps results
to arms and reports wins, claims met and agreement between orders.

Agreement compares winners and ties across image orders. If the choice changes,
repeat that task pair once. Report claim marks and unknown claims separately.

The judge sees still frames only. It marks audio and exact beat-timing claims unknown.
Automatic gates check those properties separately.

The judge receives no input fixtures or scene source. It cannot check CSV values,
compare the result with an original screenshot or prove vector construction. Claim
counts report its marks without establishing these source-dependent properties.

If a valid run produces no final video, automatic checks fail it. The pair has no
contact sheet for blind judgment. Report the missing video as the reason. Keep the
task in automatic results and do not create a blank substitute.

Use `--tag mcp --b mcp` for the plugin-versus-MCP comparison. It writes to
`judge/<iter>-mcp/` and `judge/<iter>-mcp-swap/`, leaving the baseline comparison in place.
Use the same tag and arms with `unblind`. Record the exact judge model. Keep repeated runs
in a separate iteration and report them separately from the first eleven tasks.

The earlier comparison used Claude Sonnet. The clean comparison uses GPT 6.1 Sol
(`gpt-6.1-sol`) through Codex. Every available original pair has two completed,
audited verdicts. A repeat without two final videos has no blind verdict.

`tools/evals/judge.py --via codex` sends two images and task text. It disables tools,
skills, memory and integrations. It rejects tool use, provider errors and invalid
verdicts. Each pass starts a new session without the answer key. `judge-logs/` stores
model provenance, input hashes and logs. `unblind` validates verdicts and reports
unknown claims. `--via pioneer` remains available for a configured Pi judge model.

`prepare --keep-existing` adds finished pairs while preserving earlier verdicts. It
checks their input files and A/B mapping, and reuses mappings when another task becomes
ready earlier in the order. Judgment can proceed while later tasks run.
`judge.py --skip-existing` keeps validated judgments.

## Running through Pioneer

Pioneer runs agents in a macOS sandbox. `run.py --via pioneer` configures these paths
and controls:

- `cav` uses spool mode. The harness starts `cav relay` outside the sandbox and writes a
  `.cav-spool` file in the run folder.
- Pioneer loads enabled provider and authentication extensions from a private snapshot. It removes
  their tools. Auto-loaded skills and context files remain disabled for all arms; only the
  plugin arm gets the cavalry skill explicitly.
- When build-brief is installed, the batch copies its executable into frozen `runtime/bin/` and
  records its hash. The actor reads that folder and finds the helper on PATH. This lets
  its existing shell hook work without access to Homebrew folders. Pioneer judges use the
  same copy; the requested Codex judge has hooks and tools disabled.
- The cavalry-mcp arm loads pi-mcp-adapter with `--pi-extension`. It can reach only port 8722
  (`--allow-loopback`). It starts the server through `/usr/bin/env HOME=<real home>`.
  Pioneer gives the agent a private home folder. cavalry-mcp reads its token from the
  real home folder. The wrapper sets `TMPDIR` to `out/.mcp-tmp/` inside the run folder,
  so preview PNGs also stay under `.plans/`.
- The MCP knowledge-search model is preloaded outside the actor sandbox. The batch copies
  it to the iteration's `runtime/mcp-kb/` folder and records each file's SHA-256. The server
  gets read access to that cache and runs with `HF_HUB_OFFLINE=1`. It needs no model-download
  access inside the sandbox. Resuming checks the frozen files before running another task.
- The harness writes agent stdout and stderr to files (`--stdout-file`, `--stderr-file`). Otherwise
  pi's JSON events, which carry every viewed image as base64, pass Pioneer's 4 MiB in-memory
  limit.
- During an active MCP run, observer checks read only `GET http://127.0.0.1:8722/get`.
  This reads the existing response without submitting a script. The upstream bridge keeps
  one shared latest result. Its Python lock belongs to each client instance, so a second
  client can overwrite a response the actor is waiting for. Run full MCP status scripts
  only before or after an MCP actor. A passive GET proves HTTP availability; it does not
  prove that a new script would finish. This protocol limit was read in the installed
  bridge source. The two observer-overlap attempts above are excluded from task-failure
  measurements.
- Scratch scenes and renders stay in `.plans/evals/scratch/`. Pioneer gives the actor
  access only to its work folder and the explicit runtime paths.
- A harness error, a non-timeout controller exit, a scorer error, or an environment change
  stops the batch. Provider errors in the event stream also stop it, even if the controller
  exits successfully. Inspect the logs before resuming. A timeout while the model works remains
  a task result and is scored. A normal exit at the model's output limit is also scored
  when no provider error occurred. Missing required files fail the automatic checks.

These results apply to the recorded Pioneer access grants. Direct `ffprobe` calls returned
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
| A cavalry-mcp agent found its bridge down, started a port forwarder to cav-bridge and copied the cav-bridge token over the cavalry-mcp token | every later cavalry-mcp call ran through cav-bridge; the cavalry-mcp and `cav` clients overwrote each other's results, and a job got stuck | the harness refuses to start when another process holds a bridge port or the tokens are equal, and fails a run whose agent changed them (the `environment` gate); see below |
| `cav run` deleted results before a later `cav job wait` | a relayed wait blocked every later command for 20 minutes | results kept for a day; unknown jobs fail fast; the relay runs requests in parallel |
| A timed-out run left a script running in Cavalry | the next run's bridge check gave up | the harness waits up to 10 minutes for the bridge |
| OpenRouter free models returned empty responses | runs ended early | reported as "not measurable"; excluded from model failures |

The team applied each gate change to all arms and labelled every rescored result.

### The port forwarder

The port forwarder invalidated later cavalry-mcp runs and needs a full account.

On 2026-09-28 around 16:00, the iteration 6 cavalry-mcp agent could not reach its
bridge on port 8722 during `ui-walkthrough`. This Pi run had no sandbox. The agent
found cav-bridge on port 8723, wrote `/tmp/portfwd.py` and forwarded 8722 to 8723.
It copied `~/.cav/token` to `~/.cavalry-mcp-token` and called cav-bridge with curl.
On 2026-10-01, the event log and `lsof` showed the Python process on port 8722.

The forwarder remained active after the run. Later cavalry-mcp calls reached cav-bridge,
and the cavalry-mcp bridge could not open port 8722.

The initial investigation suspected shared global names between bridges. Cavalry 2.8.0
did not show that behavior: a cav job could not read cavalry-mcp top-level names. After
stopping the forwarder and separating tokens, ten overlapping jobs from both bridges
returned correct results on 2026-10-01.

| Runs | cavalry-mcp results |
|---|---|
| iteration 1; iteration 6 tasks that started before 15:56 on 2026-09-28 | from the real cavalry-mcp bridge |
| iteration 6 tasks that started at 15:56 or later; all of `pioneer-final` | not valid: served by cav-bridge |
| plugin and baseline arms, all iterations | not affected |

The harness now applies these checks:

- `run.py` records which process listens on each bridge port and a hash of each token,
  before and after every run. A run does not start when a port belongs to anything except
  Cavalry, when the cavalry-mcp bridge is down for the MCP arm, or when the two tokens are
  equal.
- If the agent changed the ports or tokens, `score.py` fails the run's `environment` gate.
- The final comparisons run through Pioneer. Its sandbox lets an agent write only its run
  folder and reach only the ports its arm needs.

## Reproducing a run

Open Cavalry with cav-bridge running. For the MCP arm, also start cavalry-mcp-bridge.
The harness needs `pi` with a model, `uv` and Pioneer 0.4.2 or newer for
`--via pioneer`. The MCP arm also needs a cavalry-mcp checkout at
`~/src/cavalry-mcp` or `CAVALRY_MCP_DIR`. Install its adapter once:

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

The Codex binary must support the requested judge model. If the desktop app has a
newer binary than PATH, select it with `--codex-bin <path>`.

`tools/evals/peek.py <run dir>` shows tool calls during a run. Use `--skip-existing`
to resume a batch without rerunning scored tasks. A recorded `environmentChanged`
value stops resumed batches.

Runs use one Cavalry scene at a time. A full comparison has 33 runs, each limited to
30 minutes, and can take many hours. Planning and rendering count toward task time.
Excluded infrastructure failures add wall time. Task means exclude them.
