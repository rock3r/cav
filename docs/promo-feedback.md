# Launch-video feedback implementation

All eight feedback areas now have an implementation or a verified existing behavior.
The feedback came from Cavalry 2.8 and PR #1 at 7232fd4. It described observations and
suggestions; the implementations below retain bounded checks and explicit recovery.

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

Two successive `cav.key` calls on `position.x` preserved all four frames, 0, 10, 20 and 30,
in Cavalry 2.8. The helper submits keys without disconnecting the input. An automated
regression verifies both calls. No append/merge option is required for this case.
Inspect intervening setters and connections before diagnosing an attribute-specific issue.

An isolated native 2D Duplicator used scalar `shapeRotation`, despite the earlier report
that all full rotation required a vector. Forcing a vector left its copies upright. The
helper follows the native type, handles scalar/vector rotation, and converts source-fill
colours. A rendered fixture visibly rotated the red/green copies. Unknown paths such as
`shapeColor` are rejected instead of creating a useless connection.

Native testing also established that `getOutFrame` returns the first invisible frame,
while `setOutFrame` accepts the inclusive last-visible frame. Seam candidates use the
getter directly. A layer set to out 9 therefore hands off at frame 10, not frame 11.

## Practical bounds

- Seam differences are review signals. The command does not infer creative intent or
  map boundaries through reference time remapping. Inspect referenced comps separately.
- Metadata truncation/failures and omitted boundaries are explicit. Risk warnings do
  not attribute measured render time to a node.
- Font axes come from ordered installed OpenType `fvar`/`name` tables. Native names/order
  must match; unknown/ambiguous metadata fails instead of guessing numeric indices.
  Discovery has a 10000-entry cap and exposes no maps after an incomplete scan.
- New chunks replay simulations from comp start, within `--max-warmup`; validated output
  is reused but simulation evaluation can still be expensive. At most 1000 chunks.
- Ordinary resume never resubmits uncertain work. Explicit chunk restart requires
  acknowledged reconciliation, unchanged inputs and a different idle bridge session.
- Comp switching can mark Cavalry's scene unsaved. Chunk rendering requires an explicit
  save. The native tests use disposable scenes and restore the owner's saved launch scene.

Font metadata references: [OpenType fvar](https://learn.microsoft.com/en-us/typography/opentype/spec/fvar)
and [OpenType name](https://learn.microsoft.com/en-us/typography/opentype/spec/name).
Native evidence is retained locally under `validation-1.1/remaining/`. Automated fixtures
run in CI on Linux and macOS; native Cavalry tests were run on macOS with Cavalry 2.8.
