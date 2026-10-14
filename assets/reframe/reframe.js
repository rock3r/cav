// cav scene reframe: make a copy of a comp at 9:16 or 1:1, with the key content fitted
// into the middle half of the new frame. The ORIGINAL comp is never changed.
// Input: opts = {source, to: [w, h], name, key: [names or ids], scale, samples, minText,
// dryRun, budgetMs}. source is a comp id or unique name ('' means the active comp).
// One job measures the original and then builds the copy, so nothing changes in between.
// Coordinates are the comp's: the centre is (0, 0) and +y is up, as in cav.comp().

var MAX_LAYERS = 400
var FRAME_TOLERANCE = 1 // pixels of slack when a box is compared with a zone
var warnings = []

// targetSize is the size of the copy, from the short side of the source. 9:16 is
// short x round(short * 16 / 9) rounded to an even number; 1:1 is short x short.
function targetSize(sw, sh, to) {
	var short = Math.min(sw, sh)
	if (to[0] === 1 && to[1] === 1) return [short, short]
	if (to[0] === 9 && to[1] === 16) {
		var h = Math.round((short * 16) / 9)
		return [short, h % 2 ? h + 1 : h]
	}
	throw new Error('reframe supports 9:16 and 1:1 only')
}

function unionBox(boxes) {
	if (!boxes.length) return null
	var u = { l: Infinity, r: -Infinity, t: -Infinity, b: Infinity }
	boxes.forEach(function (x) {
		u.l = Math.min(u.l, x.l)
		u.r = Math.max(u.r, x.r)
		u.t = Math.max(u.t, x.t)
		u.b = Math.min(u.b, x.b)
	})
	return u
}

function toBox(b) {
	return { l: b.left, r: b.right, t: b.top, b: b.bottom }
}

function clip(x, W, H) {
	var c = { l: Math.max(x.l, -W / 2), r: Math.min(x.r, W / 2), t: Math.min(x.t, H / 2), b: Math.max(x.b, -H / 2) }
	return c.r > c.l && c.t > c.b ? c : null
}

function zone(z) {
	return { l: z.left, r: z.right, t: z.top, b: z.bottom }
}

function sampleFrames(start, end, n) {
	var out = []
	if (n <= 1 || end <= start) return [start]
	for (var i = 0; i < n; i++) {
		var f = Math.round(start + ((end - start) * i) / (n - 1))
		if (out.indexOf(f) < 0) out.push(f)
	}
	return out
}

function resolveComp(selector) {
	var comps = api.getComps()
	if (!selector) return api.getActiveComp()
	var m = comps.filter(function (c) {
		return c === selector
	})
	if (!m.length)
		m = comps.filter(function (c) {
			return api.getNiceName(c) === selector
		})
	if (m.length !== 1) throw new Error('composition must resolve to one ID: ' + selector)
	return m[0]
}

var originalComp = api.getActiveComp(),
	originalFrame = api.getFrame(),
	newComp = null,
	began = Date.now(),
	budget = opts.budgetMs || 20000

function restoreView() {
	try {
		api.setActiveComp(originalComp)
		api.setFrame(originalFrame)
	} catch (e) {}
}

function reframe() {
	var sourceComp = resolveComp(opts.source),
		sourceName = api.getNiceName(sourceComp),
		res = api.get(sourceComp, 'resolution'),
		Ws = res.x,
		Hs = res.y,
		start = api.get(sourceComp, 'startFrame'),
		end = api.get(sourceComp, 'endFrame')
	var target = targetSize(Ws, Hs, opts.to),
		Wt = target[0],
		Ht = target[1]
	if (Wt === Ws && Ht === Hs) throw new Error('the comp is already ' + opts.to[0] + ':' + opts.to[1])
	var newName = opts.name || sourceName + ' ' + opts.to[0] + 'x' + opts.to[1]
	// Read-only, so a dry run reports the clash too.
	api.getComps().forEach(function (c) {
		if (api.getNiceName(c) === newName) throw new Error('a comp named "' + newName + '" already exists: pass --name with a different name')
	})

	// 1. Measure the original. Nothing is written.
	api.setActiveComp(sourceComp)
	var nodes = {},
		order = [],
		topIds = api.getCompLayers(true),
		skipped = 0,
		coverageComplete = true
	// --key selectors are resolved against every layer of the comp, not the measured part, so a
	// big decorative group in front cannot hide them. Key subtrees (and the layers above them)
	// have their own cap, so they are measured even when the rest hits its cap.
	var keySel = {},
		needed = {},
		keyCount = 0
	if (opts.key && opts.key.length) {
		var everyId = api.getCompLayers(false)
		opts.key.forEach(function (sel) {
			var m = everyId.filter(function (id) {
				return id === sel
			})
			if (!m.length)
				m = everyId.filter(function (id) {
					return api.getNiceName(id) === sel
				})
			if (m.length !== 1) throw new Error(m.length ? 'layer name is not unique in the comp: ' + sel + ' (use its id)' : 'no layer named or numbered "' + sel + '" in the comp')
			keySel[m[0]] = true
			for (var p = m[0]; p && p !== sourceComp; p = api.getParent(p)) needed[p] = true
		})
	}
	function addNode(id, parent, inKey) {
		// Past the cap only the top-level layers are still recorded, so every one of them is
		// paired and transformed in the copy; nothing under them is measured.
		inKey = inKey || !!keySel[id]
		var isNeeded = inKey || !!needed[id]
		var over = isNeeded ? keyCount >= MAX_LAYERS : order.length - keyCount >= MAX_LAYERS
		if (over) {
			skipped++
			coverageComplete = false
			if (parent !== null) return null
		}
		var n = { id: id, type: api.getLayerType(id), name: api.getNiceName(id), parent: parent, children: [], childIds: [], inF: start, outF: end }
		try {
			n.inF = api.getInFrame(id)
			n.outF = api.getOutFrame(id)
		} catch (e) {}
		nodes[id] = n
		n.childIds = api.getChildren(id) || []
		if (over) return n
		n.measured = true
		order.push(id)
		if (isNeeded) keyCount++
		n.childIds.forEach(function (c) {
			var cn = addNode(c, id, inKey)
			if (cn) n.children.push(cn.id)
		})
		return n
	}
	// Top-level nodes with no position and scale (a sound node from cav.sound, a noise node
	// from cav.wiggle, ...) are kept exactly as they are: not measured, not transformed.
	function hasTransform(id) {
		try {
			var p = api.get(id, 'position'),
				s = api.get(id, 'scale')
			return !!p && !!s && typeof p.x === 'number' && typeof s.x === 'number'
		} catch (e) {
			return false
		}
	}
	var tops = [],
		kept = []
	topIds.forEach(function (id) {
		var n = addNode(id, null)
		if (!n) return
		if (hasTransform(id)) tops.push(n.id)
		else kept.push({ id: n.id, name: n.name, type: n.type })
	})
	var keptIds = {}
	kept.forEach(function (k) {
		keptIds[k.id] = true
	})
	function topOf(id) {
		while (nodes[id].parent) id = nodes[id].parent
		return id
	}
	var leaves = order.filter(function (id) {
		var n = nodes[id]
		return !keptIds[topOf(id)] && !n.children.length && !n.childIds.length && n.type !== 'group' && n.type !== 'null'
	})

	// Transform facts for the top-level layers: keyed or connected position and scale.
	var TRANSFORM_ATTRS = ['position', 'position.x', 'position.y', 'scale', 'scale.x', 'scale.y']
	tops.forEach(function (id) {
		var n = nodes[id],
			dynamic = false
		try {
			;(api.getAnimatedAttributes(id) || []).forEach(function (a) {
				if (/^(position|scale)/.test(a)) dynamic = true
			})
		} catch (e) {}
		TRANSFORM_ATTRS.forEach(function (a) {
			try {
				if (api.getInConnection(id, a)) dynamic = true
			} catch (e) {}
		})
		n.dynamic = dynamic
		n.position = api.get(id, 'position')
		n.scale = api.get(id, 'scale')
	})

	// Sample the frames. A leaf counts on a frame where it is visible: inside its own and its
	// ancestors' in and out frames, not hidden, with opacity above 0.
	var framesSkipped = false
	var frames = sampleFrames(start, end, opts.samples || 12)
	leaves.forEach(function (id) {
		nodes[id].looks = []
	})
	frames.forEach(function (f) {
		if (Date.now() - began > budget) {
			coverageComplete = false
			framesSkipped = true
			return
		}
		api.setFrame(f)
		leaves.forEach(function (id) {
			var p = id,
				op = 1,
				sc = 1
			while (p) {
				var n = nodes[p]
				if (f < n.inF || f > n.outF) return
				try {
					if (api.get(p, 'hidden')) return
					op *= api.get(p, 'opacity') / 100
					sc *= Math.abs(api.get(p, 'scale').y)
				} catch (e) {}
				p = n.parent
			}
			if (!(op > 0)) return
			var b
			try {
				b = api.getBoundingBox(id, true)
			} catch (e) {
				return
			}
			if (!b || !(b.width > 0 || b.height > 0)) return
			var look = { f: f, raw: toBox(b), sc: sc }
			var n0 = nodes[id]
			if (n0.type === 'textShape') {
				try {
					look.size = api.get(id, 'fontSize') * sc
				} catch (e) {}
			}
			n0.looks.push(look)
		})
	})
	var frameBox = { l: -Ws / 2, r: Ws / 2, t: Hs / 2, b: -Hs / 2 }

	// 2. Plan: key layers, backgrounds, the fit.
	var bgPattern = /^(bg|background|backdrop)/i
	var bgs = {}
	tops.forEach(function (id) {
		var n = nodes[id]
		if (n.type === 'compositionReference' || n.type === 'textShape' || n.type === 'cameraTransform') return
		if (bgPattern.test(n.name)) {
			bgs[id] = true
			return
		}
		if (n.children.length || n.childIds.length) return
		var boxes = (n.looks || [])
			.map(function (l) {
				return clip(l.raw, Ws, Hs)
			})
			.filter(Boolean)
		var u = unionBox(boxes)
		if (u && u.r - u.l >= 0.98 * Ws && u.t - u.b >= 0.98 * Hs) bgs[id] = true
	})
	var keyMode = 'rig',
		keyTops = []
	var keySet = {}
	if (opts.key && opts.key.length) {
		keyMode = 'explicit'
		Object.keys(keySel).forEach(function (id) {
			if (!nodes[id]) throw new Error('layer "' + id + '" could not be recorded')
			keySet[id] = true
		})
	} else {
		var rig = tops.filter(function (id) {
			return nodes[id].type === 'group' && nodes[id].name.toLowerCase() === 'rig'
		})
		if (rig.length) rig.forEach(function (id) {
			keySet[id] = true
		})
		else {
			keyMode = 'all'
			tops.forEach(function (id) {
				if (!bgs[id]) keySet[id] = true
			})
		}
	}
	// A layer named in --key is content, never a background, nor is the top-level layer it sits in.
	if (keyMode === 'explicit')
		Object.keys(keySet).forEach(function (id) {
			delete bgs[topOf(id)]
		})
	function underKey(id) {
		while (id) {
			if (keySet[id]) return true
			id = nodes[id].parent
		}
		return false
	}
	// The fit is only as good as the key measurement: every node under a key layer must have
	// been collected, and every sample frame measured.
	function subtreeMeasured(id) {
		var n = nodes[id]
		if (!n.measured || n.children.length !== n.childIds.length) return false
		return n.children.every(subtreeMeasured)
	}
	var keyComplete = !framesSkipped && Object.keys(keySet).every(subtreeMeasured)
	var keyBoxes = []
	leaves.forEach(function (id) {
		if (!underKey(id)) return
		nodes[id].looks.forEach(function (l) {
			var c = clip(l.raw, Ws, Hs)
			if (c) keyBoxes.push(c)
		})
	})
	var keyBox = unionBox(keyBoxes) || frameBox
	var zones = cav.safeArea(Wt, Ht),
		kz = zones.key
	var s = Math.min(kz.width / (keyBox.r - keyBox.l), kz.height / (keyBox.t - keyBox.b))
	if (opts.scale > 0) s = opts.scale
	else s = Math.min(s, 1)
	var kcx = (keyBox.l + keyBox.r) / 2,
		kcy = (keyBox.t + keyBox.b) / 2
	var tx = kz.centre[0] - s * kcx,
		ty = kz.centre[1] - s * kcy
	var cover = Math.max(Wt / Ws, Ht / Hs)

	function affineOf(id) {
		return bgs[topOf(id)] ? { k: cover, x: 0, y: 0 } : { k: s, x: tx, y: ty }
	}
	function mapBox(x, a) {
		return { l: a.k * x.l + a.x, r: a.k * x.r + a.x, t: a.k * x.t + a.y, b: a.k * x.b + a.y }
	}

	// Predicted findings: the measured boxes through the same transform. Nothing is measured
	// after the build: Cavalry 2.8.0 keeps stale world boxes in the run that creates a layer.
	var targetFrame = { l: -Wt / 2, r: Wt / 2, t: Ht / 2, b: -Ht / 2 },
		safe = zone(zones.safe),
		keyZone = zone(kz)
	var findings = []
	function find(kind, id, detail, fix, frame) {
		var list = findings.filter(function (x) {
			return x.kind === kind
		})
		if (list.length >= 20) return
		findings.push({ kind: kind, source: id, name: id ? nodes[id].name : undefined, frame: frame, detail: detail, fix: fix })
	}
	var partialKey = keyMode === 'explicit' || opts.scale > 0
	leaves.forEach(function (id) {
		var n = nodes[id]
		if (bgs[topOf(id)]) return
		var a = affineOf(id),
			isKey = underKey(id),
			off = null,
			clipped = null,
			unsafe = null,
			outside = null
		n.looks.forEach(function (l) {
			var x = mapBox(l.raw, a),
				over = x.l < targetFrame.l - FRAME_TOLERANCE || x.r > targetFrame.r + FRAME_TOLERANCE || x.t > targetFrame.t + FRAME_TOLERANCE || x.b < targetFrame.b - FRAME_TOLERANCE
			if (x.r <= targetFrame.l || x.l >= targetFrame.r || x.t <= targetFrame.b || x.b >= targetFrame.t) {
				off = off === null ? l.f : off
				return
			}
			if (over) clipped = clipped === null ? l.f : clipped
			else if ((isKey || n.type === 'textShape') && (x.l < safe.l - FRAME_TOLERANCE || x.r > safe.r + FRAME_TOLERANCE || x.t > safe.t + FRAME_TOLERANCE || x.b < safe.b - FRAME_TOLERANCE))
				unsafe = unsafe === null ? l.f : unsafe
			if (partialKey && (x.l < keyZone.l - FRAME_TOLERANCE || x.r > keyZone.r + FRAME_TOLERANCE || x.t > keyZone.t + FRAME_TOLERANCE || x.b < keyZone.b - FRAME_TOLERANCE))
				outside = outside === null ? l.f : outside
		})
		if (off !== null) find('offframe', id, 'would be completely outside the ' + Wt + 'x' + Ht + ' frame at frame ' + off, 'move it into the frame, or include its group in --key', off)
		else if (clipped !== null) find('clipped', id, 'would be cut off by the edge of the ' + Wt + 'x' + Ht + ' frame at frame ' + clipped, 'move it inward, or shrink it', clipped)
		else if (unsafe !== null) find('unsafe', id, 'would sit under the platform buttons or captions at frame ' + unsafe, 'move it inside the safe area (cav.safeArea().safe)', unsafe)
		if (outside !== null && off === null) find('outside-key', id, 'would sit outside the middle half at frame ' + outside, 'move it into the middle half, or add its group to --key', outside)
		if (n.type === 'textShape') {
			var sizes = n.looks.filter(function (l) {
				return l.size > 0
			})
			if (sizes.length) {
				// The smallest look; its size already includes the scale inherited from the layer and its parents.
				var small = sizes.reduce(function (a, b) {
					return b.size < a.size ? b : a
				})
				// --min-text is in pixels at 1080p, so the size in the copy is normalised to the short side.
				var px = small.size * a.k,
					norm = (px * 1080) / Math.min(Wt, Ht),
					needPx = Math.ceil((opts.minText * Math.min(Wt, Ht)) / 1080)
				if (norm < opts.minText) find('small-text', id, 'text would be ' + Math.round(norm) + ' px (at 1080p); below ' + opts.minText + ' px it is hard to read on a phone', 'make it at least ' + needPx + ' px tall in the ' + Wt + 'x' + Ht + ' copy (font size ' + Math.ceil(needPx / (a.k * small.sc)) + ' before the fit and inherited scales), or cut the text')
			}
		}
	})
	var shared = []
	order.forEach(function (id) {
		var n = nodes[id]
		if (n.type === 'compositionReference') {
			var ref = ''
			try {
				ref = api.getCompFromReference(id)
			} catch (e) {}
			var rn = ref ? api.getNiceName(ref) : n.name
			shared.push({ layer: id, comp: rn })
			find('shared-precomp', id, 'it shows the pre-comp "' + rn + '", which is shared with the original comp', 'edits inside "' + rn + '" also change the ' + Ws + 'x' + Hs + ' version; duplicate the pre-comp first')
		}
		if (n.type === 'cameraTransform') find('camera', id, 'the comp has a camera; reframe does not adjust it', 'check the shot through the new camera view')
	})
	var contentPct = Math.round((100 * s * (keyBox.t - keyBox.b)) / Ht)
	findings.push({ kind: 'space', detail: 'the content fills ' + contentPct + ' % of the height; the bands above and below the middle half are free for titles and captions', fix: 'place titles and captions in them (cav.safeArea())' })

	var plan = {
		source: { id: sourceComp, name: sourceName, w: Ws, h: Hs },
		target: { id: null, name: newName, w: Wt, h: Ht },
		zones: zones,
		key: {
			mode: keyMode,
			layers: Object.keys(keySet).map(function (id) {
				return { id: id, name: nodes[id].name }
			}),
			box: keyBox,
			coverage: keyBoxes.length,
		},
		scale: s,
		offset: [tx, ty],
		cover: cover,
		direct: [],
		wrapped: [],
		backgrounds: Object.keys(bgs).map(function (id) {
			return { id: id, name: nodes[id].name }
		}),
		findings: findings,
		coverageComplete: coverageComplete,
		skipped: skipped,
		kept: kept,
		warnings: warnings,
		dryRun: !!opts.dryRun,
	}
	tops.forEach(function (id) {
		var n = nodes[id]
		if (n.type === 'cameraTransform') return
		if (n.dynamic) plan.wrapped.push({ id: id, name: n.name, group: null })
		else plan.direct.push({ id: id, name: n.name })
	})
	if (!keyComplete) {
		var N = MAX_LAYERS
		if (opts.dryRun || opts.scale > 0)
			warnings.push('the key layers have more than ' + N + ' nodes or took too long to measure, so the fit comes from a partial box' + (opts.scale > 0 ? ' (the scale is yours; the centre is a guess)' : '; a real run needs --key with fewer layers, or --scale S'))
		else throw new Error('the key layers have more than ' + N + ' nodes or took too long to measure, so the fit would be a guess: pass --key with fewer layers, or --scale S to set the scale yourself')
	}
	if (opts.dryRun) {
		restoreView()
		return plan
	}

	// 3. Build the copy. The source is only read from here on.
	api.select([])
	var before = api.getComps()
	api.duplicate(sourceComp, false)
	var made = api.getComps().filter(function (c) {
		return before.indexOf(c) < 0
	})
	if (made.length !== 1) {
		made.forEach(function (c) {
			try {
				api.deleteLayer(c)
			} catch (e) {}
		})
		throw new Error('expected one new comp from the copy, found ' + made.length)
	}
	newComp = made[0]
	api.rename(newComp, newName)
	api.set(newComp, { resolution: [Wt, Ht] })
	api.setActiveComp(newComp)
	api.setFrame(start)

	// Pair each source layer with its copy by walking both trees in order.
	var copy = {}
	function pair(srcId, dstId) {
		var sn = nodes[srcId]
		if (api.getLayerType(dstId) !== sn.type || api.getNiceName(dstId) !== sn.name) throw new Error('the copy differs from the original at "' + sn.name + '"')
		copy[srcId] = dstId
		var kids = api.getChildren(dstId) || []
		if (kids.length !== sn.childIds.length) throw new Error('the copy differs from the original under "' + sn.name + '"')
		sn.children.forEach(function (c) {
			pair(c, kids[sn.childIds.indexOf(c)])
		})
	}
	var dstTops = api.getCompLayers(true)
	if (dstTops.length !== topIds.length) throw new Error('the copy has ' + dstTops.length + ' top-level layers, the original has ' + topIds.length)
	tops.forEach(function (id) {
		pair(id, dstTops[topIds.indexOf(id)])
	})

	tops.forEach(function (id) {
		var n = nodes[id]
		if (n.type === 'cameraTransform') return
		var a = affineOf(id),
			L = copy[id]
		if (n.dynamic) {
			// Keyed or connected transforms keep their values: a group around the layer carries the fit.
			api.select([])
			var g = api.create('group', 'reframe ' + n.name)
			api.select([])
			api.reorder(g, L)
			api.parent(L, g)
			api.set(g, { 'position.x': a.x, 'position.y': a.y, 'scale.x': a.k, 'scale.y': a.k })
			plan.wrapped.forEach(function (w) {
				if (w.id === id) w.group = g
			})
		} else {
			api.set(L, {
				'position.x': a.k * n.position.x + a.x,
				'position.y': a.k * n.position.y + a.y,
				'scale.x': a.k * n.scale.x,
				'scale.y': a.k * n.scale.y,
			})
		}
	})
	plan.direct.forEach(function (d) {
		d.layer = copy[d.id]
	})
	plan.wrapped.forEach(function (w) {
		w.layer = copy[w.id]
	})
	findings.forEach(function (f) {
		if (f.source && copy[f.source]) f.layer = copy[f.source]
	})

	// Guides do not render: they mark the middle half and the safe area on the new comp.
	;[
		[kz.top, 'top of the middle half'],
		[kz.bottom, 'bottom of the middle half'],
		[zones.safe.top, 'top of the safe area'],
		[zones.safe.bottom, 'bottom of the safe area'],
	].forEach(function (g) {
		try {
			api.addGuide(newComp, false, g[0])
		} catch (e) {
			warnings.push('could not add a guide for the ' + g[1] + ': ' + e.message)
		}
	})
	api.select([])
	plan.target.id = newComp
	return plan
}

try {
	return reframe()
} catch (e) {
	restoreView()
	if (newComp) {
		try {
			api.deleteLayer(newComp)
		} catch (x) {}
	}
	throw e
}
