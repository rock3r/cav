# Cavalry scripting traps

Verified in Cavalry 2.7.2 unless a row says otherwise. "Helper" means the `cav` helper that
already handles the trap.

## Layers and transforms

| Trap | Symptom | Fix | Helper |
|---|---|---|---|
| `api.create` / `api.primitive` add the layer next to the current selection. | Whole groups end up nested in a hidden group and never render. | Call `api.select([])` before and after every create. | all `cav.*` create helpers |
| `api.parent(child, parent)` keeps the child's world transform. | The child gets a compensating position and scale (for example `-95` and `1.282`). | Set position, scale and rotation again after parenting. | `cav.set(l, {parent})` |
| Y points up; (0,0) is the comp centre. | Text at `y: 300` sits in the upper half. | Use `cav.comp()` edges: `top`, `bottom`, `left`, `right`. | `cav.comp()` |
| `pivot` moves the layer so the pivot point sits at `position`. | Layer jumps when you change the pivot. | Move `position` by the same amount (times scale). | `cav.wipeIn` |
| `api.reorder(a, b)` puts `a` below `b`. | A background ends up in front. | Reorder relative to the layer you want to sit under. | `cav.order(a, b)` |
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
| Missing font. | Another font is used without an error. | Check `cavalry.fontExists(family, style)`. | `cav.text` warns |
| Filters and masks are connections, not attributes. | | `api.connect(filter,'id',shape,'filters')`, `api.connect(mask,'id',target,'masks')`. | `cav.filter`, `cav.mask` |
| Render queue range | Off-by-one frame counts were seen in one session. | `cav render` checks the count with ffprobe and prints it. | `cav render` |
| `api.renderPNGFrame(path, scale)` appends `.png` and renders the frame set with `api.setFrame`. | | | `cav frame`, `cav sheet` |

## Scripts and the bridge

- Code runs inside a function: use `return` to send a value back. Values must be JSON-safe.
- `var` variables do not survive between runs. `globalThis.x = ...` survives until the bridge restarts.
- Scripts slow down as a scene grows (believed, not measured). Build in a few larger scripts,
  and use native duplicators instead of hundreds of layers.
- A run that takes longer than `--timeout` is still running. `cav job wait <id>` picks it up.
- `api.newScene()` throws away unsaved work without asking. `cav scene new` refuses when there
  are unsaved changes; never pass `--force` on the user's own scene without asking.
- The first time the bridge starts, Cavalry asks "Do you trust this script?". The user must answer.
