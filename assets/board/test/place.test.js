const test = require('node:test'), assert = require('node:assert/strict'), fs = require('node:fs'), path = require('node:path'), vm = require('node:vm')
const source = fs.readFileSync(path.join(__dirname, '..', 'place.js'), 'utf8')

// fakeCavalry is a composition with the given settings and layers; it records what the job does.
function fakeCavalry(comp, layers) {
	layers = layers || []
	const assets = {}, calls = { load: [], connect: [], set: [], added: [] }
	let n = 0
	const byId = id => layers.find(l => l.id === id)
	const api = {
		getActiveComp: () => 'comp',
		get: (id, attr) => ({ resolution: { x: comp.width, y: comp.height }, fps: comp.fps, startFrame: comp.start, endFrame: comp.end })[attr],
		getAssetWindowLayers: () => Object.values(assets),
		isFileAsset: () => true,
		getAssetFilePath: a => Object.keys(assets).find(p => assets[p] === a),
		reloadAsset: () => {},
		loadAsset: p => { calls.load.push(p); return (assets[p] = 'asset#' + ++n) },
		getCompLayers: () => layers.map(l => l.id),
		getLayerType: id => byId(id).type,
		getNiceName: id => byId(id).name,
		getOutConnections: id => byId(id).outs || [],
		connect: (from, fa, to, ta) => calls.connect.push([from, to, ta]),
		filePathExists: () => true,
		select: () => {},
		addAssetToComp: a => { const id = 'footageShape#' + ++n; layers.push({ id, type: 'footageShape', name: '' }); calls.added.push(a); return [id] },
		rename: (id, name) => { byId(id).name = name },
		set: (id, v) => calls.set.push([id, v]),
		setInFrame: (id, f) => { byId(id).in = f },
		setOutFrame: (id, f) => { byId(id).out = f },
	}
	return { api, calls, layers }
}
// JSON copies: objects made inside the VM context have its prototypes.
const run = (cv, shots) => JSON.parse(JSON.stringify(vm.runInNewContext('(function(){' + source + '})()', { api: cv.api, shots })))
const comp = { width: 1920, height: 1080, fps: 60, start: 0, end: 600 }
const shot = (id, start, end, extra) => Object.assign({ id, path: '/b/' + id + '.png', start, end, width: 1280, height: 720 }, extra)

test('shots hand off without overlap and fit the composition', () => {
	const r = run(fakeCavalry(comp), [shot('s1', 0, 2), shot('s2', 2, 4), shot('s3', 5, 6)])
	assert.deepEqual(r.placed.map(p => [p.id, p.in, p.out]), [['s1', 0, 119], ['s2', 120, 239], ['s3', 300, 359]])
	assert.ok(r.placed.every(p => p.scale === 1.5 && !p.updated))
	assert.deepEqual(r.comp, comp)
})

test('beat 0 lands on the composition start frame, and odd rates round to the nearest frame', () => {
	const r = run(fakeCavalry({ width: 1080, height: 1920, fps: 25, start: 100, end: 400 }), [shot('s1', 2.25, 4.25), shot('s2', 4.25, 4.26)])
	assert.deepEqual(r.placed.map(p => [p.in, p.out]), [[156, 205], [206, 206]])
	// A landscape frame in a portrait composition fits its width.
	assert.equal(r.placed[0].scale, 1080 / 1280)
})

test('an unknown image size is placed at native size', () => {
	const cv = fakeCavalry(comp), r = run(cv, [shot('s1', 0, 1, { width: 0, height: 0 })])
	assert.equal(r.placed[0].scale, 0)
	assert.equal(cv.calls.set.length, 0)
})

test('shots that share a frame load it once', () => {
	const cv = fakeCavalry(comp)
	run(cv, [shot('s1', 0, 1, { path: '/b/a.png' }), shot('s2', 1, 2, { path: '/b/a.png' })])
	assert.deepEqual(cv.calls.load, ['/b/a.png'])
	assert.equal(cv.calls.added.length, 2)
})

test('an existing placeholder is updated in place and keeps its transforms', () => {
	const cv = fakeCavalry(comp, [
		{ id: 'footageShape#9', type: 'footageShape', name: 's1' },
		{ id: 'imageShader#9', type: 'imageShader', name: 's1 Image Shader', outs: ['footageShape#9.material.colorShaders.0'] },
	])
	const r = run(cv, [shot('s1', 1, 2)])
	assert.deepEqual(r.placed[0], { id: 's1', layer: 'footageShape#9', in: 60, out: 119, scale: 0, updated: true })
	assert.deepEqual(cv.calls.connect, [['asset#1', 'imageShader#9', 'image']])
	assert.equal(cv.calls.set.length, 0)
	assert.equal(cv.calls.added.length, 0)
})

test('a built layer with the shot name is kept and the shot skipped', () => {
	const cv = fakeCavalry(comp, [{ id: 'group#1', type: 'group', name: 's2' }])
	const r = run(cv, [shot('s2', 0, 1)])
	assert.deepEqual(r.skipped, [{ id: 's2', reason: 'built', layer: 'group#1', type: 'group' }])
	assert.equal(r.placed.length, 0)
	assert.equal(cv.calls.added.length, 0)
})

test('a shot before beat 0 is skipped, and one that starts before it is cut', () => {
	// A negative offset moves s1 before the composition and s2 across its start.
	const r = run(fakeCavalry(comp), [shot('s1', -2, -0.5), shot('s2', -0.5, 1), shot('s3', 1, 2)])
	assert.deepEqual(r.skipped, [{ id: 's1', reason: 'before' }])
	assert.deepEqual(r.placed.map(p => [p.id, p.in, p.out]), [['s2', 0, 59], ['s3', 60, 119]])
})

test('the last frame counts skipped built shots too', () => {
	const cv = fakeCavalry(comp, [{ id: 'group#1', type: 'group', name: 's2' }])
	const r = run(cv, [shot('s1', 0, 1), shot('s2', 1, 20)])
	assert.equal(r.last, 1199)
})
