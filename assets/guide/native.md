# Native Cavalry features

Use these instead of building many layers by hand: one duplicator with 50 copies is faster to
script and to render than 50 layers. Every recipe here was built and rendered in Cavalry 2.7.2.
"Pro" marks layer types that need a Pro licence (`cav.pro(type)` checks); a Starter licence
cannot save or render them.

## Rules for every node

- Behaviours and utilities drive an attribute with `api.connect(node, 'id', layer, attr)`. The
  connection **replaces** the attribute's value. A shape at `y = 200` with an oscillator from -10
  to 10 on `position.y` moves around y = 0. Put the shape in a positioned group, or put the base
  value into the node's minimum and maximum.
- You can connect into one channel: `position.y`, `scale.x`, `rotation.z`, `shapePosition.y`.
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

- The source is hidden; the duplicator draws the copies. Move, scale and fade the duplicator itself
  like any layer (`cav.pop(grid, 30)`).
- The duplicator **ignores the source's own position, scale and rotation keys**. Opacity, size and
  colour keys do reach the copies. To animate each copy, animate a shape inside a group and pass
  the group as the source.
- Per-copy attributes (targets for stagger, oscillator, noise): `shapePosition` (x, y),
  `shapeRotation`, `shapeScale` (x, y), `shapeOpacity`, `shapeTimeOffset`.

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
- Raw stagger: `minimum` goes to copy 0, `maximum` to the last copy. It sorts the two values, so
  swapping them does **not** reverse the order: use `strength: -100` instead.

## Per-letter animation on one text layer

```js
var t = cav.text('title', 'CASCADE', 150, { style: 'Black' })
cav.textCascade(t, 20, { step: 3, dur: 16, dy: -120, scale: 0.4, rotate: -20 })
```

This uses a Sub-Mesh (a deformer that splits the text into letters) plus a stagger on its
`shapeTimeOffset`. It keeps the text as one layer, with real kerning. Use `cav.glyphs` only when
each letter needs its own unique motion.

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

On a duplicator, the oscillator's `stagger` shifts the phase per copy. 1 gives a smooth wave; the
raw default of 20 looks like a zig-zag.

## Gradients

```js
cav.gradient(card, ['#4cc9f0', '#7209b7', '#f72585'], { rotation: 45 })
cav.gradient(ball, ['#ffffff', '#ff4d2e'], { type: 'radial' })
var g = cav.gradient(title, ['#ffd166', '#ef476f'])       // spans the whole word
cav.tween(g, 'generator.offset.x', 0, 60, -300, 300, 'inOutCubic')   // shimmer
```

Other gradient types: `sweepGradientShader`, `conicalGradientShader` (`api.setGenerator(g,
'generator', type)`). A `noiseShader` gives animated grey noise (`timeScale: 1` to see it move).

## Deformers and path motion

```js
var w = cav.create('wave', 'wave'); cav.attr(w, { amplitude: 25, numberOfWaves: 3 })
cav.tween(w, 'travel', 0, 120, 0, 100); cav.connect(w, 'id', band, 'deformers')
var bend = cav.create('bend', 'arc'); cav.attr(bend, { bendAngle: 40 }); cav.connect(bend, 'id', text, 'deformers')   // Pro
var nz = cav.create('noise', 'blob'); cav.connect(nz, 'id', circle, 'deformers')    // organic wobbling outline
var pf = cav.create('pathfinder', 'follow'); cav.connect(guidePath, 'id', pf, 'inputShape')
cav.attr(pf, { loop: false }); cav.tween(pf, 'travel', 0, 120, 0, 100, 'inOutCubic'); cav.connect(pf, 'id', dot, 'position')
```

A wave on a plain rectangle shows small spikes at the ends: it has few points. With `loop: true`,
a pathfinder at travel 100 jumps back to the start.

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

## Motion blur

`cav.motionBlur(16)` after all layers exist. The comp switch alone renders sharp: every layer also
needs its own `motionBlur` set to 1, which the helper does.

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

Always switch back with `api.setActiveComp(main)`.

## Masks and mattes

- Clip: `cav.mask(shape, target)` (target can be a group; the mask shape hides itself).
- Track matte (the fill shows only through the text): `cav.connect(text, 'id', fillRect,
  'trackMattes'); cav.attr(text, { hidden: true })`.

## Physics and particles (Pro)

Forge Dynamics (rigid bodies, built on Box2D) and particles work, **but they only simulate when
frames render in order**. Preview them with a step-1 range: `cav sheet 0-59:1`.

```js
var f = cav.create('forgeDynamicsShape', 'physics')
cav.connect(box, 'id', f, 'shapes')                         // repeat for each body
cav.attr(f, { gravity: { x: 0, y: -60 }, groundBounce: 0.4 })   // default gravity -10 falls very slowly

var ps = cav.create('particleShape', 'sparks'), em = cav.create('particleEmitter', 'emitter')
cav.connect(em, 'id', ps, 'emitters')
cav.attr(em, { directionType: 1, initialSpeed: 25, emitterRate: 2000, duration: 0.1, size: { x: 20, y: 20 } })
cav.attr(ps, { shapeStyle: 0, colorMode: 0, particleRadius: 6, particleColor: '#ffd166', lifespan: 2 })
```

Particles need `shapeStyle: 0` and `colorMode: 0`, or you see nothing or purple dots. For a
simple, reliable burst, `cav.burst` is often enough.

**Not working in scripts**: the Spring behaviour connected to keyed attributes had no visible
effect (it may need "Enable Experimental Features" in Preferences; not confirmed). Use the
`spring` or `outBack` easing instead.

## Export

Lottie: add a render queue item, `api.setGenerator(item, 'generator', 'renderLottie')`, set
`filePath`, `fileName` and `frameRange`, then `api.render(item)`. Always set `filePath`: a new item
may point at another project's folder. Other generators: `renderPNG`, `renderGIF`, `renderWebM`,
`renderProRes`, `renderAPNG`, `renderSVG`.
