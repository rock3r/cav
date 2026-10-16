# Launch-video feedback implementation

The project addressed all eight feedback areas through new code or checked existing
behavior. The feedback came from Cavalry 2.8 and PR #1 at 7232fd4. The table separates
each implementation from the check that supports it.

| Feedback | Implemented behavior | Verification |
| --- | --- | --- |
| Section hand-off jumps | `seams` compares f-1/f at in/out and opacity keys, reports changed-pixel fractions and boxes, and annotates intended cuts. Finished full-comp videos are supported. | Native and video fixtures both found the sole cut at frame 10 with a full-frame box. The explicit intended cut suppressed its review flag. |
| Check coverage | Existing recursive pre-comp checks/profile selection remain. Added SkSL, input redeclaration, reference size, dense-key, key-volume and group-filter risks. | Metadata fixtures verify nested paths, bounded queries, omissions and risk attribution. |
| Scene size/opening | Existing resumable scene open remains. Scene info adds bounded layer/comp node and animated-key counts, count coverage and key-volume warnings. `driver` supplies a two-key clock. | Native scene info and driver creation/rendering passed. The large-scene open-time observation is not treated as causal proof. |
| Silent helper failures | Added SkSL, pre-comp, font-axis, typed per-copy driver, connected Look At and explicit above/below helpers. Group opacity guidance stays mode-aware. | Native helper smoke tests plus fixtures for declarations, comp restoration on error, axis order/ranges, return types, target connection and frontmost sibling ordering. |
| Render resilience | `render --save` saves the named scene. `--chunk-frames` validates segments and records hashes/counts. Explicit restart reconciliation retains uncertain attempts and reuses completed segments. | Native three-chunk render with audio passed. A separate 180-frame render timed out after two validated segments; after verifying native completion and restarting the idle bridge, explicit recovery completed six chunks with the first two unchanged. |
| Simulation preview | `frames --keep-every` evaluates intervening frames chronologically from comp start and retains selected PNGs. | Six moving-particle frames matched consecutive native renders byte for byte. The particle recipe now connects native time inputs explicitly. |
| Multiple comps | Frame, sheet, frames, check and seams accept an ID or unique comp name and restore comp/playhead in native finally. | Native frame, sheet and quick check passed. Fixtures cover native errors and duplicate names without switching. |
| Smaller items | Inclusive scene ranges, fps presets and offline installed-bridge warnings. Status adds running-version warnings. Exact-title docs search was already merged. | Native range/fps setup passed. Fixtures verify range validation, installed/running mismatch and no network call from plain version. |

## Corrected native observations

In Cavalry 2.8, two successive `cav.key` calls on `position.x` kept keys at frames 0,
10, 20 and 30. The helper does not disconnect the input, and a regression check covers
both calls. This case needs no append or merge option. Inspect setters and connections
between calls before diagnosing another attribute.

An isolated native 2D Duplicator used scalar `shapeRotation`. Vector output left the
copies upright. The helper reads the native type, handles scalar and vector rotation,
and converts source-fill colours. A rendered fixture showed rotating red and green
copies. The helper rejects unknown paths such as `shapeColor`.

Native tests found that `getOutFrame` returns the first invisible frame and
`setOutFrame` accepts the last visible frame. Seam detection uses the getter. A layer
set to out 9 hands off at frame 10.

## Practical bounds

- Seam differences identify frames to review. The command cannot infer creative intent
  or map boundaries through reference time remapping. Inspect referenced comps separately.
- Reports identify truncated metadata, failures and omitted boundaries. Risk warnings
  do not assign measured render time to a node.
- Font axes come from ordered installed OpenType `fvar` and `name` tables. Native names
  and order must match. Unknown or ambiguous metadata causes failure. Discovery stops
  at 10000 entries and gives no axis maps after an incomplete scan.
- New chunks replay simulations from composition start within `--max-warmup`. cav reuses
  validated output. Simulation evaluation can still be costly. The limit is 1000 chunks.
- Ordinary resume does not resubmit uncertain work. An explicit chunk restart requires
  acknowledged reconciliation, unchanged inputs and a different idle bridge session.
- Switching compositions can mark the scene unsaved. Save it before chunk rendering.
  Native tests used disposable scenes and restored the owner's saved launch scene.

Font metadata references: [OpenType fvar](https://learn.microsoft.com/en-us/typography/opentype/spec/fvar)
and [OpenType name](https://learn.microsoft.com/en-us/typography/opentype/spec/name).
Native evidence remains under `validation-1.1/remaining/`. CI runs automated fixtures
on Linux and macOS. The native Cavalry tests ran on macOS with Cavalry 2.8.
