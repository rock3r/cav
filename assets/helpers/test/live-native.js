// Live tests for the native-feature helpers. Run in a throwaway scene:
//   cav scene new --force --width 1280 --height 720 --fps 30 --seconds 4 && cav run assets/helpers/test/live-native.js
var failures = [], pass = 0
function check(name, cond, detail) {
	if (cond) pass++
	else failures.push(name + (detail !== undefined ? ': ' + JSON.stringify(detail) : ''))
}
function near(a, b, tol) { return Math.abs(a - b) <= (tol === undefined ? 0.5 : tol) }
function at(f, l, a) { api.setFrame(f); return api.get(l, a) }
cav.clear()

// Rotation keys land on rotation.z.
var r = cav.rect('spin', 40, 40, { x: -500, y: 300 })
cav.tween(r, 'rotation', 0, 10, 0, 90)
check('rotation keys', near(at(10, r, 'rotation').z, 90, 0.5), at(10, r, 'rotation'))

// Star geometry.
var s = cav.star('star', 6, 80, { x: 500, y: 300 })
check('star radius', near(api.getBoundingBox(s, true).height, 160, 12), api.getBoundingBox(s, true))

// Duplicator grid: 4 x 3 copies of a 20 px square over 300 x 200.
var cell = cav.rect('cell', 20, 20, { fill: '#4cc9f0' })
var grid = cav.duplicator('grid', cell, { type: 'grid', count: [4, 3], size: [300, 200] })
var gb = api.getBoundingBox(grid, true)
check('grid extent', near(gb.width, 320, 4) && near(gb.height, 220, 4), gb)
check('source hidden', api.get(cell, 'hidden') === true)

// Circle distribution.
var dot = cav.circle('dot', 8)
var ring = cav.duplicator('ring', dot, { type: 'circle', count: 12, radius: 150 })
check('ring extent', near(api.getBoundingBox(ring, true).width, 316, 6), api.getBoundingBox(ring, true))

// Stagger time cascade: copy 0 first.
var srcG = cav.group('barSrc')
var bar = cav.rect('bar', 30, 100, { parent: srcG })
cav.tween(bar, 'opacity', 0, 5, 0, 100)
var bars = cav.duplicator('bars', srcG, { type: 'linear', count: 5, size: 400 })
var st = cav.staggerTime(bars, 40)
check('stagger values', api.get(st, 'minimum') === 0 && api.get(st, 'maximum') === 40 && api.get(st, 'strength') === -100)

// Text cascade on one layer.
var t = cav.text('title', 'HELLO', 120, { y: -250 })
var sm = cav.textCascade(t, 10, { step: 4, dur: 12 })
check('text cascade sub-mesh', api.getLayerType(sm) === 'subMesh')
api.setFrame(60)
var tb = api.getBoundingBox(t, true)
check('text settles centred', near(tb.centre.x, 0, 10) && near(tb.centre.y, -250, 10), tb)

// Counter.
var num = cav.text('num', '0', 80, { x: 400, y: -250 })
cav.counter(num, 0, 30, 0, 100, { suffix: '%' })
api.setFrame(30)
var numWidth = api.getBoundingBox(num, true).width
api.setFrame(0)
check('counter changes the text width', numWidth > api.getBoundingBox(num, true).width + 20, numWidth)

// Oscillator and wiggle.
var ob = cav.circle('osc', 20)
cav.oscillate(ob, 'position.x', { min: -100, max: 100, freq: 1 })
var xs = [0, 7, 15, 22].map(function (f) { return at(f, ob, 'position').x })
check('oscillator moves', Math.max.apply(null, xs) - Math.min.apply(null, xs) > 100, xs)
var wb = cav.rect('wig', 40, 40)
cav.wiggle(wb, 'rotation', { min: -30, max: 30, freq: 2 })
var rs = [0, 10, 20, 30].map(function (f) { return at(f, wb, 'rotation').z })
check('wiggle moves', Math.max.apply(null, rs) - Math.min.apply(null, rs) > 5, rs)

// Gradient and motion blur.
var gcard = cav.rect('gcard', 300, 150, { x: -400, y: -200 })
var g = cav.gradient(gcard, ['#ff0000', '#0000ff'], { rotation: 0 })
check('gradient connected', api.getLayerType(g) === 'gradientShader')
var gt = cav.text('gtext', 'SHINE', 90, { x: 350, y: 250 })
var g2 = cav.gradient(gt, ['#ffd166', '#ef476f'])
check('text gradient screen space', api.get(g2, 'screenSpace') === true)
var n = cav.motionBlur(8)
check('motion blur', api.get(api.getActiveComp(), 'motionBlur') === true && n > 5, n)

// Multi-file run semantics are tested from the CLI side.
api.setFrame(0)
return { pass: pass, fail: failures.length, failures: failures }
