const test = require('node:test'), assert = require('node:assert/strict'), fs = require('node:fs'), path = require('node:path'), vm = require('node:vm')
const source = fs.readFileSync(path.join(__dirname, '..', 'sketch.js'), 'utf8')

// fakeCavalry is a scratch comp with the given layers; convertSVGToLayers adds a group with
// shapes inside it, like Cavalry does. Child layers are not top level.
function fakeCavalry(opts) {
	opts = opts || {}
	const layers = new Set(opts.layers || []), calls = { frames: [], renders: [], deleted: [] }
	const api = {
		getActiveComp: () => opts.active || 'comp#1',
		get: (id, attr) => (attr === 'startFrame' ? 12 : undefined),
		getCompLayers: top => Array.from(layers).filter(id => !top || !id.startsWith('child')),
		convertSVGToLayers: p => {
			calls.imported = p
			if (opts.throwOnImport) throw new Error('bad svg')
			if (opts.imported) return opts.imported
			layers.add('group#2'); layers.add('child#3'); layers.add('child#4')
			return ['group#2', 'child#3', 'child#4']
		},
		setFrame: f => calls.frames.push(f),
		renderPNGFrame: (p, s) => calls.renders.push([p, s]),
		filePathExists: () => opts.noPng !== true,
		layerExists: id => layers.has(id),
		deleteLayer: id => {
			calls.deleted.push(id)
			layers.delete(id)
			if (id === 'group#2') { layers.delete('child#3'); layers.delete('child#4') }
		},
	}
	return { api, calls, layers }
}
const sample = { svg: '/b/.cav-sketch/s1.svg', out: '/b/.cav-sketch/s1', scale: 100, comp: 'comp#1' }
// JSON copies: objects made inside the VM context have its prototypes.
const run = cv => JSON.parse(JSON.stringify(vm.runInNewContext('(function(){' + source + '})()', { api: cv.api, sample })))

test('imports, renders at the start frame and deletes everything it imported', () => {
	const cv = fakeCavalry({ layers: ['keep#1'] })
	const r = run(cv)
	assert.deepEqual(r, { png: '/b/.cav-sketch/s1.png', layers: 3 })
	assert.equal(cv.calls.imported, sample.svg)
	assert.deepEqual(cv.calls.frames, [12])
	assert.deepEqual(cv.calls.renders, [['/b/.cav-sketch/s1', 100]])
	assert.deepEqual(Array.from(cv.layers), ['keep#1'])
})

test('refuses to draw when another comp is active, and touches nothing', () => {
	const cv = fakeCavalry({ active: 'comp#9' })
	assert.throws(() => run(cv), /scratch comp changed/)
	assert.equal(cv.calls.imported, undefined)
	assert.deepEqual(cv.calls.deleted, [])
})

test('an empty import is an error and leaves no layers behind', () => {
	const cv = fakeCavalry({ imported: [] })
	assert.throws(() => run(cv), /imported nothing/)
	assert.equal(cv.layers.size, 0)
})

test('a missing PNG is an error and the import is still deleted', () => {
	const cv = fakeCavalry({ noPng: true })
	assert.throws(() => run(cv), /did not write/)
	assert.equal(cv.layers.size, 0)
})

test('a failed import keeps the layers that were there before', () => {
	const cv = fakeCavalry({ throwOnImport: true, layers: ['keep#1'] })
	assert.throws(() => run(cv), /bad svg/)
	assert.deepEqual(Array.from(cv.layers), ['keep#1'])
})

test('a stray top-level layer made by the import is deleted too', () => {
	const cv = fakeCavalry({ imported: ['group#2'] })
	cv.api.convertSVGToLayers = () => { cv.layers.add('group#2'); cv.layers.add('stray#5'); return ['group#2'] }
	run(cv)
	assert.equal(cv.layers.size, 0)
	assert.ok(cv.calls.deleted.includes('stray#5'))
})
