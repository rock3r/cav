# Motion recipes

Each recipe is a working pattern built on the `cav` helpers. Copy it into a script file, change
the numbers, run it with `cav run`, and check it with `cav sheet`. Frames assume 60 fps.

## Contents
1. Design basics (colour, type, layout, timing)
2. Scene structure: sections, rig, cuts
3. Entrances, exits, anticipation and overshoot
4. Kinetic type
5. Impacts and silence before the hit
6. Draw-ons with trim paths
7. Zoom-through transition
8. Other transitions
9. Lower third
10. Bar chart from data
11. Logo sting
12. UI rebuilt as components
13. The review loop

## 1. Design basics

- **Palette**: one dark background, one light text colour, one or two accents. Examples:
  `#0b0d12 / #f5f7fa / #ff4d2e`, `#101820 / #fefae0 / #2ec4b6 / #ffbf69`, `#1b1033 / #ffffff / #7b5cff / #36f1cd`.
- **Type**: one bold display font for headlines (`Inter` Black/Bold), one mono or regular font for
  small labels. Headlines 100-180 px at 1080p; labels 28-40 px. Letter spacing 0 to -2 for big
  bold words; +4 to +10 for small upper-case kickers.
- **Layout**: keep text inside the middle 90 % (title safe). Align things to a few shared x/y lines.
  One focal point at a time.
- **Timing at 60 fps**: small UI moves 10-18 frames; big moves 20-40 frames; stagger steps 2-6
  frames; hold readable text at least 60 frames (about 1 s per 3 words).
- **Easing**: entrances `outExpo`, `outQuint` or `outBack`; exits `inBack` or `inCubic`;
  moves between two rests `inOutCubic`; springy landings `spring`.

## 2. Scene structure

Build each section in its own group with in/out frames. Put everything under one top-level `rig`
group so camera shake and zooms move the whole picture.

```js
var c = cav.comp(), b = cav.beats(120)
var rig = cav.group('rig')
var s1 = cav.group('s1_intro', { parent: rig, in: 0, out: b.bar(2) })       // bars 0-1
var s2 = cav.group('s2_features', { parent: rig, in: b.bar(2), out: b.bar(4) })
cav.plane('bg', '#0b0d12')              // background outside the rig so shake does not reveal edges
cav.order(cav.find('bg'), rig)          // bg below the rig
```

Look up a group in a later script with `cav.find('s1_intro')`.

## 3. Entrances, exits, anticipation, overshoot

```js
var card = cav.rect('card', 600, 340, { radius: 24, fill: '#1c2230', parent: s1 })
cav.pop(card, 30, { dur: 16 })                        // scale 0 -> 1 with outBack overshoot
cav.slideIn(title, 36, { dy: -80, dur: 22 })          // outExpo from 80 px above, fades in
cav.slideOut(title, 150, { dy: 60, dur: 14 })         // inBack: winds up, then leaves
// Hand-made anticipation: squash before a jump, then overshoot on landing.
cav.key(ball, 'scale', [[40, [1, 1], 'outQuad'], [48, [1.2, 0.8], 'inOutCubic'], [56, [0.9, 1.15], 'spring'], [80, [1, 1]]])
```

## 4. Kinetic type

Letters one by one (one layer per glyph, measured with real kerning):

```js
var word = cav.glyphs('headline', 'MOTION', 160, { font: 'Inter', style: 'Black', color: '#f5f7fa', parent: s1 })
cav.cascade(word, 20, { step: 3, dy: -120, dur: 22, ease: 'outBack', rotate: -12 })
```

Word swap on the beat (slot machine): stack words in a masked group, move the column.

```js
var words = ['FAST', 'CLEAR', 'YOURS'], col = cav.group('col', { parent: s2 })
words.forEach(function (w, i) { cav.text('w' + i, w, 140, { parent: col, y: -i * 170 }) })
var win = cav.rect('window', 900, 170, { parent: s2 })
cav.mask(win, col)
words.forEach(function (w, i) { if (i) cav.tween(col, 'position.y', b.beat(4 * i) - 10, b.beat(4 * i), (i - 1) * 170, i * 170, 'outBack') })
```

Typewriter: `cav.typeOn(cav.glyphs('code', 'npm run dev', 48, { font: 'JetBrains Mono', align: 'left', x: -300 }), 60, { rate: 3 })`.

Counters: text keys do not work. Stack digit layers in a masked column and move it (like the word swap).

## 5. Impacts and silence before the hit

```js
var hit = b.bar(4)
// Silence: in the half beat before the hit, everything collapses to a glowing dot.
cav.tween(s1, 'scale', hit - 15, hit - 2, 1, 0.02, 'inExpo')
var dot = cav.circle('dot', 10, { fill: '#ffffff', in: hit - 15, out: hit + 2 })
cav.pop(dot, hit - 15, { dur: 10 })
// The hit: all on the same frame.
cav.flash(hit, { dur: 10 })
cav.ring(hit, 0, 0, { r1: 700, dur: 30, width: 16, color: '#ff4d2e' })
cav.burst(hit, 0, 0, { count: 16, dist: 500, size: 12, color: '#ffffff' })
cav.shake(rig, hit, 26, 22)
```

## 6. Draw-ons with trim paths

```js
var box = cav.rect('frame', 700, 400, { noFill: true, stroke: { color: '#2ec4b6', width: 4 }, radius: 16 })
cav.drawOn(box, 30, 40)                                // trimEnd 0 -> 100
var arc = cav.path('arc', [[-400, -200], [0, 150], [400, -200]], { smooth: true, stroke: { color: '#ffffff', width: 6, cap: 'round' } })
cav.drawOn(arc, 50, 45, 'inOutCubic')
```

## 7. Zoom-through transition

Scale the rig into a point (for example a button) so it fills the screen, then start the next
section from oversize and settle.

```js
var target = [320, -180]                                       // the button's position in rig space
cav.zoomThrough(rig, b.bar(4) - 30, b.bar(4), target, { scale: 14, ease: 'inExpo' })
cav.tween(s2, 'scale', b.bar(4), b.bar(4) + 30, 3, 1, 'outExpo')  // next section settles from big
cav.flash(b.bar(4), { dur: 6, peak: 70 })
```

## 8. Other transitions

- **Wipe**: a full-frame rectangle `cav.wipeIn(panel, f, {from: 'left'})`, then cut the old section's `out` to the frame where the panel covers the screen.
- **Mask reveal**: animate a mask rect's width with `cav.wipeIn` while it masks the new section.
- **Push**: tween section A to `x: -c.width` and section B from `x: c.width` to 0 on the same frames with `inOutCubic`.
- **Flash cut**: `cav.flash(f, {dur: 8})` and switch sections on `f`.

## 9. Lower third

```js
var lt = cav.group('lower_third', { x: c.left + 120, y: c.bottom + 170, in: 30, out: 300 })
var barA = cav.rect('lt_bar', 12, 90, { parent: lt, fill: '#ff4d2e', x: 0 })
var name = cav.text('lt_name', 'Ada Lovelace', 56, { parent: lt, align: 'left', x: 36, y: 18 })
var role = cav.text('lt_role', 'FIRST PROGRAMMER', 26, { parent: lt, align: 'left', x: 38, y: -30, style: 'Medium', spacing: 6, color: '#aab3c5' })
cav.wipeIn(barA, 30, { from: 'bottom', dur: 14 })
cav.slideIn(name, 36, { dx: -40, dy: 0, dur: 20 })
cav.slideIn(role, 42, { dx: -40, dy: 0, dur: 20 })
cav.slideOut(name, 270, { dx: -40, dy: 0 }); cav.slideOut(role, 266, { dx: -40, dy: 0 }); cav.fadeOut(barA, 280, 10)
```

## 10. Bar chart from data

Read CSV in the script (absolute path), then build bars that grow from a baseline.

```js
var rows = api.readFromFile('/abs/path/data.csv').trim().split('\n').slice(1).map(function (l) { var p = l.split(','); return { label: p[0], value: +p[1] } })
var max = Math.max.apply(null, rows.map(function (r) { return r.value }))
var W = 1200, H = 600, gap = 24, bw = (W - gap * (rows.length - 1)) / rows.length, base = -300
rows.forEach(function (r, i) {
  var x = -W / 2 + bw / 2 + i * (bw + gap), h = (r.value / max) * H
  var bar = cav.rect('bar_' + i, bw, h, { fill: '#2ec4b6', radius: 6 })
  cav.set(bar, { pivot: [0, -h / 2], x: x, y: base })       // pivot at the bottom edge, placed on the baseline
  cav.tween(bar, 'scale.y', 20 + i * 6, 50 + i * 6, 0, 1, 'outBack')
  cav.text('label_' + i, r.label, 28, { x: x, y: base - 36, style: 'Medium', color: '#aab3c5' })
  var v = cav.text('value_' + i, String(r.value), 34, { x: x, y: base + h + 30 })
  cav.fadeIn(v, 44 + i * 6, 12)
})
```

## 11. Logo sting

Pattern: dark frame, a small spark, build-up, hit on the downbeat, logo lands with overshoot, hold, glint.

```js
var logo = cav.image('/abs/path/logo.png', 'logo', { scale: 0.5 })   // or build the mark from shapes
cav.set(logo, { opacity: 0 })
cav.ring(58, 0, 0, { r1: 500, dur: 30 })
cav.pop(logo, 60, { dur: 20, ease: 'spring' }); cav.fadeIn(logo, 60, 6)
cav.flash(60, { dur: 8, peak: 60 })
var tag = cav.text('tagline', 'build something bright', 40, { y: -220, style: 'Medium', color: '#aab3c5' })
cav.slideIn(tag, 90, { dy: -30, dur: 24 })
```

## 12. UI rebuilt as components

Rebuild UI from shapes and text (not a flat screenshot), so every part can move. Measure the
screenshot, then rebuild at the same proportions: window rect, title bar, sidebar, buttons, fields.
Animate them in with small staggered `slideIn`s (dy 20-30, 3-4 frame steps), then show one
interaction: a cursor (small path arrow) moves with `inOutCubic`, the button `punch`es, a new
panel `pop`s. Keep a reference screenshot beside it in the review sheet if you have one.

## 13. The review loop

1. After each script: `cav sheet 12` (or frames around the part you changed) and look at it.
2. Something looks wrong -> check numbers: `cav layer <id>` shows the bounding box, parent and keys.
3. Before rendering: `cav sheet 0-<end>:<step> --bpm <bpm>` over the whole piece.
4. Render: `cav render -o renders/final.mp4 --audio <track>`; say what you could not check.
