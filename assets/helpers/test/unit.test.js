// Unit tests for cav-helpers.js against a mock Cavalry API. Run: node --test assets/helpers/test
const test = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')

function mockEnv() {
	const calls = []
	const layers = {}
	let n = 0
	const make = (type, name) => {
		const id = type + '#' + ++n
		layers[id] = { name, type, attrs: { position: { x: 0, y: 0 }, scale: { x: 1, y: 1 }, opacity: 100 } }
		return id
	}
	const api = {
		select: (s) => calls.push(['select', s]),
		create: (t, name) => make(t, name),
		primitive: (t, name) => make('basicShape', name),
		layerExists: (id) => id in layers,
		keyframe: (l, f, d) => calls.push(['keyframe', l, f, d]),
		magicEasing: (l, a, f, name, expr) => calls.push(['ease', l, a, f, name, expr]),
		set: (l, d) => {
			calls.push(['set', l, d])
			Object.assign(layers[l].attrs, d)
		},
		get: (l, a) => {
			if (a === 'resolution') return { x: 1920, y: 1080 }
			if (a === 'fps') return 60
			return layers[l] ? layers[l].attrs[a] : undefined
		},
		getActiveComp: () => 'compNode#1',
		getFrame: () => 0,
		setFrame: () => {},
		parent: (c, p) => calls.push(['parent', c, p]),
		rename: () => {},
		setStroke: () => {},
		setFill: () => {},
		setInFrame: () => {},
		setOutFrame: () => {},
		getNiceName: (id) => (layers[id] || {}).name,
		connect: (...a) => calls.push(['connect', ...a]),
		getCompLayers: () => Object.keys(layers),
	}
	const cavalry = {
		fontExists: (f) => f === 'Inter',
		getFontStyles: () => ['Regular', 'Bold'],
		measureText: (s) => ({ width: s.length * 10 }),
	}
	const ctx = { api, cavalry, globalThis: {}, console: { warn: (m) => calls.push(['warn', m]), log() {} } }
	const src = fs.readFileSync(path.join(__dirname, '..', 'cav-helpers.js'), 'utf8')
	new Function('api', 'cavalry', 'globalThis', 'console', src)(ctx.api, ctx.cavalry, ctx.globalThis, ctx.console)
	return { cav: ctx.globalThis.cav, calls, layers, api, registry: ctx.globalThis }
}

test('key splits vectors and eases each component on the segment start', () => {
	const { cav, calls } = mockEnv()
	const r = cav.rect('r', 10, 10)
	cav.key(r, 'position', [
		[0, [1, 2], 'outBack'],
		[10, [3, 4]],
	])
	const keys = calls.filter((c) => c[0] === 'keyframe')
	assert.deepEqual(keys[0][3], { 'position.x': 1, 'position.y': 2 })
	const eases = calls.filter((c) => c[0] === 'ease')
	assert.deepEqual(
		eases.map((e) => [e[2], e[3], e[4]]),
		[
			['position.x', 0, 'Custom'],
			['position.y', 0, 'Custom'],
		],
	)
})

test('hex colour keys are split per channel', () => {
	const { cav, calls } = mockEnv()
	const r = cav.rect('r', 10, 10)
	cav.key(r, 'fill', [
		[0, '#ff8000'],
		[10, '#000000'],
	])
	const keys = calls.filter((c) => c[0] === 'keyframe').map((c) => c[3])
	assert.deepEqual(keys, [{ 'material.materialColor.r': 255 }, { 'material.materialColor.r': 0 }, { 'material.materialColor.g': 128 }, { 'material.materialColor.g': 0 }, { 'material.materialColor.b': 0 }, { 'material.materialColor.b': 0 }])
})

test('scale takes one number', () => {
	const { cav, calls } = mockEnv()
	const r = cav.rect('r', 10, 10)
	cav.key(r, 'scale', [[5, 1.5]])
	assert.deepEqual(calls.filter((c) => c[0] === 'keyframe')[0][3], { 'scale.x': 1.5, 'scale.y': 1.5 })
})

test('unknown easing names fail with the list of valid names', () => {
	const { cav } = mockEnv()
	const r = cav.rect('r', 10, 10)
	assert.throws(() => cav.key(r, 'opacity', [[0, 0, 'easeOutBounce']]), /unknown easing "easeOutBounce".*outBounce/)
})

test('bad layer ids and bad values fail clearly', () => {
	const { cav } = mockEnv()
	assert.throws(() => cav.key('nope#1', 'opacity', [[0, 1]]), /does not exist/)
	const r = cav.rect('r', 10, 10)
	assert.throws(() => cav.key(r, 'position', [[0, 5]]), /needs \[x, y\]/)
	assert.throws(() => cav.rgb('red'), /#rrggbb/)
})

test('create helpers clear the selection and reset the transform after parenting', () => {
	const { cav, calls } = mockEnv()
	const g = cav.group('g')
	const r = cav.rect('r', 10, 10, { parent: g })
	const sets = calls.filter((c) => c[0] === 'set' && c[1] === r)
	const last = sets[sets.length - 1][2]
	assert.equal(last['position.x'], 0)
	assert.equal(last['scale.x'], 1)
	assert.equal(last.rotation, 0)
	assert.ok(calls.some((c) => c[0] === 'select' && c[1].length === 0))
})

test('tween places a start key so later tweens hold', () => {
	const { cav, calls } = mockEnv()
	const r = cav.rect('r', 10, 10)
	cav.tween(r, 'opacity', 0, 10, 0, 100)
	cav.tween(r, 'opacity', 40, 50, 100, 0)
	const frames = calls.filter((c) => c[0] === 'keyframe').map((c) => c[2])
	assert.deepEqual(frames, [0, 10, 40, 50])
	assert.throws(() => cav.tween(r, 'opacity', 10, 5, 0, 1), /must be after/)
})

test('beat grid', () => {
	const { cav } = mockEnv()
	const g = cav.beats(120, { offset: 6 })
	assert.equal(g.frames, 30)
	assert.equal(g.beat(4), 126)
	assert.equal(g.bar(2), 246)
	assert.throws(() => cav.beats(0), /bpm/)
})

test('missing font falls back with a warning', () => {
	const { cav, calls } = mockEnv()
	assert.deepEqual(cav.font('Nope', 'Bold'), { font: 'Inter', style: 'Bold' })
	assert.ok(calls.some((c) => c[0] === 'warn' && /not installed/.test(c[1])))
})

test('every easing expression is valid and hits 0 and 1 at the ends', () => {
	const { cav } = mockEnv()
	for (const [name, expr] of Object.entries(cav.E)) {
		const f = new Function('x', 'var pow=Math.pow,exp=Math.exp,sin=Math.sin,cos=Math.cos; return ' + expr)
		assert.ok(Math.abs(f(0)) < 1e-6, name + '(0)=' + f(0))
		assert.ok(Math.abs(f(1) - 1) < 1e-3, name + '(1)=' + f(1))
	}
})

test('zoomThrough ends with the target at the centre', () => {
	const { cav, calls } = mockEnv()
	const rig = cav.group('rig')
	cav.zoomThrough(rig, 0, 30, [200, 100], { scale: 10, ease: 'inOutCubic' })
	const keys = calls.filter((c) => c[0] === 'keyframe' && c[3]['position.x'] !== undefined)
	const last = keys[keys.length - 1][3]
	const sKeys = calls.filter((c) => c[0] === 'keyframe' && c[3]['scale.x'] !== undefined)
	const s = sKeys[sKeys.length - 1][3]['scale.x']
	assert.ok(Math.abs(last['position.x'] + 200 * s) < 1e-6)
	assert.deepEqual(keys[0][3], { 'position.x': 0, 'position.y': 0 })
})

test('repeated key calls preserve each submitted frame', () => {
	const { cav, calls } = mockEnv(),
		l = cav.rect('r', 10, 10)
	cav.key(l, 'opacity', [
		[0, 0],
		[10, 100],
	])
	cav.key(l, 'opacity', [
		[20, 100],
		[30, 0],
	])
	assert.deepEqual(
		calls.filter((c) => c[0] === 'keyframe').map((c) => c[2]),
		[0, 10, 20, 30]
	)
})
test('precomp restores comp and playhead after a build error', () => {
	const { cav, api, layers } = mockEnv()
	let comp = 'root',
		frame = 17
	api.getActiveComp = () => comp
	api.getFrame = () => frame
	api.setActiveComp = (c) => {
		comp = c
	}
	api.setFrame = (f) => {
		frame = f
	}
	api.createComp = () => {
		layers.sub = { attrs: {} }
		return 'sub'
	}
	assert.throws(
		() =>
			cav.precomp('test', () => {
				frame = 99
				throw Error('build failed')
			}),
		/build failed/
	)
	assert.equal(comp, 'root')
	assert.equal(frame, 17)
})
test('axes use real ordered tags, clamp values, and validate before setting', () => {
	const { cav, api, registry, calls, layers } = mockEnv(),
		l = cav.rect('text', 10, 10)
	layers[l].attrs.font = { font: 'Variable' }
	registry.CAV_FONT_AXES = {
		Variable: [
			{ tag: 'ROND', name: 'Roundness', min: 0, max: 100 },
			{ tag: 'wght', name: 'Weight', min: 100, max: 900 },
		],
	}
	api.getAttrChildren = () => ['fontAxes.0', 'fontAxes.1']
	api.getAttributeNiceName = (_, a) => (a.endsWith('0') ? 'Roundness' : 'Weight')
	cav.axes(l, { wght: 999, ROND: 50 })
	assert.deepEqual(calls.at(-1), ['set', l, { 'fontAxes.1': 900, 'fontAxes.0': 50 }])
	assert.ok(calls.some((c) => c[0] === 'warn' && c[1].includes('clamped')))
	const before = calls.length
	assert.throws(() => cav.axes(l, { wght: 600, FAKE: 1 }), /unknown axis/)
	assert.equal(calls.length, before)
	api.getAttributeNiceName = () => 'Wrong'
	assert.throws(() => cav.axes(l, { wght: 600 }), /axis order/)
})
test('SkSL removes only generated input declarations including comma lists', () => {
	const { cav, api, layers } = mockEnv(),
		l = cav.rect('r', 10, 10)
	let children = ['inputs.0']
	api.addDynamic = () => children.push('inputs.' + children.length)
	api.getAttrChildren = () => children
	api.renameAttribute = () => {}
	api.getCustomAttributeName = () => 'n0'
	api.getAttributeNiceName = () => 'n0'
	api.getAttrType = () => 'double'
	const f = cav.sksl(
		l,
		'uniform shader layer; uniform highp float amount, other; half4 main(float2 p){return layer.eval(p)*amount;}',
		{ inputs: { amount: 0.5 } }
	)
	assert.ok(layers[f].attrs.code.includes('uniform shader layer;'))
	assert.ok(layers[f].attrs.code.includes('uniform highp float  other;'))
	assert.ok(!layers[f].attrs.code.includes('float amount'))
	assert.throws(() => cav.sksl(l, '', { inputs: { layer: 1 } }), /invalid\/reserved/)
	const reused = cav.sksl(l, 'uniform float n0;', { inputs: { n0: 0.25 } })
	assert.equal(layers[reused].attrs['inputs.0'], 0.25)
	assert.equal(layers[reused].attrs.code, '')
})
test('copy drivers coerce rotation and colour and reject wrong return types', () => {
	const { cav, api, layers } = mockEnv(),
		l = cav.rect('r', 10, 10)
	api.getAttrType = () => 'double'
	let driver = cav.copyDriver(l, 'shapeRotation', 'ctx.index*12')
	assert.equal(
		new Function('ctx', 'return eval(' + JSON.stringify(layers[driver].attrs.expression) + ')')({ index: 2 }),
		24
	)
	driver = cav.copyDriver(l, 'shapeRotation', '({x:0,y:0,z:24})')
	assert.equal(eval(layers[driver].attrs.expression), 24)
	api.getAttrType = () => 'double3'
	driver = cav.copyDriver(l, 'shapeRotation', 'ctx.index*12')
	assert.deepEqual(
		new Function('ctx', 'return eval(' + JSON.stringify(layers[driver].attrs.expression) + ')')({ index: 2 }),
		{ x: 0, y: 0, z: 24 }
	)
	driver = cav.copyDriver(l, 'shapeRotation', '"wrong"')
	assert.throws(() => eval(layers[driver].attrs.expression), /rotation driver requires/)
	api.getAttrType = () => 'color'
	driver = cav.copyDriver(l, 'fill', '"#ff800080"')
	assert.deepEqual(eval(layers[driver].attrs.expression), { r: 255, g: 128, b: 0, a: 128 })
	api.getAttrType = () => ''
	assert.throws(() => cav.copyDriver(l, 'shapeColor', '1'), /unknown destination/)
})
test('driver uses a two-key clock and Look At connects its target', () => {
	const { cav, api, calls } = mockEnv(),
		l = cav.rect('r', 10, 10),
		target = cav.rect('t', 10, 10)
	api.get = (id, a) => (a === 'startFrame' ? 0 : a === 'endFrame' ? 100 : { x: 1920, y: 1080 })
	const driver = cav.driver(l, 'position.x', 'n0*100')
	assert.deepEqual(
		calls.filter((c) => c[0] === 'keyframe' && c[1] === driver).map((c) => [c[2], c[3]]),
		[
			[0, { 'array.0': 0 }],
			[100, { 'array.0': 1 }],
		]
	)
	const aim = cav.lookAt(l, target, { offset: 90 })
	assert.ok(calls.some((c) => JSON.stringify(c) === JSON.stringify(['connect', target, 'id', aim, 'target'])))
	assert.ok(calls.some((c) => c[0] === 'set' && c[1] === aim && c[2].offset === 90))
})
test('above handles frontmost siblings without using the no-op empty reorder', () => {
	const { cav, api, calls } = mockEnv(),
		a = cav.rect('a', 10, 10),
		b = cav.rect('b', 10, 10)
	api.getParent = () => ''
	api.getCompLayers = () => [b, a]
	api.reorder = (...args) => calls.push(['reorder', ...args])
	cav.above(a, b)
	assert.deepEqual(
		calls.filter((c) => c[0] === 'reorder'),
		[
			['reorder', a, b],
			['reorder', b, a],
		]
	)
	assert.equal(cav.below, cav.order)
})

test('precomp restores comp/playhead when new-comp setup itself fails', () => {
	const { cav, api, layers } = mockEnv()
	let comp = 'root',
		frame = 17
	api.getActiveComp = () => comp
	api.getFrame = () => frame
	api.setActiveComp = (c) => {
		comp = c
	}
	api.setFrame = (f) => {
		frame = f
	}
	api.createComp = () => {
		comp = 'sub'
		frame = 0
		layers.sub = { attrs: {} }
		return 'sub'
	}
	api.set = () => {
		throw Error('setup failed')
	}
	assert.throws(() => cav.precomp('test', () => {}), /setup failed/)
	assert.equal(comp, 'root')
	assert.equal(frame, 17)
})
