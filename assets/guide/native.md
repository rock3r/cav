# Native Cavalry features

Native features can replace many separate layers. A duplicator with 50 copies took less
script and render work than 50 layers in the original tests. The first recipes ran in
Cavalry 2.7.2. Later sections identify official documentation or Cavalry 2.8 launch-video
observations. "Pro" marks layer types that require a Pro licence. Check with
`cav.pro(type)`; Starter cannot save or render those types.

## Rules for every node

- Connect a behaviour or utility with `api.connect(node, 'id', layer, attr)`. The
  connection **replaces** the attribute value. An oscillator from -10 to 10 on
  `position.y` moves a shape around y = 0, even if the shape was at `y = 200`.
  Put the shape in a positioned group or include the base value in the node's range.
- Connect to one channel with paths such as `position.y`, `scale.x`, `rotation.z`
  or `shapePosition.y`.
- `rotation` has x, y and z. `cav.key(l, 'rotation', ...)` keys `rotation.z`; raw `api.keyframe`
  on `'rotation'` makes no keys.
- Keyframes are an input connection too: `api.disconnectInput(l, attr)` deletes them.

## Duplicator (repeat a shape)

```js
var cell = cav.rect('cell', 24, 24, { fill: '#4cc9f0' })
var grid = cav.duplicator('grid', cell, { type: 'grid', count: [8, 5], size: [700, 400] })
var ring = cav.duplicator('ring', cav.circle('dot', 8), { type: 'circle', count: 24, radius: 260 })
var line = cav.duplicator('line', cav.rect('bar', 30, 120), { type: 'linear', count: 12, size: 900 })
var dust = cav.duplicator('dust', cav.circle('p', 3), { type: 'random', count: 80, size: [1600, 900], seed: 4 })
var onPath = cav.duplicator('onPath', cav.star('s', 5, 16), { type: 'path', count: 10, path: somePath })
```

- The duplicator hides its source and draws the copies. Move, scale and fade the
  duplicator like another layer (`cav.pop(grid, 30)`).
- The duplicator **ignores the source's position, scale and rotation keys**. It applies
  opacity, size and colour keys to the copies. For per-copy motion, animate a shape in a
  group and use that group as the source.
- Per-copy attributes (targets for stagger, oscillator, noise): `shapePosition` (x, y),
  `shapeRotation`, `shapeScale` (x, y), `shapeOpacity`, `shapeTimeOffset`.

## Per-copy JavaScript (official docs; additional 2.8 observations)

JavaScript utilities require Pro. A Duplicator supplies `ctx.index` and `ctx.count` to
them. Utilities on source children receive the same context, so text or colour can vary
by copy. The official JavaScript Utility example connects a string expression to
`text.string` on the source Text Shape:

```js
var labels = cav.create('javaScript', 'copyLabels')
cav.attr(labels, { expression: '["2d", "3d", "Cel"][ctx.index % 3]' })
cav.connect(labels, 'id', sourceText, 'text.string')  // sourceText is inside the Duplicator
```

Match each output to its native destination type. Production feedback described vector
rotation. A new 2D Duplicator in Cavalry 2.8 declared `shapeRotation` as a scalar.
Vector output failed silently there; numeric output rotated the copies. Vector rotation
destinations require `{x, y, z}`. Colours require `{r, g, b, a}` unless a helper converts
hex strings. `cav.copyDriver` handles these types and rejects unknown paths. Profile a
busy section before increasing a JavaScript-driven copy count.

## Look At (official docs; orientation observed in 2.8)

Connect the target Shape's `id` to Look At's `target`. Then connect Look At's `id` to
the destination rotation attribute. Assigning a value to `target` does not make this
connection. Set Offset for the source's direction. A +x-facing arrow in the launch
video needed 90 degrees; check each source's actual direction.

## Stagger: copies animate one after another

```js
var src = cav.group('barSrc')
var bar = cav.rect('bar', 40, 160, { fill: '#4cc9f0', radius: 6, parent: src })
cav.tween(bar, 'position.y', 0, 18, -120, 0, 'outBack')
cav.tween(bar, 'opacity', 0, 10, 0, 100)
var bars = cav.duplicator('bars', src, { type: 'linear', count: 12, size: 900 })
cav.staggerTime(bars, 40)          // copy 0 starts at frame 0, the last copy 40 frames later
```

- `cav.staggerTime(target, spread, {reverse: true})` flips the order.
- Static per-copy values: `var st = cav.create('stagger', 'heights'); cav.attr(st, {minimum: 40,
  maximum: 300}); cav.connect(st, 'id', bars, 'shapeScale.y')`.
- Raw stagger assigns `minimum` to copy 0 and `maximum` to the last copy. It sorts these
  values, so swapping them does **not** reverse the order. Use `strength: -100`.

## Per-letter animation on one text layer

```js
var t = cav.text('title', 'CASCADE', 150, { style: 'Black' })
cav.textCascade(t, 20, { step: 3, dur: 16, dy: -120, scale: 0.4, rotate: -20 })
```

This uses a Sub-Mesh to split one text layer into letters. A stagger drives
`shapeTimeOffset`. The layer keeps the font's kerning. Use `cav.glyphs` when letters need
different motion that the Sub-Mesh cannot provide.

## Number counters

```js
var n = cav.text('stat', '0', 160, { style: 'Black' })
cav.counter(n, 30, 90, 0, 250000, { prefix: '$' })        // no thousands separator
cav.counter(n2, 30, 90, 0, 98.6, { decimals: 1, suffix: '%' })
```

`cav.counter` uses a String Generator. For custom formats (thousands separators), a JavaScript
utility works (Pro): `var js = cav.create('javaScript', 'fmt'); cav.attr(js, {expression:
'"$" + Math.round(n0).toLocaleString("en-US")'}); cav.key(js, 'array.0', [[30, 0, 'outCubic'],
[90, 25000]]); cav.connect(js, 'id', n, 'text')`. There is no `ctx.frame` in a JavaScript utility;
animate the input `n0` (attribute `array.0`) instead.

## Oscillator and noise (loops, idle motion, equalisers)

```js
cav.oscillate(ball, 'position.y', { min: -30, max: 30, freq: 0.5 })   // freq = cycles per second
cav.wiggle(card, 'rotation', { min: -4, max: 4, freq: 1.5 })          // smooth random
cav.oscillate(bars, 'shapeScale.y', { min: 0.3, max: 1.5, freq: 1, stagger: 1 })  // travelling wave
```

On a duplicator, oscillator `stagger` shifts phase by copy. A value of 1 makes a smooth
wave. The raw default of 20 creates abrupt changes between copies.

## Dense animation curves

Use a native oscillator or noise node for procedural motion. For a custom expression,
drive a JavaScript utility with a keyed input, `array.0`, exposed as `n0`. A two-key
clock can supply the time value. Put planned holds and easing in that clock. Inspect the
motion and section boundaries after connecting the utility.

One Cavalry 2.8 launch scene became smaller and opened faster after sampled keys were
replaced with drivers. A restart occurred between measurements, so the cause is unknown.
Drivers add evaluation work. Compare repeated open and render times before claiming a gain.

## Gradients

```js
cav.gradient(card, ['#4cc9f0', '#7209b7', '#f72585'], { rotation: 45 })
cav.gradient(ball, ['#ffffff', '#ff4d2e'], { type: 'radial' })
var g = cav.gradient(title, ['#ffd166', '#ef476f'])       // spans the whole word
cav.tween(g, 'generator.offset.x', 0, 60, -300, 300, 'inOutCubic')   // shimmer
```

Other gradient types include `sweepGradientShader` and `conicalGradientShader`. Set one
with `api.setGenerator(g, 'generator', type)`. `noiseShader` makes animated grey noise;
set `timeScale: 1` to see movement.

## Deformers and path motion

```js
var w = cav.create('wave', 'wave'); cav.attr(w, { amplitude: 25, numberOfWaves: 3 })
cav.tween(w, 'travel', 0, 120, 0, 100); cav.connect(w, 'id', band, 'deformers')
var bend = cav.create('bend', 'arc'); cav.attr(bend, { bendAngle: 40 }); cav.connect(bend, 'id', text, 'deformers')   // Pro
var nz = cav.create('noise', 'blob'); cav.connect(nz, 'id', circle, 'deformers')    // organic wobbling outline
var pf = cav.create('pathfinder', 'follow'); cav.connect(guidePath, 'id', pf, 'inputShape')
cav.attr(pf, { loop: false }); cav.tween(pf, 'travel', 0, 120, 0, 100, 'inOutCubic'); cav.connect(pf, 'id', dot, 'position')
```

A wave can create small end spikes on a plain rectangle because the shape has few points.
With `loop: true`, a pathfinder returns to the start at travel 100.

## Filters

`cav.filter(layer, type, attrs)` connects a filter to a layer or group.

| Type | Useful attributes | Notes |
|---|---|---|
| `blurFilter` | `amount` [x, y] | `cav.blur(layer, 10)`; animate `amount` for focus pulls |
| `sceneGroup::gaussianBlurFilter` | `amount` [x, y] | softer than blurFilter |
| `glowFilter` (Pro) | `intensity` (keep ≤ 1), `blur` [x, y], `glowColor` | on a group it also brightens the background |
| `dropShadowFilter` | `offset` {x, y}, `amount` {x, y}, `shadowColor` | |
| `chromaticAberrationFilter` | `strength` | the default 100 renders broken colour noise; use 15-30 |
| `rgbSplitFilter` | `redOffset`/`greenOffset`/`blueOffset` {x, y} | brightens (channels add) |
| `halftoneFilter` | `size`, `foreColor`, `backColor` | paints a white background by default |
| `pixelateFilter` | `size` | |
| `crtScanLines` (Pro) | `linesCount`, `lineOpacity` | |

### SkSL inputs and composite effects

The Inputs UI declares custom SkSL uniforms. Use their names in code without another
declaration. SkSL Filter still needs built-ins such as `uniform shader layer;` to sample
the Shape with `layer.eval(p)` (official SkSL Filter docs). Duplicate custom declarations
caused silent failure during Cavalry 2.8 launch-video work.

In that scene, a group filter ran on each child separately. For a composite effect,
filter a pre-comp reference and inspect overlapping shapes. Group opacity depends on mode:
**Individual Shapes** fades children separately. **Artboard** fades one combined image
(official Group docs). Opacity mode does not establish filter behavior.

## Variable-font axes (observed in 2.8)

Inspect each variable font before connecting a driver. In the Cavalry 2.8 scene,
`fontAxes.N` indices followed the font's `fvar` order. Indices can differ across fonts.
The family name alone does not establish axis tags.

## Motion blur

Call `cav.motionBlur(16)` after creating the layers. The composition setting alone leaves
the render sharp. The helper also sets each layer's `motionBlur` to 1.

## 2.5D and camera (Pro)

```js
cav.attr(card, { is3d: true, 'position.z': 400 })            // +z comes toward the camera
var cam = cav.create('planarCamera', 'cam')
cav.tween(cam, 'position.z', 0, 120, 1600, 700, 'inOutCubic') // push in
cav.connect(cam, 'id', api.getActiveComp(), 'cameraTransform')
cav.tween(text, 'rotation.y', 0, 60, -90, 0, 'outBack')        // card flip (needs is3d)
```

## Pre-comps

```js
var main = api.getActiveComp()
var sub = api.createComp('badge')
api.set(sub, { resolution: { x: 300, y: 300 }, fps: 30, startFrame: 0, endFrame: 59,
  backgroundColor: { r: 0, g: 0, b: 0, a: 0 } })   // alpha 0, or it draws an opaque box
api.setActiveComp(sub)
// build and animate the badge with cav.* here
api.setActiveComp(main)
var ref = api.createCompReference(sub)
cav.set(ref, { x: -380 })
api.offsetLayerTime(ref, 20)        // starts 20 frames later; after its end it holds the last frame
```

Return to the original composition with `api.setActiveComp(main)`.

## Masks and mattes

- Clip: `cav.mask(shape, target)` (target can be a group; the mask shape hides itself).
- Track matte (the fill shows only through the text): `cav.connect(text, 'id', fillRect,
  'trackMattes'); cav.attr(text, { hidden: true })`.

## Physics and particles (Pro)

Forge Dynamics uses Box2D for rigid bodies. Forge Dynamics and particles **simulate only
when frames render in order**. Preview them with `cav frames 0-599 --keep-every 10` or
a short sheet range with step 1. `frames` evaluates intervening frames from composition
start and keeps only selected PNGs.

```js
var f = cav.create('forgeDynamicsShape', 'physics')
cav.connect(box, 'id', f, 'shapes')                         // repeat for each body
cav.attr(f, { gravity: { x: 0, y: -60 }, groundBounce: 0.4 })   // default gravity -10 falls very slowly

var ps = cav.create('particleShape', 'sparks'), em = cav.create('particleEmitter', 'emitter')
cav.connect(em, 'id', ps, 'emitters')
cav.connect(cav.comp().id, 'time', ps, 'time')
cav.connect(cav.comp().id, 'time', em, 'time')
cav.attr(em, { directionType: 1, initialSpeed: 25, emitterRate: 2000, duration: 0.1, size: { x: 20, y: 20 } })
cav.attr(ps, { shapeStyle: 0, colorMode: 0, particleRadius: 6, particleColor: '#ffd166', lifespan: 2 })
```

Set `shapeStyle: 0` and `colorMode: 0` for these particles. Other values can produce
an empty frame or purple dots. Use `cav.burst` for a simple burst.

**Spring behaviour in scripts:** A Spring behaviour connected to keyed attributes had no
visible effect. The possible need for "Enable Experimental Features" in Preferences
remains unconfirmed. Use `spring` or `outBack` easing for this case.

## Export

For Lottie, add a render queue item and call
`api.setGenerator(item, 'generator', 'renderLottie')`. Set `filePath`, `fileName` and
`frameRange`, then call `api.render(item)`. Always set `filePath`; a new item can point
to another project's folder. Other generators include `renderPNG`, `renderGIF`,
`renderWebM`, `renderProRes`, `renderAPNG` and `renderSVG`.


The guidance uses Cavalry's [JavaScript Layers](https://cavalry.studio/docs/nodes/general/javascript-layers/)
and [JavaScript Utility](https://cavalry.studio/docs/nodes/utilities/javascript-utility/).
It also uses [Look At](https://cavalry.studio/docs/nodes/behaviours/look-at/),
[SkSL Filter](https://cavalry.studio/docs/nodes/effects/filters/sksl-filter/) and
[Group](https://cavalry.studio/docs/nodes/shapes/group/).


## Production helpers

```js
var ref = cav.precomp('section', function () {
    cav.circle('dot', 20)
}, {width: 1920, height: 1080})
cav.sksl(ref, 'uniform shader layer; uniform float amount; half4 main(float2 p) { return layer.eval(p)*amount; }', {
    inputs: {amount: 0.5}
})
cav.axes(title, {wght: 900, ROND: 50})
cav.driver(title, 'position.x', 'Math.sin(n0*6.283)*40', {start: 0, end: 599})
cav.copyDriver(dup, 'shapeRotation', 'ctx.index*12')
cav.copyDriver(source, 'fill', 'ctx.index%2 ? "#ff8000" : "#0080ff"')
cav.lookAt(dup, target, {attr: 'shapeRotation', offset: 90})
cav.above(title, ref)
```

`precomp` restores the original composition and playhead even if its callback fails.
Its reference can receive a composite filter. `sksl` removes declarations only for inputs
it adds. Required built-ins such as `shader layer` remain. Scalar inputs use numbers;
other inputs use `{type: 'double2', value: [1, 2]}` or their native types.

`axes` reads installed font files. It checks native axis order and names against OpenType
`fvar` and `name` metadata. It warns when clamping values and rejects unknown or
ambiguous tags before changing the font. Add custom folders through `CAV_FONT_DIR`.
Discovery stops at 10000 entries. Incomplete scans provide no axis map.

`driver` supplies `n0` with two keys. This avoids expression keys every few frames.
Colour drivers attach to the source shape's `fill` (native `material.materialColor`),
so Duplicator context reaches each source child. The wrapper checks the native destination
type. Cavalry 2.8 tests found scalar `shapeRotation` in a 2D Duplicator. The wrapper also
supports vector destinations and rejects unknown paths.
`copyDriver` evaluates JavaScript for each native copy. It wraps scalar rotation as
`{x:0,y:0,z:value}` for vector destinations. It preserves scalar rotation destinations
or reads z from an object. It converts hex colours to native RGBA objects. For other
attributes, return the destination's type. `lookAt` connects the target ID;
`offset:90` fits arrows drawn toward +x. `above` and `below` set stack order explicitly.
`order` remains an alias for `below`.
