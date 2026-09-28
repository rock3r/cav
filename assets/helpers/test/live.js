// Live tests: run inside Cavalry with `cav run assets/helpers/test/live.js` in a new scene.
// Returns {pass, fail, failures}. Deletes nothing outside the active comp; call after
// `cav scene new --force` in a throwaway scene.
var failures = [], pass = 0
function check(name, cond, detail) {
	if (cond) pass++
	else failures.push(name + (detail !== undefined ? ': ' + JSON.stringify(detail) : ''))
}
function near(a, b, tol) { return Math.abs(a - b) <= (tol === undefined ? 0.5 : tol) }
function at(f, l, a) { api.setFrame(f); return api.get(l, a) }

cav.clear()
var c = cav.comp()
check('comp fps', c.fps > 0, c)

// Every easing expression must be accepted by Cavalry and match the JS curve at x = 0.5.
var names = Object.keys(cav.E)
names.forEach(function (n, i) {
	var r = cav.rect('ease_' + n, 10, 10)
	// position.x, not opacity: opacity is clamped to 0..100, which hides overshoot.
	cav.key(r, 'position.x', [[0, 0, n], [20, 100]])
	var fn = new Function('x', 'var pow=Math.pow,exp=Math.exp,sin=Math.sin,cos=Math.cos; return ' + cav.E[n])
	var want = fn(0.5) * 100
	var got = at(10, r, 'position.x')
	check('ease ' + n, near(got, want, 1.5), { got: got, want: want })
})
// Opacity is clamped to 0..100: overshoot easings do not overshoot it.
var oc = cav.rect('clamp', 10, 10)
cav.key(oc, 'opacity', [[0, 0, 'outBackBig'], [20, 100]])
check('opacity clamps', at(10, oc, 'opacity') <= 100, at(10, oc, 'opacity'))
cav.clear()

// Magic Easing names pass through.
var m = cav.rect('magic', 10, 10)
cav.key(m, 'opacity', [[0, 0, 'SlowOut'], [20, 100]])
check('magic ease runs', at(10, m, 'opacity') > 50, at(10, m, 'opacity'))

// Parent-safe creation: child at local (100, 50) inside a scaled, moved group.
var g = cav.group('g', { x: 300, y: -100, scale: 2 })
var r = cav.rect('child', 40, 40, { parent: g, x: 100, y: 50, fill: '#ff0000' })
var bb = api.getBoundingBox(r, true)
check('parented child world centre', near(bb.centre.x, 500, 1) && near(bb.centre.y, 0, 1), bb.centre)
check('parented child world size', near(bb.width, 80, 1), bb.width)

// Colour keys per channel.
var col = cav.rect('col', 10, 10)
cav.key(col, 'fill', [[0, '#000000'], [10, '#ff8040']])
api.setFrame(10)
var cc = api.get(col, 'material.materialColor')
check('colour keys', near(cc.r, 255, 1) && near(cc.g, 128, 1) && near(cc.b, 64, 1), cc)

// Tween holds between segments.
var h = cav.rect('hold', 10, 10)
cav.tween(h, 'position.x', 0, 10, 0, 100)
cav.tween(h, 'position.x', 30, 40, 100, 0)
check('tween hold', near(at(20, h, 'position.x'), 100), at(20, h, 'position.x'))

// Text and font fallback.
var t = cav.text('title', 'Hello', 80, { font: 'Definitely Not A Font', color: '#00ff00' })
check('text exists', api.getLayerType(t) === 'textShape')
check('text font fallback', api.get(t, 'font').font !== 'Definitely Not A Font', api.get(t, 'font'))
check('text is centred', near(api.getBoundingBox(t, true).centre.x, 0, 3), api.getBoundingBox(t, true).centre)

// Glyphs: one layer per non-space char, centred.
var gl = cav.glyphs('word', 'AB C', 100)
check('glyph count', gl.chars.length === 3, gl.chars.length)
var gb = api.getBoundingBox(gl.group, true)
check('glyphs centred', near(gb.centre.x, 0, 12), gb)
cav.cascade(gl, 0, { step: 3 })
check('cascade keys', api.getKeyframeTimes(gl.chars[2], 'position.y').length === 2)

// Path, line and draw-on.
var ln = cav.line('ln', [-200, 0], [200, 0], { stroke: { color: '#ffffff', width: 6 } })
cav.drawOn(ln, 0, 20, 'linear')
check('drawOn mid', near(at(10, ln, 'stroke.trimEnd'), 50, 1), at(10, ln, 'stroke.trimEnd'))
var pa = cav.path('tri', [[0, 100], [100, -50], [-100, -50]], { closed: true, fill: '#3366ff' })
check('closed path bbox', near(api.getBoundingBox(pa, true).height, 150, 2), api.getBoundingBox(pa, true))

// Recipes.
var p = cav.circle('pop', 50, { x: -300 })
cav.pop(p, 10, { dur: 10 })
check('pop starts at 0', near(at(10, p, 'scale').x, 0, 0.01))
check('pop overshoots', at(15, p, 'scale').x > 0.9, at(15, p, 'scale'))
var s = cav.rect('slide', 50, 50, { x: 100, y: 100 })
cav.slideIn(s, 0, { dy: -60, dur: 10 })
check('slideIn start', near(at(0, s, 'position').y, 40), at(0, s, 'position'))
check('slideIn end', near(at(10, s, 'position').y, 100), at(10, s, 'position'))
// Chained recipes read values at their own start frame, not at the playhead.
var ch = cav.rect('chain', 40, 40, { x: -400, y: 200 })
api.setFrame(0)
cav.slideIn(ch, 0, { dy: -100, dur: 10 })
cav.slideOut(ch, 40, { dy: 100, dur: 10 })
check('chain holds rest position', near(at(30, ch, 'position').y, 200, 0.5), at(30, ch, 'position'))
check('chain exits from rest', near(at(40, ch, 'position').y, 200, 0.5) && near(at(50, ch, 'position').y, 300, 0.5), [at(40, ch, 'position'), at(50, ch, 'position')])
// valueAt reads held key values without moving the playhead, and agrees with the slow path.
var va = cav.rect('valueAt', 20, 20, { x: 10, y: 20 })
cav.key(va, 'position.x', [[10, 100], [20, 200, 'inOut']])
cav.key(va, 'opacity', [[10, 30], [20, 80]])
api.setFrame(3)
var before = cav.valueAt(va, 'position', 5), after = cav.valueAt(va, 'position', 40), mid = cav.valueAt(va, 'position', 15)
check('valueAt before first key', near(before.x, 100) && near(before.y, 20), before)
check('valueAt after last key', near(after.x, 200) && near(after.y, 20), after)
check('valueAt between keys', near(mid.x, at(15, va, 'position.x')), mid)
check('valueAt scalar held', near(cav.valueAt(va, 'opacity', 30), 80), cav.valueAt(va, 'opacity', 30))
api.setFrame(3)
cav.valueAt(va, 'position', 40)
check('valueAt keeps playhead', api.getFrame() === 3, api.getFrame())
var w = cav.rect('wipe', 200, 40, { x: 50, y: -200 })
cav.wipeIn(w, 0, { dur: 10, from: 'left' })
api.setFrame(10)
var wb = api.getBoundingBox(w, true)
check('wipeIn keeps place', near(wb.centre.x, 50, 1) && near(wb.width, 200, 1), wb)
api.setFrame(5)
check('wipeIn grows from left edge', near(api.getBoundingBox(w, true).left, -50, 1), api.getBoundingBox(w, true))
var fl = cav.flash(20)
check('flash peak', near(at(20, fl, 'opacity'), 60, 1), at(20, fl, 'opacity'))
var ring = cav.ring(20, 0, 0, { r1: 300 })
check('ring grows', at(44, ring, 'generator.radius').x > 250, at(44, ring, 'generator.radius'))
var burst = cav.burst(20, 0, 0, { count: 6 })
check('burst children', api.getChildren(burst).length === 6)
var sh = cav.rect('shake', 10, 10)
cav.shake(sh, 10, 30, 10)
check('shake returns home', near(at(20, sh, 'position').x, 0, 0.01))

// Zoom-through: a child at (200, 100) in the rig ends at the comp centre.
var rig = cav.group('rig')
var tgt = cav.rect('target', 20, 20, { parent: rig, x: 200, y: 100 })
cav.zoomThrough(rig, 0, 30, [200, 100], { scale: 8, ease: 'inOutCubic' })
api.setFrame(30)
var tb = api.getBoundingBox(tgt, true)
check('zoomThrough centres target', near(tb.centre.x, 0, 2) && near(tb.centre.y, 0, 2), tb.centre)
api.setFrame(0)
check('zoomThrough starts in place', near(api.getBoundingBox(tgt, true).centre.x, 200, 1))

// Filters, masks, create by type.
var bl = cav.blur(t, 10)
check('blur connected', api.getLayerType(bl) === 'blurFilter', api.getLayerType(bl))
var mk = cav.rect('maskShape', 100, 100)
cav.mask(mk, t)
check('mask connected', true)
var dup = cav.create('duplicator', 'dup')
check('create by type', api.getLayerType(dup) === 'duplicator')
var bad = null
try { cav.create('noSuchLayerType', 'x') } catch (e) { bad = e.message }
check('bad type error', /cav types/.test(String(bad)), bad)

// Beat grid.
var bg = cav.beats(120)
check('beats per frame', near(bg.frames, (60 / 120) * c.fps, 0.01))

api.setFrame(0)
return { pass: pass, fail: failures.length, failures: failures }
