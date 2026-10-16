# Operation recovery and performance diagnostics

`cav scene open`, `cav frame`, `cav sheet`, `cav render`, `cav check` and `cav run`
record operations separately from bridge jobs. Other commands keep raw-job behavior.
An operation ID tracks its command, working directory, transport, phase, jobs, output,
partial results and checkpoints. cav stores private records (0600) in
`~/.cav/operations/` until the operator removes them. They can contain scene data.

## Recover a timeout

```sh
cav render -o renders/final.mp4 --timeout 90m --json
cav operation status <operation-id> --json
cav operation resume <operation-id> --timeout 90m --json
```

One timeout covers preparation, bridge waits, rendering, muxing and validation. If the
deadline interrupts ffmpeg or ffprobe, cav returns exit 3 and keeps staged output.
It does not classify the interrupted subprocess as a mux or validation failure.
Script input preparation also uses this budget, including stdin reads and named pipes.
If input times out before job preparation, cav submitted no native work. Resume cannot
reconstruct a partial stream. Keep the complete source and supply it to a new run.
cav keeps the incomplete local record for inspection.
Audio hashing uses a streaming helper under the same deadline. A FIFO without a writer
or a stalled stream cannot hold the operation lock indefinitely. These preparation
guarantees cover direct operation dispatch. The spool client still reads stdin before
dispatch and has no timeout for that read.
A resume gets a new budget. It reads completed checkpoints, waits for the pending job
and continues remaining phases. Waiting for a metadata job with
`cav job wait <job-id>` completes only that job; it does not start rendering. Raw job-wait
remains available, including for older CLI jobs.

Exit 3 does not justify submitting the original render or build again. cav saves the job
ID and submission intent **before** POST. If the acknowledgement is lost, delivery is
`uncertain`. Resume searches for that job's result, even with an older bridge, and never
posts the job again. A `prepared` job with no POST attempt can be submitted. New bridges
can also return stored results for completed duplicate IDs.
The original operation restriction mode is persisted before preparation; resuming under
a permissive relay cannot remove the original process-launching API restrictions. A
stricter current mode also applies. Older records without a recorded mode can recover
existing results and refuse new submissions, because their original mode is unknown.

On Unix, checkpoint publication syncs file data and the parent directory after rename.
Windows uses [MoveFileExW](https://learn.microsoft.com/windows/win32/api/winbase/nf-winbase-movefileexw)
with replacement and write-through. New checkpoint directories use the same namespace
barrier. A failed barrier prevents POST. These guarantees depend on filesystem
persistence primitives. Fixtures do not simulate a machine power cut.

`cav status`, `cav job status <id>` and `cav operation status <id>` only read existing
state. They do not queue metadata or rendering. `cav scene info` requests scene metadata.
Status can be `queued`, `running`, `busy`, `unknown` or `refused`. A completed preparation
job leaves an operation `ready-to-resume`. Inspection does not change checkpoint times.
Silence cannot distinguish a blocked native call from a stopped app. A heartbeat or
cancellation cannot interrupt a native call inside Cavalry.

A refused HTTP bridge connection ends a job wait with exit 4 and
`failureReason: bridge-disconnected`. A new session on an updated bridge returns
`bridge-session-changed`, including when the original job's script file remains on disk.
The native outcome stays `unknown`. A lost bridge alone does not prove Cavalry crashed
or lost output. A stored result for the original job takes precedence. Older bridges
without session IDs and remote spool transports lack the same restart evidence. Inspect
their unresolved outcomes.
Timeouts from a blocked native call remain `busy` or `unknown`, with no automatic retry.

A kernel lock serializes operation clients that share `CAV_HOME`. A pending native job
continues to block new operations on the same transport after its CLI exits. Resume
checks the same gate before new submissions, excluding its own operation. Clients can
still read completed results or wait on recorded jobs.
When the gate refuses a command, cav records `refused` with
`failureReason: blocked-by-operation`. cav submits no native work for that command.
It cannot block later work and needs no `abandon`. The error and its JSON
(`blockedBy`, `blockingJob`, `blockingStatus`) name the blocking operation. Check that
operation with `cav operation status <id>`. If its outcome stays unknown, check Cavalry
and its outputs, then clear it with `cav operation abandon <id> --acknowledge-unknown-outcome`.
After that, `cav operation resume` continues the refused command.
The relay runs forwarded operations in its own state folder; inspect/resume through the same relay.
Raw commands, independent `CAV_HOME` folders, other tools and GUI edits are outside this
coordination. Avoid them until the operation completes.

Keep the input scene and audio unchanged during recovery. Checkpoints bind scene-open
input and audio hashes, the default output directory and helper-library version. New
bridges expose a session ID. cav refuses remaining submissions into another bridge
window. With older bridges, the operator must confirm the original scene and session.
Frame exports, renders and profiles also check active comp and scene path. These checks
cannot detect every edit within the same scene. Use the same CLI build during resume;
cav rejects changed generated job code. Once script preparation finishes, a recovered
`run` uses the recorded script. It does not reread files or stdin. Async runs become
synchronous waits during recovery. A run without a captured script cannot resume.

## Scene opening and image exports

```sh
cav scene open scenes/large.cv --timeout 15m --json
cav frame 840 -o renders/f840.png --timeout 10m --json
cav sheet 0-59:1 --timeout 10m --json
```

Scene opening has a ten-minute default budget. It covers input hashing, the unsaved-scene
guard, the open call and later scene metadata. `api.openScene` can return before the
native scene is ready. If a phase exceeds the budget, resume the recorded operation.
cav does not submit the open call twice. Resume refuses changed input and checks the
opened scene path in its later query.

Frame and sheet commands default to five minutes and at most 120 frames. Their native
script binds the original scene and comp. A `finally` block restores the playhead even
after a failed PNG call. Native PNGs go to an operation-specific directory beside the
output. Resume waits for the recorded job, then publishes frames or assembles the sheet.
Each output image replaces an existing file through a sibling temporary file and rename.
A multi-frame export does not publish the full set atomically. Staging remains available
to repeat interrupted publication without another render.

cav saves the publication checkpoint before removing sheet frames. `--keep` retains
those frames, and cav always retains the staged sheet. Final publication checks the
budget after assembly, so assembly can exceed it. A native crash can prevent playhead
restoration. Keep the scene and CLI build unchanged for recovery.

## Stale or failed operations

An unknown result can come from an expired one-day bridge cache or a restarted bridge.
cav does not infer success, remove recovery records or submit the job again. Inspect
the native output and original scene. Fix local failures such as a missing ffprobe,
then resume. A failed script returns its cached result without repeating scene changes.

After manually reconciling an unknown outcome, release its local gate explicitly:

```sh
cav operation abandon <id> --acknowledge-unknown-outcome
```

This keeps the record and marks local recovery abandoned. It **does not cancel** native
work. Do not resume abandoned operations. Completed records need no abandonment.

cav keeps render output in `.cav-<operation-id>` beside the target. Native video, partial
mux and completed mux remain separate. A completed mux survives a lost checkpoint.
ffprobe must confirm the requested frame count before publication. Hard-link publication
keeps existing output and can recognize a published file after a crash. The destination
filesystem must support hard links. Keep staging and prior installed app bundles until
completion is confirmed. Render queue items have operation-specific names and remain
in the scene until removed manually.

## Render stopped partway through

Use `cav render` and its operation ID to follow a render. `operation status` reports
staged file sizes, modification times, ages and expected frame count. It does not query
the scene or run ffprobe. These files remain `unvalidated`. A large file does not prove
completion. An unchanged file can indicate a slow frame, buffering, a closed bridge or
an exited app. File activity alone cannot change operation state or justify a restart.

If the bridge disconnects or restarts, stop automatic waiting. Inspect the saved scene,
original result and staged output. Keep partial files. Resume only waits for the recorded
native job; it never resubmits it after an app restart. If the original call cannot
finish, reconcile its outcome. Explicitly abandon it before rendering to a new path.
cav does not relaunch the app, start shorter chunks, delete partial files or infer an
out-of-memory failure from a missing crash report. A successful native result still
needs exact frame-count validation. cav rejects and retains a short or unreadable video.

## Choose an inspection cost

```sh
cav check --quick --json
cav check --timeout 2m --json
cav check --profile --samples 3 --timeout 2m --json
cav check --profile-render --samples 3 --timeout 2m --json
```

The quick pass reads comp and layer metadata and connections. It does not move the
playhead, calculate bounds or render. It visits referenced pre-comps without switching
the active comp. It reads layer membership once per scene and shared references once.
Default limits are 1,000 reachable layers, 10,000 connections, 64 compositions and
10,000 membership queries (`--max-scan`). cav checks time between calls. Truncated
scans, unresolved or cyclic references and accessor failures leave coverage incomplete.
Known static distributions give copy estimates. cav skips connected, animated,
expression-driven and unsupported counts. Distribution accessors can still take time.
The quick pass avoids explicit scene evaluation. Some metadata calls remain unbounded.

Stable warning kinds are `perf-copy-javascript`, `perf-nested-duplication`,
`perf-fanout`, `perf-javascript-distribution` and `perf-simulation`. Findings include
layer ID and name, connection and type evidence, `comp`, `compPath`, supported
`estimatedCopies`, a remedy and `attribution: structural-risk-not-measured`. JavaScript
on duplicated shape attributes or source layers can run for each copy. The Duplicator's
global transform does not count as per-copy work. Estimates cover the named duplication
connection. Total scene cost remains unmeasured.
See [Duplicator](https://cavalry.studio/docs/nodes/shapes/duplicator/),
[JavaScript layers](https://cavalry.studio/docs/nodes/general/javascript-layers/) and
[context propagation](https://cavalry.studio/docs/getting-started/key-concepts/context/).

Default visual checks inspect at most 200 layers and six frame evaluations. They also
limit animated attributes and keyframes. A `finally` block restores the playhead.
Visual jobs check original scene and comp identity before accessing the playhead,
including after resume. If motion inspection fails or skips work, cav does not infer
whole-comp stillness. Missing samples cannot establish a hold. Stateful simulations,
referenced pre-comps and incomplete layer-type coverage also skip these visual checks.
Jumping between selected frames could give misleading results. Review a sheet for blank
starts and ends; the pass does not measure blank-run duration. `failures`, `skipped`,
`complete` and `clean` describe coverage. Material failures or skips prevent a complete
clean verdict.

Profiling uses up to three representative frames in one chronological job. Simulations
or incomplete layer-type coverage use consecutive initial frames. The first sample is
labelled `first-evaluation-cache-state-unknown`; cav does not flush caches or establish
a cold scene. Later samples are `subsequent-evaluation`. `setFrameMs` measures the
setFrame call, and `renderPNGMs` separately measures optional 10 percent PNG rendering.
Lazy work can happen during the PNG call. These API times do not isolate GPU or render
cost. Profiling does not hide, freeze or change layers to assign cost.

To measure later regions, select up to twelve frames explicitly:

```sh
cav check --profile-frames 840,1560,1908 --max-warmup 2000 --timeout 30m --json
```

The command sorts selected frames, removes duplicates and checks the comp range. With
simulation or incomplete layer coverage, it starts at comp start and renders every
intervening frame at 10 percent. Selected frames also render at
10 percent, even without `--profile-render`, so the next warm-up segment starts from an
evaluated state. `--max-warmup` defaults to 600 and counts extra frames, excluding measured
samples; a selection exceeding it is refused before frame evaluation. The example with
start frame 0 has 1,906 warm-up frames and three measured samples. One warm-up image is
overwritten each time; it is not a PNG sequence export.

The `warmup` field reports counts, API time and the last warm-up frame separately from
sample timings. A warmed first sample is labelled
`first-measured-after-warmup-cache-state-unknown`. This label does not mean caches were
reset or a cold baseline was measured. Budget checks run between evaluations. Incomplete
warm-up produces no timing for its target. Selected samples do not establish whole-comp
performance or identify a costly layer. Repeat runs before attributing a speed change
to freezing or fewer keyframes.

Each sample writes cumulative progress to an operation-owned file that
`operation status` can read. A native call can exceed the CLI budget. After normal
completion or a recoverable error, the job restores the original playhead. Crashes and
force-quitting Cavalry can prevent restoration. Partial results show sample limits and
failures.
