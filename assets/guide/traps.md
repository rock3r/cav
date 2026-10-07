# Cavalry scripting traps

Verified in Cavalry 2.7.2 unless a row says otherwise. "Helper" means the `cav` helper that
already handles the trap. Rows marked 2.8 were observed during launch-video production;
these observations have not all been independently reproduced.

## Layers and transforms

| Trap | Symptom | Fix | Helper |
|---|---|---|---|
| `api.create` / `api.primitive` add the layer next to the current selection. | Whole groups end up nested in a hidden group and never render. | Call `api.select([])` before and after every create. | all `cav.*` create helpers |
| `api.parent(child, parent)` keeps the child's world transform. | The child gets a compensating position and scale (for example `-95` and `1.282`). | Set position, scale and rotation again after parenting. | `cav.set(l, {parent})` |
| Y points up; (0,0) is the comp centre. | Text at `y: 300` sits in the upper half. | Use `cav.comp()` edges: `top`, `bottom`, `left`, `right`. | `cav.comp()` |
| One number on a two-value (`double2`) attribute, e.g. `api.set(poly, {'generator.radius': 100})`. | The value silently becomes (0, 0); the shape has no size. | Pass `[100, 100]`. | `cav.attr`, `cav.polygon` |
| `stroke.dashPattern` is a string. | An array throws `type must be string, but is array`. | `'12, 8'` (dash, gap). | `o.stroke.dash` accepts both |
| `pivot` moves the layer so the pivot point sits at `position`. | Layer jumps when you change the pivot. | Move `position` by the same amount (times scale). | `cav.wipeIn` |
| `api.reorder(a, b)` puts `a` below `b`. | A background ends up in front. | Reorder relative to the layer you want to sit under. | `cav.order(a, b)` |
| Group opacity in Individual Shapes mode (official Group docs). | Overlapping children show through each other during a fade. | Use Artboard mode to apply opacity to the group as one image (see official Group docs). | |
| Group in/out frames hide all children. | Children vanish outside the group's range. | Use this on purpose to cut between sections. | `o.in`, `o.out` |
| `api.soloLayers([nested])` + `api.renderPNGFrame` | Blank render. | Solo the top-level ancestor, or render the full frame. | `cav frame` renders full frames |

## Keyframes and easing

| Trap | Symptom | Fix | Helper |
|---|---|---|---|
| Magic Easing on a key shapes the segment that **starts** at that key. | The wrong move gets the ease. | Put the ease on the first key of each move. | `cav.key` |
| No easing means linear. | Robotic motion. | Always give an ease. | |
| `'Custom'` easing takes an expression in `x` (0..1): `pow`, `exp`, `sin`, `cos`, ternaries all work. | | e.g. `1 - exp(-7*x)*cos(14*x)` is a spring. | `cav.E` |
| Colour keys as hex strings. | Silently ignored: no keys. | Key `.r`, `.g`, `.b` separately. | `cav.key` with `'fill'` |
| String keys on `text`. | Silently ignored. | One layer per glyph, or cross-fade text layers. | `cav.glyphs` |
| Chained keys interpolate across gaps. | A layer drifts slowly for seconds. | Add a hold key right before the next move. | `cav.tween` |
| Opacity is clamped to 0..100. | Overshoot curves do nothing on opacity. | Use overshoot on position/scale/rotation. | |
| Keys on `'rotation'` (a 3-channel attribute). | Silently no keys. | Key `'rotation.z'`. | `cav.key` |
| `api.get(textLayer, 'text')` returns an object `{text, overrides}`. | String code sees "[object Object]". | Read `.text`. | `cav.textOf(l)` |
| `api.get` returns the value at the playhead. | A helper that reads "the current position" gets the wrong value when keys exist. | Read at a frame: `cav.valueAt(l, attr, f)`. | all recipes |

## Look and render

| Trap | Symptom | Fix | Helper |
|---|---|---|---|
| Stroke width scales with the layer's scale. | Expanding rings become very thick. | Animate `generator.radius`, not `scale`. | `cav.ring` |
| Strokes must be switched on first. | `stroke.*` sets do nothing. | `api.setStroke(l, true)` first. | `o.stroke` |
| Trim uses `stroke.trim` + `stroke.trimStart`/`trimEnd` (0-100). | | | `cav.drawOn` |
| Glow filter at high intensity on large text. | The word turns into a white blob (seen once; normal settings looked clean). | Keep `intensity` at 1 or lower, or use a blurred copy behind the text. | |
| `chromaticAberrationFilter` at its default strength (100). | Broken colour noise. | Use `strength` 15-30. | |
| Comp `motionBlur` alone. | The render is sharp. | Also set each layer's `motionBlur` to 1. | `cav.motionBlur()` |
| A new render queue item. | Its `filePath` can point at another project's folder. | Always set `filePath` and `fileName`. | `cav render` |
| Pre-comp background. | The pre-comp draws an opaque box. | Give the sub-comp a background with alpha 0. | |
| Missing font. | Another font is used without an error. | Check `cavalry.fontExists(family, style)`. | `cav.text` warns; `cav check` reports it |
| Filters and masks are connections, not attributes. | | `api.connect(filter,'id',shape,'filters')`, `api.connect(mask,'id',target,'masks')`. | `cav.filter`, `cav.mask` |
| Render queue range | Off-by-one frame counts were seen in one session. | `cav render` checks the count with ffprobe and prints it. | `cav render` |
| `api.renderPNGFrame(path, scale)` appends `.png` and renders the frame set with `api.setFrame`. | | | `cav frame`, `cav sheet` |

## Launch-video observations (Cavalry 2.8)

| Trap | Symptom | Fix |
|---|---|---|
| Redeclaring a SkSL input uniform added in the Inputs UI. | The effect can silently fail to compile. | Use the input's name directly; Cavalry declares it. Keep required built-ins such as `uniform shader layer;` in filter code. |
| A filter attached to a group. | In the observed scene it filtered each child instead of the composite. | For an effect over the flattened image, filter a pre-comp reference; verify the result on overlapping children. |
| Hard-coded variable-font `fontAxes.N` indices. | A different axis animates without an error. | Inspect the selected font's axis order (its `fvar` table); indices are not portable between fonts. |
| JavaScript output driving a Duplicator's `shapeRotation`. | Returning a scalar produces no rotation. | Return `{x: 0, y: 0, z: angle}` for the full attribute, or connect a scalar to `.z`. |
| JavaScript output driving colour. | A hex string renders black. | Return a colour object `{r, g, b, a}` with the destination's channel ranges. |
| Setting Look At's `target` as a value. | The copies do not aim at it. | Connect the target's `id` to Look At's `target`; adjust Offset for the source orientation (the observed +x arrow needed 90 degrees). |
| Swapping whole text for individual glyph layers. | Kerning jumps at the hand-off. | Keep one text layer with Sub-Mesh where possible, or compare the two rendered boundary frames and align the glyphs. |
| Shake or section boundaries. | The first shake frame retains an offset, or a covering shape arrives after a section ends. | Inspect frames f−1 and f at each hand-off, preferably from the final video so simulation state is correct. |

## Scripts and the bridge

- Code runs inside a function: use `return` to send a value back. Values must be JSON-safe.
- `var` variables do not survive between runs. `globalThis.x = ...` survives until the bridge restarts.
- Scripts slow down as a scene grows (believed, not measured). Build in a few larger scripts,
  and use native duplicators instead of hundreds of layers.
- A timeout does not prove a native job stopped. Inspect/resume the printed operation ID
  with `cav operation status/resume <id>`; never retry the original command. Raw
  `cav job wait <job-id>` waits only for that job and cannot continue render phases.
- An MP4 that stops growing is not a reliable job-status signal. Use operation status/resume,
  keep partial artifacts, and stop automatic waiting on exit 4 (bridge disconnect/session
  change). A missing crash report does not establish an out-of-memory kill. A success result
  still needs the requested frame count before publication; never silently retry or chunk.
- `api.newScene()` throws away unsaved work without asking. `cav scene new` refuses when there
  are unsaved changes; never pass `--force` on the user's own scene without asking.
- The first time the bridge starts, Cavalry asks "Do you trust this script?". The user must answer.


### Repeated key calls

Two `cav.key` calls preserve earlier keys on a plain animated attribute in Cavalry 2.8.
The helper submits each frame through `api.keyframe`; it does not disconnect the input.
Inspect `cav.keys(layer)` after intervening setters/connections before assuming replacement.
`api.disconnectInput` can delete keys. No merge option is needed for the verified case.

### Boundary frame numbering

`api.getOutFrame` is the first invisible frame in Cavalry 2.8, while `setOutFrame` takes
an inclusive last-visible frame. `cav seams` compares the first invisible frame against
its predecessor. A layer set to out 9 hands off at frame 10.
