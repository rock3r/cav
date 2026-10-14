const test = require('node:test'), assert = require('node:assert/strict'), fs = require('node:fs'), path = require('node:path'), vm = require('node:vm')
const source = fs.readFileSync(path.join(__dirname, '..', 'reframe.js'), 'utf8')
const helpers = fs.readFileSync(path.join(__dirname, '..', '..', 'helpers', 'cav-helpers.js'), 'utf8')

// Layer specs: {name, type, children, pos, scale, keyed, box:[l, r, t, b] (world), size, in, out, ref}.
// A leaf has a box; a group has children. fakeCavalry models comps, layers and the calls that change them.
function fakeCavalry(spec) {
	const comps = {}, layers = {}, mutations = []
	let n = 0, active = 'compNode#1', frame = 0
	const comp = (name, w, h, tops) => {
		const id = 'compNode#' + ++n
		comps[id] = { id, name, w, h, start: 0, end: 119, top: tops || [], guides: [] }
		return id
	}
	const make = (compId, s, parent) => {
		const id = (s.type || (s.children ? 'group' : 'basicShape')) + '#' + ++n
		layers[id] = {
			id, comp: compId, parent, name: s.name, type: s.type || (s.children ? 'group' : 'basicShape'), children: [],
			pos: s.pos || { x: 0, y: 0 }, scale: s.scale || { x: 1, y: 1 }, keyed: !!s.keyed, box: s.box, size: s.size,
			inF: s.in === undefined ? 0 : s.in, outF: s.out === undefined ? 119 : s.out, opacity: 100, hidden: false, ref: s.ref,
		}
		layers[id].children = (s.children || []).map(c => make(compId, c, id))
		return id
	}
	const source = comp('Launch', spec.w || 1920, spec.h || 1080)
	comps[source].top = spec.layers.map(l => make(source, l, null))
	active = source
	const fail = {}
	const track = (fn, ...targets) => mutations.push({ fn, targets, comp: active })
	const list = id => { const l = layers[id]; return l.parent ? layers[l.parent].children : comps[l.comp].top }
	const remove = id => { const a = list(id); a.splice(a.indexOf(id), 1) }
	const api = {
		getComps: () => Object.keys(comps),
		getActiveComp: () => active,
		setActiveComp: id => { active = id },
		getFrame: () => frame,
		setFrame: f => { frame = f },
		getNiceName: id => (comps[id] || layers[id] || { name: id }).name,
		getLayerType: id => layers[id].type,
		getInFrame: id => layers[id].inF,
		getOutFrame: id => layers[id].outF,
		getChildren: id => layers[id].children.slice(),
		getParent: id => layers[id].parent,
		getCompLayers: top => {
			if (top) return comps[active].top.slice()
			const out = [], walk = l => l.forEach(i => { out.push(i); walk(layers[i].children) })
			walk(comps[active].top)
			return out
		},
		getAnimatedAttributes: id => layers[id].keyed ? ['position.x', 'position.y'] : [],
		getInConnection: () => '',
		getBoundingBox: id => {
			const b = layers[id].box
			return { left: b[0], right: b[1], top: b[2], bottom: b[3], width: b[1] - b[0], height: b[2] - b[3] }
		},
		getCompFromReference: id => layers[id].ref,
		get: (id, attr) => {
			if (comps[id]) return { resolution: { x: comps[id].w, y: comps[id].h }, startFrame: comps[id].start, endFrame: comps[id].end }[attr]
			const l = layers[id]
			return { position: l.pos, scale: l.scale, opacity: l.opacity, hidden: l.hidden, fontSize: l.size }[attr]
		},
		select: () => {},
		duplicate: id => {
			track('duplicate', id)
			const c = comps[id], nid = comp(c.name, c.w, c.h)
			const copy = (lid, parent) => {
				const o = layers[lid], cid = o.type + '#' + ++n
				layers[cid] = Object.assign({}, o, { id: cid, comp: nid, parent, pos: Object.assign({}, o.pos), scale: Object.assign({}, o.scale) })
				layers[cid].children = o.children.map(k => copy(k, cid))
				return cid
			}
			comps[nid].top = c.top.map(t => copy(t, null))
			return nid
		},
		rename: (id, name) => { track('rename', id); if (fail.rename) throw new Error('rename failed'); (comps[id] || layers[id]).name = name },
		set: (id, v) => {
			track('set', id)
			if (fail.set && v['position.x'] !== undefined) throw new Error('set failed')
			if (comps[id]) { if (v.resolution) { comps[id].w = v.resolution[0]; comps[id].h = v.resolution[1] } return }
			const l = layers[id]
			if (v['position.x'] !== undefined) l.pos = { x: v['position.x'], y: v['position.y'] }
			if (v['scale.x'] !== undefined) l.scale = { x: v['scale.x'], y: v['scale.y'] }
		},
		create: (type, name) => {
			track('create', active)
			const id = type + '#' + ++n
			layers[id] = { id, comp: active, parent: null, name, type, children: [], pos: { x: 0, y: 0 }, scale: { x: 1, y: 1 }, inF: 0, outF: 119, opacity: 100, hidden: false }
			comps[active].top.unshift(id)
			return id
		},
		reorder: (a, below) => {
			track('reorder', a, below)
			remove(a)
			const arr = list(below)
			arr.splice(arr.indexOf(below) + 1, 0, a)
		},
		parent: (child, p) => {
			track('parent', child, p)
			remove(child)
			layers[child].parent = p
			layers[p].children.push(child)
		},
		deleteLayer: id => { track('deleteLayer', id); delete comps[id] },
		addGuide: (c, vertical, pos) => { track('addGuide', c); comps[c].guides.push([vertical, pos]); return 1 },
		setInFrame: id => track('setInFrame', id),
		setOutFrame: id => track('setOutFrame', id),
	}
	return { api, comps, layers, mutations, source, fail, state: () => ({ active, frame }) }
}

// Run the job like the CLI does: the helpers are loaded first, then the job body.
function run(cv, opts) {
	const o = Object.assign({ source: '', to: [9, 16], name: '', key: [], scale: 0, samples: 3, minText: 28, dryRun: false }, opts)
	const ctx = vm.createContext({ api: cv.api, cavalry: {}, console: { warn() {}, log() {} }, opts: o })
	ctx.globalThis = ctx
	vm.runInContext(helpers, ctx)
	return JSON.parse(JSON.stringify(vm.runInContext('(function(){' + source + '\n})()', ctx)))
}

const FULL = [-960, 960, 540, -540]
// A launch-style scene: a full-frame bg plane, and a rig with a card, a 40 px title and a bar.
const launch = (extra) => Object.assign({
	layers: [
		{ name: 'rig', children: [
			{ name: 'card', box: [-800, 800, 400, -400] },
			{ name: 'title', type: 'textShape', size: 40, box: [-300, 300, 360, 320] },
			{ name: 'bar', box: FULL },
		] },
		{ name: 'bg', box: FULL },
	],
}, extra || {})
const topNamed = (cv, comp, name) => Object.values(cv.layers).find(l => l.comp === comp && l.name === name)
const near = (a, b, e) => assert.ok(Math.abs(a - b) < (e || 1e-9), a + ' vs ' + b)

test('a full-frame rig is fitted into the middle half of a 1080x1920 copy', () => {
	const cv = fakeCavalry(launch())
	const r = run(cv, {})
	assert.deepEqual([r.target.w, r.target.h, r.target.name], [1080, 1920, 'Launch 9x16'])
	near(r.scale, 0.50625)
	near(r.offset[0], 0); near(r.offset[1], 0)
	near(r.cover, 1920 / 1080)
	assert.equal(cv.comps[r.target.id].w, 1080)
	assert.equal(cv.comps[r.target.id].h, 1920)
	assert.deepEqual(cv.state(), { active: r.target.id, frame: 0 })
	const rig = topNamed(cv, r.target.id, 'rig'), bg = topNamed(cv, r.target.id, 'bg')
	near(rig.scale.x, 0.50625)
	near(bg.scale.x, 1920 / 1080)
	assert.deepEqual(r.backgrounds.map(b => b.name), ['bg'])
	assert.deepEqual(r.key.layers.map(k => k.name), ['rig'])
	assert.equal(r.key.mode, 'rig')
	// The guides mark the middle half and the safe area.
	assert.equal(cv.comps[r.target.id].guides.length, 4)
})

test('the same rig is fitted into a 1080x1080 square', () => {
	const cv = fakeCavalry(launch())
	const r = run(cv, { to: [1, 1] })
	assert.deepEqual([r.target.w, r.target.h, r.target.name], [1080, 1080, 'Launch 1x1'])
	// The middle half of a square is 972 x 540, so the height limits the fit: 540 / 1080.
	near(r.scale, 0.5)
	near(r.cover, 1)
})

test('a small key group is never enlarged unless --scale says so', () => {
	const spec = { layers: [{ name: 'rig', children: [{ name: 'dot', box: [-200, 200, 100, -100] }] }] }
	let r = run(fakeCavalry(spec), {})
	assert.equal(r.scale, 1)
	near(r.offset[0], 0); near(r.offset[1], 0)
	r = run(fakeCavalry(spec), { scale: 2 })
	assert.equal(r.scale, 2)
})

test('the fit centres a rig that sits off-centre in the original', () => {
	const cv = fakeCavalry({ layers: [{ name: 'rig', children: [{ name: 'card', box: [100, 500, 300, 100] }] }] })
	const r = run(cv, {})
	// Centre (300, 200) must land on the centre of the middle half (0, 0).
	near(r.offset[0], -300); near(r.offset[1], -200)
})

test('a frame-sized comp reference is not a background, a frame-sized shape is', () => {
	const cv = fakeCavalry({ layers: [
		{ name: 'scene', type: 'compositionReference', ref: 'compNode#99', box: FULL },
		{ name: 'wash', box: FULL },
		{ name: 'rig', children: [{ name: 'card', box: [-400, 400, 200, -200] }] },
	] })
	const r = run(cv, {})
	assert.deepEqual(r.backgrounds.map(b => b.name), ['wash'])
	const ref = topNamed(cv, r.target.id, 'scene')
	assert.equal(ref.scale.x, r.scale)
})

test('a keyed rig is wrapped in a group, in the order create, reorder, parent, set', () => {
	const cv = fakeCavalry({ layers: [
		{ name: 'rig', keyed: true, pos: { x: 50, y: 20 }, children: [{ name: 'card', box: FULL }] },
		{ name: 'logo', pos: { x: 100, y: 40 }, scale: { x: 2, y: 2 }, box: [-100, 100, 60, 20] },
	] })
	const r = run(cv, {})
	const after = cv.mutations.slice(cv.mutations.findIndex(m => m.fn === 'create'))
	assert.deepEqual(after.slice(0, 4).map(m => m.fn), ['create', 'reorder', 'parent', 'set'])
	const rig = topNamed(cv, r.target.id, 'rig'), group = cv.layers[rig.parent]
	assert.equal(group.name, 'reframe rig')
	// The keyed layer keeps its own values; the group carries the fit.
	assert.deepEqual(rig.pos, { x: 50, y: 20 })
	assert.deepEqual(rig.scale, { x: 1, y: 1 })
	near(group.scale.x, r.scale)
	assert.equal(r.wrapped.length, 1)
	assert.equal(r.wrapped[0].group, group.id)
	// The static layer is composed directly: position and scale follow the affine.
	const logo = topNamed(cv, r.target.id, 'logo')
	near(logo.pos.x, r.scale * 100 + r.offset[0])
	near(logo.scale.x, r.scale * 2)
})

test('a world point of a static layer with a pivot maps through the reframe affine', () => {
	// world = position + scale * (p - pivot); the pivot is a local offset, so it does not change the mapping.
	const world = (l, pivot, p) => ({ x: l.pos.x + l.scale.x * (p.x - pivot.x), y: l.pos.y + l.scale.y * (p.y - pivot.y) })
	const cv = fakeCavalry({ layers: [{ name: 'rig', children: [{ name: 'card', box: [-800, 800, 400, -400] }] }, { name: 'badge', pos: { x: 300, y: -120 }, scale: { x: 1.5, y: 1.5 }, box: [250, 350, -80, -160] }] })
	const before = world(cv.layers[cv.comps[cv.source].top[1]], { x: 20, y: 10 }, { x: 40, y: 30 })
	const r = run(cv, {})
	const after = world(topNamed(cv, r.target.id, 'badge'), { x: 20, y: 10 }, { x: 40, y: 30 })
	near(after.x, r.scale * before.x + r.offset[0])
	near(after.y, r.scale * before.y + r.offset[1])
})

test('no change ever reaches the original comp or its layers', () => {
	const cv = fakeCavalry(launch({ layers: launch().layers.concat([{ name: 'spin', keyed: true, box: [-50, 50, 50, -50] }]) }))
	const sourceLayers = new Set(Object.keys(cv.layers))
	const r = run(cv, {})
	const mutating = cv.mutations.filter(m => m.fn !== 'duplicate')
	assert.ok(mutating.length > 5)
	for (const m of mutating) {
		for (const t of m.targets) {
			assert.ok(!sourceLayers.has(t), m.fn + ' targeted source layer ' + t)
			assert.notEqual(t, cv.source, m.fn + ' targeted the source comp')
		}
	}
	// The only call that names the source comp is the read-only copy.
	assert.deepEqual(cv.mutations.filter(m => m.targets.includes(cv.source)).map(m => m.fn), ['duplicate'])
	assert.equal(cv.comps[cv.source].w, 1920)
	assert.notEqual(r.target.id, cv.source)
})

test('--dry-run makes no mutating call and leaves the comp count and view as they were', () => {
	const cv = fakeCavalry(launch())
	const r = run(cv, { dryRun: true })
	assert.equal(cv.mutations.length, 0)
	assert.equal(Object.keys(cv.comps).length, 1)
	assert.deepEqual(cv.state(), { active: cv.source, frame: 0 })
	assert.equal(r.dryRun, true)
	assert.equal(r.target.id, null)
	near(r.scale, 0.50625)
})

test('an existing comp name is refused and nothing is copied', () => {
	const cv = fakeCavalry(launch())
	assert.throws(() => run(cv, { name: 'Launch' }), /already exists/)
	assert.equal(Object.keys(cv.comps).length, 1)
	assert.equal(cv.mutations.length, 0)
	assert.throws(() => { const c = fakeCavalry(launch()); run(c, { source: c.source }); run(c, { source: c.source }) }, /already exists/)
})

test('a failure while building deletes the partial copy and restores the comp and frame', () => {
	const cv = fakeCavalry(launch())
	cv.api.setFrame(7)
	cv.fail.set = true
	assert.throws(() => run(cv, {}), /set failed/)
	assert.deepEqual(Object.keys(cv.comps), [cv.source])
	assert.deepEqual(cv.state(), { active: cv.source, frame: 7 })
})

test('an unknown --key is an error and the aspect of the source is refused', () => {
	assert.throws(() => run(fakeCavalry(launch()), { key: ['nope'] }), /no layer named/)
	assert.throws(() => run(fakeCavalry({ w: 1080, h: 1920, layers: [{ name: 'a', box: [-1, 1, 1, -1] }] }), {}), /already 9:16/)
})

test('findings: small text, cut-off and off-frame content under a partial --key, shared pre-comps', () => {
	const cv = fakeCavalry({ layers: [
		{ name: 'rig', children: [
			{ name: 'card', box: [-400, 400, 200, -200] },
			{ name: 'title', type: 'textShape', size: 40, box: [-100, 100, 40, 0] },
		] },
		{ name: 'side', children: [{ name: 'wide panel', box: [-940, 940, 100, -100] }, { name: 'rail', box: [1500, 1700, 100, -100] }] },
		{ name: 'clip', ref: 'compNode#99', type: 'compositionReference', box: [-200, 200, 50, -50] },
	] })
	// Make the shared pre-comp name resolvable.
	cv.api.getNiceName = ((f) => id => id === 'compNode#99' ? 'Shared UI' : f(id))(cv.api.getNiceName)
	const r = run(cv, { key: ['rig'], scale: 1 })
	const kinds = r.findings.map(f => f.kind + ':' + (f.name || ''))
	const small = r.findings.find(f => f.kind === 'small-text')
	assert.equal(small, undefined, '40 px text at scale 1 is not small')
	const r2 = run(fakeCavalry({ layers: [
		{ name: 'rig', children: [{ name: 'title', type: 'textShape', size: 40, box: [-100, 100, 40, 0] }, { name: 'card', box: FULL }] },
	] }), {})
	const t = r2.findings.find(f => f.kind === 'small-text')
	assert.ok(t && /20 px/.test(t.detail), JSON.stringify(r2.findings))
	assert.ok(kinds.includes('offframe:rail'), kinds.join())
	assert.ok(kinds.includes('clipped:wide panel') || kinds.includes('outside-key:wide panel'), kinds.join())
	const shared = r.findings.find(f => f.kind === 'shared-precomp')
	assert.ok(shared && /Shared UI/.test(shared.detail) && /duplicate/.test(shared.fix))
	assert.equal(r.findings[r.findings.length - 1].kind, 'space')
})

test('content that reaches the right rail is reported as unsafe', () => {
	// Full-width rig at scale 0.506 fills 972 px; the safe area ends 129.6 px before the right edge.
	const r = run(fakeCavalry(launch()), {})
	assert.ok(r.findings.some(f => f.kind === 'unsafe'), JSON.stringify(r.findings.map(f => f.kind)))
})

test('findings carry ids in the new comp', () => {
	const cv = fakeCavalry(launch())
	const r = run(cv, {})
	const f = r.findings.find(x => x.kind === 'small-text')
	assert.equal(cv.layers[f.layer].comp, r.target.id)
	assert.equal(cv.layers[f.layer].name, 'title')
})

test('a layer cap and a time budget mark the coverage as incomplete', () => {
	const many = { layers: [{ name: 'rig', children: Array.from({ length: 450 }, (_, i) => ({ name: 'dot' + i, box: [-10, 10, 10, -10] })) }] }
	const cv = fakeCavalry(many)
	cv.api.duplicate = () => { throw new Error('stop') }
	let r = run(cv, { dryRun: true })
	assert.equal(r.coverageComplete, false)
	assert.ok(r.skipped >= 50)
	r = run(fakeCavalry(launch()), { dryRun: true, budgetMs: -1 })
	assert.equal(r.coverageComplete, false)
	assert.equal(run(fakeCavalry(launch()), { dryRun: true }).coverageComplete, true)
})

test('layers hidden or outside their in and out frames do not count toward the key box', () => {
	const cv = fakeCavalry({ layers: [{ name: 'rig', children: [
		{ name: 'card', box: [-400, 400, 200, -200] },
		{ name: 'later', box: [-900, 900, 500, -500], in: 500, out: 600 },
	] }] })
	const r = run(cv, { dryRun: true })
	assert.equal(r.key.box.r, 400)
})
