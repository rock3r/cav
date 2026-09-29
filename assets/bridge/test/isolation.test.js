// Every UI script in Cavalry shares one global scope, so cav-bridge must not add global names
// (it once clashed with the cavalry-mcp bridge) and must leave `api` unchanged between jobs.
// Run with: node --test assets/bridge/test
const test = require('node:test')
const assert = require('node:assert')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')

const BRIDGE = fs.readFileSync(path.join(__dirname, '..', 'cav-bridge.js'), 'utf8')
const TOKEN = 'a'.repeat(64)

function makeCavalry() {
	const posts = []
	const files = {}
	const timers = []
	let getResult = ''
	function WebServer() {}
	WebServer.prototype.setResultForGet = (s) => { getResult = s }
	WebServer.prototype.listen = () => {}
	WebServer.prototype.addCallbackObject = () => {}
	WebServer.prototype.setHighFrequency = () => {}
	WebServer.prototype.postCount = () => posts.length
	WebServer.prototype.getNextPost = () => ({ result: posts.shift() })
	function Timer(cb) { timers.push(cb) }
	Timer.prototype.setRepeating = () => {}
	Timer.prototype.setInterval = () => {}
	Timer.prototype.start = () => {}
	const api = {
		WebServer,
		Timer,
		getHomeFolder: () => '/home/test',
		filePathExists: () => true,
		makeFolder: () => {},
		readFromFile: (p) => (p.endsWith('/token') ? TOKEN : files[p]),
		writeToFile: (p, s) => { files[p] = s },
		getCavalryVersion: () => '2.7.2',
		getActiveComp: () => 'compNode#1',
	}
	function widget() {}
	widget.prototype.setAlignment = () => {}
	widget.prototype.addStretch = () => {}
	widget.prototype.add = () => {}
	const ui = { Label: widget, VLayout: widget, setTitle() {}, add() {}, show() {} }
	const cavalry = { versionLessThan: () => false }
	return { api, ui, cavalry, posts, files, timers, get getResult() { return getResult } }
}

test('cav-bridge adds no global names and restores api after a job', () => {
	const c = makeCavalry()
	const context = vm.createContext({ api: c.api, ui: c.ui, cavalry: c.cavalry, console: { log() {}, info() {}, warn() {}, error() {} } })
	context.globalThis = context
	const originals = Object.fromEntries(Object.entries(c.api))
	const before = new Set(Object.keys(context))

	vm.runInContext(BRIDGE, context)
	const added = Object.keys(context).filter((k) => !before.has(k))
	assert.deepStrictEqual(added, [], 'bridge must not define globals')
	for (const [name, fn] of Object.entries(originals)) {
		assert.strictEqual(c.api[name], fn, `api.${name} must be unchanged after loading`)
	}

	// One job with the helper library: `cav` exists during the job only.
	c.files['/spool/helpers.js'] = ';(function () { globalThis.cav = { ok: true } })()'
	c.posts.push(JSON.stringify({ id: 'job1', token: TOKEN, code: 'return [typeof cav, api.getActiveComp()]', preload: '/spool/helpers.js', preloadVersion: 'v1' }))
	c.timers[0].onTimeout()
	const result = JSON.parse(c.files['/home/test/.cav/jobs/job1.json'])
	assert.strictEqual(result.ok, true, JSON.stringify(result.error))
	assert.deepStrictEqual(result.value, ['object', 'compNode#1'])
	assert.strictEqual(context.cav, undefined, 'cav must be removed after the job')
	for (const [name, fn] of Object.entries(originals)) {
		assert.strictEqual(c.api[name], fn, `api.${name} must be restored after a job`)
	}

	// Job code runs in global scope: it cannot read the bridge's token.
	c.posts.push(JSON.stringify({ id: 'job2', token: TOKEN, code: 'return typeof TOKEN' }))
	c.timers[0].onTimeout()
	assert.strictEqual(JSON.parse(c.files['/home/test/.cav/jobs/job2.json']).value, 'undefined')
})
