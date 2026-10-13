# Cavalry scripting traps

Verified in Cavalry 2.7.2 unless a row says otherwise. "Helper" means the `cav` helper that
already handles the trap. Rows marked 2.8 were observed during launch-video production;
these observations have not all been independently reproduced.

## Layers and transforms

| Trap | Symptom | Fix | Helper |
|---|---|---|---|
| `api.create` / `api.primitive` add the layer next to the current selection. | Whole groups end up nested in a hidden group and never render. | Call `api.select([])` before and after every create. | all `cav.*` create helpers <!-- aliases: layer missing, invisible, not rendering, disappeared, nested, selection --> |
| `api.parent(child, parent)` keeps the child's world transform. | The child gets a compensating position and scale (for example `-95` and `1.282`). | Set position, scale and rotation again after parenting. | `cav.set(l, {parent})` <!-- aliases: parenting moves layer, jumps, wrong position, scale changed --> |
| Y points up; (0,0) is the comp centre. | Text at `y: 300` sits in the upper half. | Use `cav.comp()` edges: `top`, `bottom`, `left`, `right`. | `cav.comp()` <!-- aliases: upside down, flipped, wrong place, coordinates, origin --> |
| One number on a two-value (`double2`) attribute, e.g. `api.set(poly, {'generator.radius': 100})`. | The value silently becomes (0, 0); the shape has no size. | Pass `[100, 100]`. | `cav.attr`, `cav.polygon` <!-- aliases: zero size, invisible shape, radius ignored, size missing --> |
| `stroke.dashPattern` is a string. | An array throws `type must be string, but is array`. | `'12, 8'` (dash, gap). | `o.stroke.dash` accepts both <!-- aliases: dashed line, dashes, type error --> |
| `pivot` moves the layer so the pivot point sits at `position`. | Layer jumps when you change the pivot. | Move `position` by the same amount (times scale). | `cav.wipeIn` <!-- aliases: anchor point, layer jumps, origin --> |
| `api.reorder(a, b)` puts `a` below `b`. | A background ends up in front. | Reorder relative to the layer you want to sit under. | `cav.order(a, b)` <!-- aliases: layer order, z order, behind, front, hidden, covered --> |
| Group opacity in Individual Shapes mode (official Group docs). | Overlapping children show through each other during a fade. | Use Artboard mode to apply opacity to the group as one image (see official Group docs). | <!-- aliases: fade see through, transparency, overlap --> |
| Group in/out frames hide all children. | Children vanish outside the group's range. | Use this on purpose to cut between sections. | `o.in`, `o.out` <!-- aliases: layers disappear, vanish, missing, cut --> |
| `api.soloLayers([nested])` + `api.renderPNGFrame` | Blank render. | Solo the top-level ancestor, or render the full frame. | `cav frame` renders full frames <!-- aliases: blank frame, empty render, black frame, solo --> |

## Keyframes and easing

| Trap | Symptom | Fix | Helper |
|---|---|---|---|
| Magic Easing on a key shapes the segment that **starts** at that key. | The wrong move gets the ease. | Put the ease on the first key of each move. | `cav.key` <!-- aliases: wrong ease, easing ignored, curve --> |
| No easing means linear. | Robotic motion. | Always give an ease. | <!-- aliases: robotic, stiff, mechanical, linear --> |
| `'Custom'` easing takes an expression in `x` (0..1): `pow`, `exp`, `sin`, `cos`, ternaries all work. | | e.g. `1 - exp(-7*x)*cos(14*x)` is a spring. | `cav.E` <!-- aliases: spring, bounce, elastic, wobble, expression --> |
| Colour keys as hex strings. | Silently ignored: no keys. | Key `.r`, `.g`, `.b` separately. | `cav.key` with `'fill'` <!-- aliases: color keys, colour animation ignored, fill animation lost --> |
| String keys on `text`. | Silently ignored. | One layer per glyph, or cross-fade text layers. | `cav.glyphs` <!-- aliases: text animation ignored, typewriter, changing text --> |
| Chained keys interpolate across gaps. | A layer drifts slowly for seconds. | Add a hold key right before the next move. | `cav.tween` <!-- aliases: drift, sliding, creep, hold, slow move --> |
| Opacity is clamped to 0..100. | Overshoot curves do nothing on opacity. | Use overshoot on position/scale/rotation. | <!-- aliases: overshoot ignored, fade bounce --> |
| Keys on `'rotation'` (a 3-channel attribute). | Silently no keys. | Key `'rotation.z'`. | `cav.key` <!-- aliases: keys dropped, keys ignored, missing keys, lost keys, does not rotate, rotation does nothing, no spin, spinning, turn, angle --> |
| `api.get(textLayer, 'text')` returns an object `{text, overrides}`. | String code sees "[object Object]". | Read `.text`. | `cav.textOf(l)` <!-- aliases: object Object, text value, read text --> |
| `api.get` returns the value at the playhead. | A helper that reads "the current position" gets the wrong value when keys exist. | Read at a frame: `cav.valueAt(l, attr, f)`. | all recipes <!-- aliases: wrong value, current value, read attribute, time --> |

## Look and render

| Trap | Symptom | Fix | Helper |
|---|---|---|---|
| Stroke width scales with the layer's scale. | Expanding rings become very thick. | Animate `generator.radius`, not `scale`. | `cav.ring` <!-- aliases: thick outline, ring, line width grows --> |
| Strokes must be switched on first. | `stroke.*` sets do nothing. | `api.setStroke(l, true)` first. | `o.stroke` <!-- aliases: no outline, stroke missing, stroke ignored, invisible line --> |
| Trim uses `stroke.trim` + `stroke.trimStart`/`trimEnd` (0-100). | | | `cav.drawOn` <!-- aliases: draw on, write on, path animation, reveal line --> |
| Glow filter at high intensity on large text. | The word turns into a white blob (seen once; normal settings looked clean). | Keep `intensity` at 1 or lower, or use a blurred copy behind the text. | <!-- aliases: blown out, white blob, bloom --> |
| `chromaticAberrationFilter` at its default strength (100). | Broken colour noise. | Use `strength` 15-30. | <!-- aliases: colour noise, rgb split, glitch --> |
| Comp `motionBlur` alone. | The render is sharp. | Also set each layer's `motionBlur` to 1. | `cav.motionBlur()` <!-- aliases: no motion blur, blur missing, sharp render --> |
| A new render queue item. | Its `filePath` can point at another project's folder. | Always set `filePath` and `fileName`. | `cav render` <!-- aliases: wrong folder, output missing, render path, file location --> |
| Pre-comp background. | The pre-comp draws an opaque box. | Give the sub-comp a background with alpha 0. | <!-- aliases: black box, opaque background, transparency lost, alpha --> |
| Missing font. | Another font is used without an error. | Check `cavalry.fontExists(family, style)`. | `cav.text` warns; `cav check` reports it <!-- aliases: wrong font, font fallback, typeface --> |
| Filters and masks are connections, not attributes. | | `api.connect(filter,'id',shape,'filters')`, `api.connect(mask,'id',target,'masks')`. | `cav.filter`, `cav.mask` <!-- aliases: mask ignored, filter ignored, set filter, matte --> |
| Render queue range | Off-by-one frame counts were seen in one session. | `cav render` checks the count with ffprobe and prints it. | `cav render` <!-- aliases: frame count, missing frame, extra frame, off by one --> |
| `api.renderPNGFrame(path, scale)` appends `.png` and renders the frame set with `api.setFrame`. | | | `cav frame`, `cav sheet` <!-- aliases: png file name, still frame, snapshot --> |

## Launch-video observations (Cavalry 2.8)

| Trap | Symptom | Fix |
|---|---|---|
| Redeclaring a SkSL input uniform added in the Inputs UI. | The effect can silently fail to compile. | Use the input's name directly; Cavalry declares it. Keep required built-ins such as `uniform shader layer;` in filter code. <!-- aliases: shader does not compile, shader blank, uniform error --> |
| SkSL coordinates on a pre-comp reference. | A rim or SDF drawn around `fragCoord` stays at the comp centre and cuts across a card that is off centre. | `fragCoord` is centred on the filtered layer and y-up, and `resolution` is that layer's size. A pre-comp reference is as large as its comp, so its centre is the comp centre. Pass the card's centre and size as inputs, or derive edges from `layer.eval(...)` alpha. On a plain shape the coordinates follow the shape (checked in 2.8.0). <!-- aliases: shader offset, wrong centre, fragCoord --> |
| A filter attached to a group. | In the observed scene it filtered each child instead of the composite. | For an effect over the flattened image, filter a pre-comp reference; verify the result on overlapping children. <!-- aliases: group filter per child, composite --> |
| Hard-coded variable-font `fontAxes.N` indices. | A different axis animates without an error. | Inspect the selected font's axis order (its `fvar` table); indices are not portable between fonts. <!-- aliases: font weight animation, wrong axis, variable font --> |
| JavaScript output driving a Duplicator's `shapeRotation`. | Returning a scalar produces no rotation. | Return `{x: 0, y: 0, z: angle}` for the full attribute, or connect a scalar to `.z`. <!-- aliases: copies do not rotate, rotation ignored, no spin, duplicator rotation does nothing --> |
| JavaScript output driving colour. | A hex string renders black. | Return a colour object `{r, g, b, a}` with the destination's channel ranges. <!-- aliases: black colour, color black, wrong colour --> |
| Setting Look At's `target` as a value. | The copies do not aim at it. | Connect the target's `id` to Look At's `target`; adjust Offset for the source orientation (the observed +x arrow needed 90 degrees). <!-- aliases: look at ignored, aim, point at, orient --> |
| Swapping whole text for individual glyph layers. | Kerning jumps at the hand-off. | Keep one text layer with Sub-Mesh where possible, or compare the two rendered boundary frames and align the glyphs. <!-- aliases: kerning jump, letters jump, glyph hand off --> |
| Shake or section boundaries. | The first shake frame retains an offset, or a covering shape arrives after a section ends. | Inspect frames f−1 and f at each hand-off, preferably from the final video so simulation state is correct. <!-- aliases: jump at cut, pop, glitch at boundary, offset --> |

## Scripts and the bridge

- Code runs inside a function: use `return` to send a value back. Values must be JSON-safe.
- `var` variables do not survive between runs. `globalThis.x = ...` survives until the bridge restarts.
- Scripts slow down as a scene grows. Build in a few larger scripts, and use native
  duplicators instead of hundreds of layers.
- Connect masks and filters last: fill the group and key its children first, then call
  `cav.mask`, `cav.filter` or `cav.sksl`. One user reported a masked build that went from
  over 180 s to 14 s after this change. In our own checks with up to 480 layers, the order
  made no difference, so the cause is not known. The order costs nothing, so use it.
- Deleting many layers from the active comp is quadratic in Cavalry 2.8: 1,500 layers took
  65 s one by one. `cav.clear()` deletes them while an empty comp is active, which took 4 s.
  To start a big build again from nothing, `cav scene new --force` is faster still.
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
