# Operation recovery and performance diagnostics

`cav render`, `cav check` and `cav run` record a complete operation separately from its
bridge jobs. Other commands retain their raw-job behavior. An operation ID identifies the
command, original working directory, transport, phase, jobs, intended output, partial
results and completion checkpoints. Records under `~/.cav/operations/` are private (0600)
and retained until the operator removes them; source and results can contain scene data.

## Recover a timeout

```sh
cav render -o renders/final.mp4 --timeout 90m --json
cav operation status <operation-id> --json
cav operation resume <operation-id> --timeout 90m --json
```

The timeout is one budget for preparation, bridge waits, rendering, muxing and validation.
A deadline that interrupts ffmpeg or ffprobe returns exit 3 and retains staged output;
it does not classify the killed subprocess as a mux/validation failure.
Script input preparation also observes the budget, including stdin waiting for EOF and
named-pipe paths. An input timeout before any job was prepared means no native submission
occurred. A partial stream cannot be reconstructed on resume: preserve the complete source
and supply it to a new run instead. The incomplete local record is retained for inspection.
Audio hashing uses a streaming owned helper under the same deadline; opening a FIFO without
a writer or reading a stalled stream cannot retain the operation lock indefinitely.
These preparation guarantees apply to direct operation dispatch. The existing spool-client
forwarding path reads stdin before operation dispatch and does not yet bound that input read.
A resume gets a fresh budget. It reads completed job checkpoints and waits for a pending
job, then continues the remaining command phases. Waiting for a metadata job with
`cav job wait <job-id>` completes only that job; it does not start rendering. Raw job-wait
remains available, including for older CLI jobs.

Never retry the original render/build just because cav returned 3. Submission intent and
the job ID are saved **before** POST. If acknowledgement is lost, delivery is `uncertain`;
resumption only looks for that same job's result, including with an older running bridge.
It never posts that job again. A `prepared` checkpoint that has not attempted POST can be
submitted safely. New bridges also return stored results for completed duplicate IDs.

Checkpoint publication syncs file data and the parent directory after rename on Unix;
Windows uses [MoveFileExW](https://learn.microsoft.com/windows/win32/api/winbase/nf-winbase-movefileexw)
with replacement and write-through publication. Newly created checkpoint directories use
the same namespace barrier. A barrier failure prevents POST. This relies on the underlying
filesystem honoring its persistence primitives; fixtures do not simulate a machine power cut.

`cav status`, `cav job status <id>` and `cav operation status <id>` read existing state.
They never queue metadata or rendering. `cav scene info` explicitly retrieves scene
metadata. Status may be `queued`, `running`, `busy` or `unknown`; a completed preparatory
job makes an operation `ready-to-resume`, rather than complete. Inspection does not update
checkpoint timestamps. Silence cannot distinguish a blocking native call from a stopped
app. A heartbeat or cancellation cannot interrupt a native call inside Cavalry.

A refused HTTP bridge connection ends a job wait promptly with exit 4 and
`failureReason: bridge-disconnected`. A new session on an updated bridge returns
`bridge-session-changed`, including when the original job's script file remains on disk.
The operation's native outcome stays `unknown`: a lost bridge does not by itself prove
that Cavalry crashed or that its output was lost. A stored result for the original job
still takes precedence. Older bridges without session IDs and remote spool transports
cannot provide the same restart evidence; their unresolved outcomes need inspection.
Timeouts from a blocked native call remain `busy` or `unknown`, with no automatic retry.

A kernel lock prevents simultaneous operation clients sharing `CAV_HOME`. After a CLI
exits, a pending native job still gates new operations on the same transport. cav also
runs the same gate before any new submission during resume, excluding the resumed
operation itself. Reading completed results or waiting on a recorded job remains allowed.
The relay runs forwarded operations in its own state folder; inspect/resume through the same relay.
Raw commands, independent `CAV_HOME` folders, other tools and GUI edits are outside this
coordination. Avoid them until the operation completes.

Keep the input scene and audio intact during recovery. Audio content hashes, the original
default output directory and helper-library version are bound to the checkpoints. New bridges expose a session ID;
cav refuses to submit remaining jobs into a different bridge window. Older bridges have
no session identity, so the operator must confirm the original scene/session. Renders and
profiles also check the active comp and scene path. These checks cannot detect all edits
inside the same scene. Keep the same CLI build while resuming: changed generated job code
is rejected. After a script job was prepared, a recovered `run` uses that recorded script
rather than rereading changed files or stdin; async runs become synchronous waits on recovery.
Input preparation that never captured a script cannot be resumed.

## Stale or failed operations

An unknown result may have expired from the bridge's one-day cache or belonged to a
bridge that restarted. cav does not infer success, prune recovery records or re-submit.
Inspect the native output and original scene first. Recoverable local failures (for
example a missing ffprobe) can be resolved and resumed. A failed script's cached result
is returned again, without executing its scene mutations a second time.

After manually reconciling an unknown outcome, release its local gate explicitly:

```sh
cav operation abandon <id> --acknowledge-unknown-outcome
```

This preserves the record, marks local recovery abandoned, and **does not cancel** native
work. Do not resume abandoned operations. Completed records need no abandonment.

Render output is isolated in a `.cav-<operation-id>` directory beside the target. Native
video, partial mux and completed mux are distinct. A completed mux is preserved even if
its checkpoint was lost. ffprobe must confirm the requested frame count before final
publication. Hard-link publication does not overwrite existing output and can recognize
an already-published result after a crash. The destination filesystem must support hard
links. Staging and prior installed app bundles are retained as recovery evidence; clean
them manually only after confirming completion. Render queue items use operation-specific
names and are not automatically removed from the scene.

## Render stopped partway through

Use `cav render` and its operation ID instead of a loop that only watches the MP4 grow.
`operation status` reads staged artifact sizes, modification times and ages, plus the
expected frame count, without submitting a scene query or running ffprobe. These files
are explicitly `unvalidated`. A large file is not a finished render, and an unchanged
file may reflect a slow frame, buffering, a closed bridge or an exited app. File activity
alone never changes the operation's state or justifies restarting it.

When the bridge disconnects or restarts, stop automatic waiting and inspect the saved
scene, original result and staged output. Preserve partial files. Resuming the operation
only waits for the recorded native job and never re-submits it, even after an app restart.
If the original call cannot finish, reconcile the unknown outcome and explicitly abandon
that operation before attempting a new render to a new output path. cav does not relaunch
the app, silently start shorter chunks, remove partial artifacts or infer an OOM kill
from a missing crash report. A job reporting success must still pass exact frame-count
validation before publication; a short or unreadable video is rejected and retained.

## Choose an inspection cost

```sh
cav check --quick --json
cav check --timeout 2m --json
cav check --profile --samples 3 --timeout 2m --json
cav check --profile-render --samples 3 --timeout 2m --json
```

The cheap pass reads comp/layer metadata and connections. It does not move the playhead,
compute bounding boxes or render. Defaults bound metadata to 1,000 layers and 10,000
connections, with time checks between layers. Known static distributions provide copy
estimates; connected, animated, expression-driven and unsupported counts are omitted and
reported as skipped. Distribution values may themselves invoke native accessors; cheap
means avoiding explicit scene evaluation, not a guarantee that every metadata call returns
immediately.

Stable warning kinds are `perf-copy-javascript`, `perf-nested-duplication`, `perf-fanout`
, `perf-javascript-distribution` and `perf-simulation`. Findings carry the layer ID/name, connection/type evidence,
`estimatedCopies` when supported, a remedy and `attribution: structural-risk-not-measured`.
JavaScript on Duplicator shape attributes or its duplicated source can multiply across
index context. The Duplicator's global transform is not labelled per-copy. Estimates are
scoped to the named duplication connection; they are not a complete scene cost model.
See [Duplicator](https://cavalry.studio/docs/nodes/shapes/duplicator/),
[JavaScript layers](https://cavalry.studio/docs/nodes/general/javascript-layers/) and
[context propagation](https://cavalry.studio/docs/getting-started/key-concepts/context/).

Default visual checks inspect at most 200 layers and six frame evaluations, with bounded
animated attributes/keyframes. They restore the original playhead in `finally`.
Visual jobs verify the original scene/comp identity before reading or changing the playhead,
including after a resume. Whole-comp stillness conclusions are skipped when motion
inspection has failed or skipped work; missing samples are not evidence of a hold.
Stateful simulations, or incomplete layer-type coverage, skip these visual checks because
jumping among resting/key frames would give
misleading results. Blank-run duration is skipped; review a sheet to assess empty heads
and tails. `failures`, `skipped`, `complete` and `clean` make this coverage explicit. No
complete clean verdict is emitted when a material inspection failed or was skipped.

Profiling uses up to three representative frames, or consecutive initial frames for
simulations or incomplete layer-type coverage, in one chronological job. The first sample is labelled
`first-evaluation-cache-state-unknown`; cav does not flush caches or promise a cold scene.
Later samples are `subsequent-evaluation`. `setFrameMs` times the setFrame call;
`renderPNGMs` separately times optional 10 percent PNG rendering. Lazy evaluation can
occur inside the PNG call, so these are API timings rather than exclusive GPU/render
attribution. No layer is hidden, frozen or modified to attribute cost.

Each returned sample writes cumulative progress to an operation-owned file visible in
`operation status`. A native call can overrun the total CLI budget; the job restores the
original playhead when it returns on normal completion or a recoverable error. Crashes or
force-quitting Cavalry cannot guarantee restoration. Sample limits and failures remain
explicit in partial results.
