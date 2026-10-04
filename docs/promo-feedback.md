# Launch-video feedback disposition

Feedback came from a 60-second Cavalry 2.8 production scene and validation of PR #1 at
7232fd4. This document distinguishes shipped behavior, this follow-up, and proposals that
still need design or reproduction. It does not approve the proposed commands or helpers.

## Already shipped in 1.1.0

- Scene opening persists native loading and follow-up metadata as a resumable operation,
  with a total timeout and input hash. Resume does not reopen a still-loading scene.
- Structural checks inspect referenced pre-comps and preserve comp paths, including
  JavaScript connected to Duplicator source children. Bounded omissions are explicit.
- `--profile-frames` reaches selected later sections, with chronological simulation warm-up,
  a bounded warm-up allowance, coverage and separate warm-up timings/progress.

These fixes address the feedback's original scene-open, nested-driver and intro-only
profiling gaps. See [1.1 validation](validation-1.1.md) for the actual test limits.

## This follow-up

- Offline docs search prefers the longest complete page title at the query's start,
  case-insensitively and with normalized whitespace/punctuation. `Look At duplicator` now prioritizes
  Look At; BM25 still ranks sections within that title. API search is unchanged.
- `cav guide traps` and `cav guide native` document the production traps: automatic SkSL
  input uniforms (distinct from required built-ins), per-copy JavaScript output types and
  context, connected Look At targets, variable-font axis indices and text hand-off seams.
- Group opacity guidance is mode-aware: official docs distinguish Individual Shapes from
  Artboard. The reported per-child group filter remains a scoped production observation.
- Dense-key guidance recommends considering procedural drivers and comparing repeated
  measurements. The scene-size/open-time observation after a restart is not causal proof.
- `cav.order(a, b)` already documents that a goes below b; no rename or API break is needed.

New production observations are labelled as such, rather than presented as independently
reproduced native tests. Primary references: [JavaScript Layers](https://cavalry.studio/docs/nodes/general/javascript-layers/),
[JavaScript Utility](https://cavalry.studio/docs/nodes/utilities/javascript-utility/),
[Look At](https://cavalry.studio/docs/nodes/behaviours/look-at/),
[SkSL Filter](https://cavalry.studio/docs/nodes/effects/filters/sksl-filter/) and
[Group](https://cavalry.studio/docs/nodes/shapes/group/).

## Follow-up candidates, in suggested order

1. **Seam inspection.** Compare f−1/f at bounded in/out and opacity boundaries, preferably
   decoded from a completed video for correct simulation state. Report changed pixels and
   bounding boxes, allow explicit intended-cut annotations, and treat difference as a review
   signal rather than proof of a bug. Define frame numbering, endpoint handling, candidate
   limits and artifact validation before adding a command. Never auto-correct scene cuts.
2. **Additional cheap risk findings.** Consider SkSL, reference complexity and dense keys.
   Bound metadata queries, distinguish unavailable data from zero, avoid evaluating unsafe
   dependency graphs, and label these as heuristics rather than measured attribution.
   Group filter warnings need mode/effect context to avoid flagging legitimate per-child use.
3. **Chronological sparse previews and comp selection.** Reuse warm-up accounting for a
   proposed `frames --keep-every` command. Bound evaluated and retained frame counts. A comp
   selector should resolve a unique ID, guard scene/comp identity, and restore both comp and
   playhead on success, failure and recovery; duplicate names must not pick silently.
4. **Chunked rendering.** Define validated segment manifests, scene hashes, frame counts,
   audio/mux boundaries and final checks. Simulations cannot safely restart at a chunk's
   first frame without replay from comp start or verified cached state. Never resubmit native
   work whose outcome is unknown. Preserve partial artifacts after disconnects. A proposed
   `render --save` must explicitly save the intended scene before submission.
5. **Helpers and presets.** SkSL, pre-comp, font-axis and driver wrappers need type/attribute
   discovery and native fixtures. Do not strip all uniform declarations, guess font axes or
   switch comps without restoring them. Scene range/fps presets can be scoped separately.
6. **Bridge-version messaging.** Status can report mismatch from its existing bridge probe.
   Keep `version` offline: its embedded bridge/helper versions are not a running-bridge
   assertion. Do not add a network dependency to the offline version command.

## Needs a minimal native reproduction

The feedback says a second `cav.key` call deletes earlier keys. Current helper code calls
`api.keyframe` for each supplied frame and does not disconnect the input; `cav.tween` uses
that same helper, and chained native tween validation passed for 1.1. This does not rule out
an attribute-specific native issue. Reproduce two key calls on one fresh attribute, inspect
keyframe times after each, then separately try setters and `api.disconnectInput` (which
explicitly deletes keyframes). Record the attribute, Cavalry version and intervening calls
before introducing merge options or warnings.
