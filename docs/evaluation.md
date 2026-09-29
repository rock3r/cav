# How we evaluate cavalry-skill

How we measure whether the CLI and the skill help an agent make good motion graphics, and
what we learned about measuring it. For people who run or change the evaluation.

Status: method stable; the final comparison through Pioneer is being rerun after the bridge
isolation fix. Updated 2026-09-30.

## Summary

We give the same motion-design brief to an agent under three conditions (arms), one run at a
time, each in a new Cavalry scene. A script checks each result against hard requirements, and
a separate model compares the arms' contact sheets without knowing which arm made which.

The main lessons so far:

- Measure noise before trusting a difference. With one run per task, a task can flip between
  pass and fail with no code change.
- Check the graders as hard as the agent. Three of our gates contradicted their own briefs,
  and one bug in our bridge contaminated a whole arm.
- Separate plumbing failures (harness, bridge, provider) from model failures. They look alike
  in a pass/fail table.

The harness code and all run data live in `.plans/evals/`, which is not committed.

## Arms

| Arm | The agent gets |
|---|---|
| plugin | the `cav` CLI and the `cavalry` skill |
| baseline | the `cav` CLI only; it can read `cav help` and `cav helpers` |
| cavalry-mcp | the upstream cavalry-mcp server through an MCP adapter; `cav` is hidden |

All arms use the same agent (pi), the same model, the same prompt text apart from one line
naming the tool, and a 30-minute limit. Every arm of one comparison runs from one frozen
snapshot of the CLI and the skill (`.plans/evals/snapshots/iter<N>/`), so edits made while a
batch runs do not leak into it.

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

The music fixtures are synthesised (`harness/make_fixtures.py`), so their tempo and structure
are known exactly, including a short silence before each drop. Each task has five rubric
claims written so that a person can check them from the frames, for example "The quote
appears phrase by phrase (not all at once), then the attribution."

## Automatic checks

`harness/score.py` checks the saved scene and the rendered video. A run passes only if every
gate passes.

| Gate | Passes when |
|---|---|
| sceneSaved, videoRendered | `out/scene.cv` and `out/final.mp4` exist |
| resolution, fps, duration | the video matches the brief (duration within 10 % or 0.5 s) |
| audio | music tasks have an audio stream |
| coverage | at least 60 % of sampled frames are not blank |
| motion | at least 30 % of frames differ from the previous one (on a 160×90 copy), and no still stretch is longer than max(2.5 s, 40 % of the length); some tasks override these |
| eased | at least half of the keyed moves longer than 4 frames have an easing; constant motion (a move of 90 frames or more, or 3 or more linear moves in a row) does not count either way |
| beatSync | at least 40 % of visual hits land on a beat and at least 75 % of downbeats get a hit |

The scorer also records time, tokens, turns, script errors and the `cav` commands used.

## Blind judging

`harness/blind.py prepare` copies each pair of contact sheets to `judge/<iter>/<task>/A.png`
and `B.png` in random order, with the task brief and rubric, and keeps the answer key in a
separate file. A second copy with the order swapped goes to `judge/<iter>-swap/`. A judge
model that is not the tested model (Claude Sonnet) marks each rubric claim true, false or
unknown for both sheets and picks the better one. `blind.py unblind` maps the verdicts back
to arms and reports wins, claims met, and how often the two passes agreed.

The judge sees only still frames. It cannot judge audio or exact beat timing, so those claims
are marked unknown, and the automatic beat gate covers them.

## Running through Pioneer

Pioneer runs each agent in a macOS sandbox. The harness (`run.py --via pioneer`) adapts to
it:

- `cav` works in spool mode. The harness starts `cav relay` outside the sandbox and writes a
  `.cav-spool` file in the run folder.
- The cavalry-mcp arm loads pi-mcp-adapter with `--pi-extension`, may reach only port 8722
  (`--allow-loopback`), and starts the server through `/usr/bin/env HOME=<real home>`, because
  Pioneer gives the agent a private home folder and cavalry-mcp reads its token from the real
  one.
- The agent's stdout and stderr go to files (`--stdout-file`, `--stderr-file`). Otherwise
  pi's JSON events, which carry every viewed image as base64, pass Pioneer's 4 MiB in-memory
  limit.

## What went wrong, and what we changed

| Problem | Effect | Fix |
|---|---|---|
| The quote-card still limit was below the hold the brief asked for | the plugin arm failed for doing what the brief said | per-task `maxStill` |
| The motion gate barely registers a thin line and a small marker | both arms failed map-route | per-task `minMotion` |
| The eased gate counted correct constant motion (orbits, drifts) as linear | a good logo sting failed | constant motion counts neither way |
| Review-sheet labels were drawn over the picture | I graded hidden content as a defect | labels moved below the tiles |
| cav-bridge shared global names with the cavalry-mcp bridge | cavalry-mcp jobs ran through our bridge code; later a stuck job blocked our bridge | bridge 0.4.0 keeps its state private; a test enforces it |
| `cav run` deleted results before a later `cav job wait` | a relayed wait blocked every later command for 20 minutes | results kept for a day; unknown jobs fail fast; the relay runs requests in parallel |
| A timed-out run left a script running in Cavalry | the next run's bridge check gave up | the harness waits up to 10 minutes for the bridge |
| OpenRouter free models returned empty responses | runs ended early | reported as "not measurable", not as model failures |

Every gate change applies to all arms equally, and every rescored result is listed as
rescored.

## Reproducing a run

The harness needs Cavalry open with both bridges running (cav-bridge, and cavalry-mcp-bridge
for the MCP arm), `pi` with a configured model, and Pioneer 0.4.2 or newer for `--via
pioneer`.

```bash
cd .plans/evals/harness
python3 batch.py --iter <name> --arms plugin,baseline,mcp --via pioneer
python3 blind.py prepare --iter <name>
# run the judge on judge/<name> and judge/<name>-swap
python3 blind.py unblind --iter <name>
python3 summary.py --iter <name>
```

Runs are serial: one Cavalry, one scene at a time. A full comparison of three arms on eleven
tasks takes 8 to 10 hours with GLM-5.3 Flash, mostly because the model plans for several
minutes before its first command.
