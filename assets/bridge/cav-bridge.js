// cav-bridge VERSION 1.0.2
//
// HTTP request/response bridge between the `cav` CLI and Cavalry.
// Runs inside Cavalry as a UI script: Scripts menu -> cav-bridge.
// Keep its window open while you use `cav`.
//
// Derived from the cavalry-mcp bridge by Michael Essandoh
// (https://github.com/m18h/cavalry-mcp), MIT licence:
//   Copyright (c) 2026 Michael Essandoh
//   Permission is hereby granted, free of charge, to any person obtaining a copy of this
//   software and associated documentation files (the "Software"), to deal in the Software
//   without restriction, including without limitation the rights to use, copy, modify, merge,
//   publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons
//   to whom the Software is furnished to do so, subject to the following conditions:
//   The above copyright notice and this permission notice shall be included in all copies or
//   substantial portions of the Software.
//   THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED.
//
// Protocol
// --------
//   POST http://127.0.0.1:8723/post
//     {"id": "<job id>", "token": "<~/.cav/token>", "file": "<path to .js>" | "code": "<js>",
//      "preload": "<path>", "preloadVersion": "<v>", "protocol": 1}
//   The bridge publishes {"type":"running","id":...} through GET /get while the job runs,
//   then {"type":"result","id":...,"ok":bool,"value":...,"logs":[...],"error":{...},"ms":N}.
//   The result is also written to ~/.cav/jobs/<id>.json, so a client that was not polling
//   (or that polled after another job replaced the GET payload) can still read it.
//   Job code runs inside a function, so `return` sends a value back.
//   Every payload carries "bridgeVersion" (the cav release that shipped this file) and
//   "protocol". The protocol changes only when this request or result format changes. The
//   bridge refuses a request whose "protocol" differs from its own; a request without one
//   is treated as protocol 1.
//
// Isolation (a precaution): the bridge keeps all its state inside one function and adds
// nothing global except the `cav` helpers, which exist only while a cav job runs. On Cavalry
// 2.8.0 each UI script has its own global scope anyway; 2.7.2 was not checked.

;(function () {
var BRIDGE_VERSION = '1.0.2'
var PROTOCOL = 1
// Optional, backwards-compatible identity for this particular bridge window.
var SESSION_ID = String(Date.now()) + '-' + String(Math.random()).slice(2)
var MIN_CAVALRY_VERSION = '2.4.0'
var HOST = '127.0.0.1'
var PORT = 8723

if (cavalry.versionLessThan(MIN_CAVALRY_VERSION)) {
	throw new Error('cav-bridge needs Cavalry ' + MIN_CAVALRY_VERSION + ' or newer')
}

var HOME = api.getHomeFolder().replace(/\\/g, '/').replace(/\/+$/, '')
var CAV_DIR = HOME + '/.cav'
var TOKEN_PATH = CAV_DIR + '/token'
var JOBS_DIR = CAV_DIR + '/jobs'

if (!api.filePathExists(TOKEN_PATH)) {
	throw new Error('cav-bridge: token file missing at ' + TOKEN_PATH + '. Run `cav setup`.')
}
var TOKEN = String(api.readFromFile(TOKEN_PATH)).trim()
if (TOKEN.length < 32) {
	throw new Error('cav-bridge: token in ' + TOKEN_PATH + ' is too short. Run `cav setup`.')
}
if (!api.filePathExists(JOBS_DIR)) {
	api.makeFolder(JOBS_DIR)
}

var server = new api.WebServer()
var jobCount = 0
var lastJobAt = 0

function jsonSafe(value) {
	if (value === undefined) {
		return null
	}
	try {
		var text = JSON.stringify(value)
		return text === undefined ? null : JSON.parse(text)
	} catch (err) {
		try {
			return String(value)
		} catch (err2) {
			return '[unserializable value]'
		}
	}
}

// Line number of the failing statement inside the job's own code. Frames from the job
// read "eval at execute"; frames from the helper library read "eval at preload" (or none).
function errorLine(err) {
	var stack = String((err && err.stack) || '')
	var m = stack.match(/eval at execute[^\n]*<anonymous>:(\d+):(\d+)/)
	if (!m) {
		return null
	}
	return { line: Number(m[1]) - 1, column: Number(m[2]) }
}

function describeArgs(args) {
	var parts = []
	for (var i = 0; i < args.length && i < 4; i++) {
		var text
		try {
			text = JSON.stringify(args[i])
		} catch (err) {
			text = String(args[i])
		}
		if (text === undefined) {
			text = String(args[i])
		}
		parts.push(text.length > 80 ? text.slice(0, 77) + '...' : text)
	}
	return parts.join(', ')
}

// Errors thrown by Cavalry's native functions have no stack, so the caller cannot see
// which call failed or on which line. During a cav job each api function is replaced by a
// wrapper that throws a JavaScript error naming the call and carrying the caller's stack.
// The originals are put back when the job ends, so other scripts see the plain api.
function wrapApi() {
	var saved = {}
	for (var name in api) {
		var fn = api[name]
		if (typeof fn !== 'function' || /^[A-Z]/.test(name)) {
			continue
		}
		saved[name] = fn
		;(function (name, fn) {
			api[name] = function () {
				try {
					return fn.apply(api, arguments)
				} catch (err) {
					if (err && err.stack) {
						throw err
					}
					throw new Error(String(err && err.message ? err.message : err) + ' (in api.' + name + '(' + describeArgs(arguments) + '))')
				}
			}
		})(name, fn)
	}
	return function unwrap() {
		for (var name in saved) {
			api[name] = saved[name]
		}
	}
}

function captureConsole(logs) {
	var original = {}
	;['log', 'info', 'warn', 'error'].forEach(function (level) {
		original[level] = console[level]
		console[level] = function () {
			var parts = []
			for (var i = 0; i < arguments.length; i++) {
				var arg = arguments[i]
				try {
					parts.push(typeof arg === 'string' ? arg : JSON.stringify(arg))
				} catch (err) {
					parts.push(String(arg))
				}
			}
			if (logs.length < 500) {
				logs.push({ level: level, message: parts.join(' ') })
			}
			original[level].apply(console, arguments)
		}
	})
	return function restore() {
		for (var level in original) {
			console[level] = original[level]
		}
	}
}

// The helper library (global `cav`) is kept here between jobs and put on globalThis only
// while a cav job runs.
var helpers = null
var helpersVersion = ''
function preload(request, logs) {
	if (!request.preload) {
		return null
	}
	var want = String(request.preloadVersion || '')
	if (helpers && helpersVersion === want && want !== '') {
		globalThis.cav = helpers
		return null
	}
	var src = String(api.readFromFile(String(request.preload)))
	;(0, eval)(src) // indirect eval: runs in global scope and defines globalThis.cav
	helpers = globalThis.cav
	helpersVersion = want
	logs.push({ level: 'info', message: 'cav: loaded helpers ' + want })
	return want
}

// Best effort only: code can still reach these through saved references.
var RESTRICTED_APIS = ['runProcess', 'runDetachedProcess']

function restrict(on) {
	var saved = {}
	if (!on) {
		return function () {}
	}
	RESTRICTED_APIS.forEach(function (name) {
		saved[name] = api[name]
		api[name] = function () {
			throw new Error('cav-bridge: api.' + name + ' is disabled for relayed jobs')
		}
	})
	return function () {
		for (var name in saved) {
			api[name] = saved[name]
		}
	}
}

function execute(request) {
	var logs = []
	var restore = captureConsole(logs)
	var unwrap = wrapApi()
	var unrestrict = restrict(!!request.restricted)
	var started = Date.now()
	var response = { type: 'result', id: String(request.id), ok: true, value: null, logs: logs, error: null }
	try {
		preload(request, logs)
		var code = request.file ? String(api.readFromFile(String(request.file))) : String(request.code)
		// Indirect eval: job code runs in global scope and cannot see the bridge's variables.
		var job = (0, eval)('(function() {\n' + code + '\n})')
		response.value = jsonSafe(job())
	} catch (err) {
		response.ok = false
		response.error = {
			message: String(err && err.message ? err.message : err),
			stack: String(err && err.stack ? err.stack : ''),
			where: errorLine(err),
		}
	} finally {
		unrestrict()
		unwrap()
		restore()
		if (helpers && globalThis.cav === helpers) {
			delete globalThis.cav
		}
	}
	response.ms = Date.now() - started
	return response
}

var processingPosts = false
function BridgeCallbacks() {
	// onPost must be an own property: Cavalry's native side may not resolve prototype methods.
	this.onPost = function () {
  if (processingPosts) return
  processingPosts = true
  try {
		while (server.postCount() > 0) {
			var post = server.getNextPost()
			var request
			try {
				request = JSON.parse(post.result)
			} catch (err) {
				console.error('cav-bridge: request was not valid JSON')
				continue
			}
			if (request.token !== TOKEN) {
				console.error('cav-bridge: rejected a request with a missing or wrong token')
				continue
			}
			if (!request.id || !(request.code || request.file)) {
				console.error('cav-bridge: request needs `id` and `code` or `file`')
				continue
			}
            // Completed IDs are idempotent. An old bridge need not support this:
            // the CLI never re-posts an uncertain submission on any bridge version.
            if (!/^[A-Za-z0-9_-]+$/.test(String(request.id))) continue
            var compatible = request.protocol === undefined || request.protocol === PROTOCOL
            var resultPath = JOBS_DIR + '/' + String(request.id) + '.json'
            try {
                if (compatible && api.filePathExists(resultPath)) {
                    var prior = JSON.parse(String(api.readFromFile(resultPath)))
                    if (prior.type === 'result' && prior.id === String(request.id)) {
                        server.setResultForGet(JSON.stringify(prior))
                        continue
                    }
                }
            } catch (err) {}
			var wanted = request.protocol === undefined ? 1 : request.protocol
			var response
			if (wanted !== PROTOCOL) {
				// An older or newer cav speaks another request format: do not guess, refuse.
				response = { type: 'result', id: String(request.id), ok: false, value: null, logs: [], ms: 0,
					error: { code: 'protocol', message: 'cav-bridge ' + BRIDGE_VERSION + ' speaks protocol ' + PROTOCOL + ', but this cav speaks protocol ' + wanted } }
			} else {
				server.setResultForGet(
					JSON.stringify({ type: 'running', id: String(request.id), startedAt: Date.now(), bridgeVersion: BRIDGE_VERSION, bridgeSession: SESSION_ID, protocol: PROTOCOL }),
				)
				response = execute(request)
				jobCount++
				lastJobAt = Date.now()
			}
			response.bridgeSession = SESSION_ID
   response.bridgeVersion = BRIDGE_VERSION
			response.protocol = PROTOCOL
			var text = JSON.stringify(response)
			try {
				api.writeToFile(JOBS_DIR + '/' + String(request.id).replace(/[^A-Za-z0-9_-]/g, '') + '.json', text)
			} catch (err) {
				// The GET payload still carries the result.
			}
			server.setResultForGet(text)
		}
  } finally { processingPosts = false }
	}
}

server.setResultForGet(
	JSON.stringify({
		type: 'hello',
		bridge: 'cav-bridge',
  bridgeSession: SESSION_ID,
		bridgeVersion: BRIDGE_VERSION,
		protocol: PROTOCOL,
		cavalryVersion: api.getCavalryVersion(),
	}),
)
server.listen(HOST, PORT)
var callbacks = new BridgeCallbacks()
server.addCallbackObject(callbacks)
// setRealtime() breaks post polling on Cavalry 2.7.2 (reported by cavalry-mcp);
// setHighFrequency() polls about once per second.
server.setHighFrequency()

// A faster poll through api.Timer, so short jobs do not wait up to a second.
// The WebServer callback above stays as the fallback.
var busy = false
function FastPoll() {
	this.onTimeout = function () {
		if (busy || server.postCount() === 0) {
			return
		}
		busy = true
		try {
			callbacks.onPost()
		} finally {
			busy = false
		}
	}
}
var fastPoll = null
try {
	fastPoll = new api.Timer(new FastPoll())
	fastPoll.setRepeating(true)
	fastPoll.setInterval(50)
	fastPoll.start()
} catch (err) {
	console.warn('cav-bridge: fast polling unavailable, using 1 s polling: ' + err)
}

var title = new ui.Label('cav-bridge v' + BRIDGE_VERSION)
title.setAlignment(1)
var status = new ui.Label('Listening on http://' + HOST + ':' + PORT)
status.setAlignment(1)
var hint = new ui.Label('Keep this window open while you use cav.')
hint.setAlignment(1)
var layout = new ui.VLayout()
layout.addStretch()
layout.add(title, status, hint)
layout.addStretch()
ui.setTitle('cav-bridge')
ui.add(layout)
ui.show()

console.log('cav-bridge v' + BRIDGE_VERSION + ' listening on ' + HOST + ':' + PORT)
})()
