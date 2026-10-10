// cav helper library, preloaded by `cav run` into the global `cav`.
// Lines that start with "//@" form the reference that `cav helpers` prints.
// Grown from the helpers of the Spectre promo build (Cavalry 2.7.2); every helper here
// is covered by tests in helpers/test.
/* global api, cavalry, globalThis */

;(function () {
	var cav = {}
	cav.VERSION = '1.3.0'

	//@ # cav helpers (global `cav`, loaded before every `cav run`)
	//@ Coordinates: (0,0) is the comp centre, +x right, +y UP. Frames are integers.
	//@ All create helpers clear the selection first (api.create nests new layers
	//@ next to the selection otherwise) and re-apply transforms after parenting.
	//@
	//@ ## Scene and time
	//@ cav.comp()                    -> {id, width, height, fps, start, end, left, right, top, bottom}
	//@ cav.clear()                   delete every layer in the active comp
	//@ cav.sec(seconds)              -> frame number at the comp fps
	//@ cav.beats(bpm, {offset, perBar}) -> grid: g.beat(n), g.bar(n), g.every(n) frames, g.frames (per beat)
	//@                                  offset is the frame of beat 0 (default 0); perBar defaults to 4.
	//@ cav.find(name)                -> layer id with that name (first match) or null

	function fail(msg) {
		throw new Error('cav: ' + msg)
	}

	cav.comp = function () {
		var id = api.getActiveComp()
		var res = api.get(id, 'resolution')
		return {
			id: id,
			width: res.x,
			height: res.y,
			fps: api.get(id, 'fps'),
			start: api.get(id, 'startFrame'),
			end: api.get(id, 'endFrame'),
			left: -res.x / 2,
			right: res.x / 2,
			top: res.y / 2,
			bottom: -res.y / 2,
		}
	}

	cav.clear = function () {
		api.select([])
		var ids = api.getCompLayers(true)
		for (var i = 0; i < ids.length; i++) {
			if (api.layerExists(ids[i])) api.deleteLayer(ids[i])
		}
		return ids.length
	}

	cav.sec = function (s) {
		return Math.round(s * api.get(api.getActiveComp(), 'fps'))
	}

	cav.beats = function (bpm, o) {
		o = o || {}
		if (!(bpm > 0)) fail('cav.beats(bpm): bpm must be a positive number')
		var fps = o.fps || api.get(api.getActiveComp(), 'fps')
		var per = (60 / bpm) * fps
		var offset = o.offset || 0
		var perBar = o.perBar || 4
		return {
			bpm: bpm,
			fps: fps,
			frames: per,
			perBar: perBar,
			beat: function (n) {
				return Math.round(offset + n * per)
			},
			bar: function (n) {
				return Math.round(offset + n * perBar * per)
			},
			every: function (n) {
				return Math.round(n * per)
			},
		}
	}

	// Convenience alias: output shows up in cav run.
	cav.log = function () {
		console.log.apply(console, arguments)
	}

	cav.find = function (name) {
		var ids = api.getCompLayers(false)
		for (var i = 0; i < ids.length; i++) {
			if (api.getNiceName(ids[i]) === name) return ids[i]
		}
		return null
	}

	//@
	//@ ## Easing (names for cav.key / cav.tween / cav.anim)
	//@ linear, in, out, inOut (cubic), inQuad, outQuad, inOutQuad, inCubic, outCubic, inOutCubic,
	//@ inQuart, outQuart, inOutQuart, inQuint, outQuint, inOutQuint, inExpo, outExpo, inOutExpo,
	//@ inBack, outBack, inOutBack, outBackBig, outElastic, outBounce, spring, springSoft,
	//@ plus Cavalry's Magic Easing names (SlowIn, SlowOut, SlowInSlowOut, OvershootOut, ...).
	//@ An ease on a key shapes the segment that STARTS at that key.
	//@ Anticipation: 'inBack' winds up before moving. Landing: 'outBack' / 'spring' overshoot.
	cav.E = {
		in: 'x*x*x',
		out: '1 - pow(1-x,3)',
		inOut: 'x<0.5 ? 4*x*x*x : 1 - pow(-2*x+2,3)/2',
		inQuad: 'x*x',
		outQuad: '1 - (1-x)*(1-x)',
		inOutQuad: 'x<0.5 ? 2*x*x : 1 - pow(-2*x+2,2)/2',
		inCubic: 'x*x*x',
		outCubic: '1 - pow(1-x,3)',
		inOutCubic: 'x<0.5 ? 4*x*x*x : 1 - pow(-2*x+2,3)/2',
		inQuart: 'pow(x,4)',
		outQuart: '1 - pow(1-x,4)',
		inOutQuart: 'x<0.5 ? 8*pow(x,4) : 1 - pow(-2*x+2,4)/2',
		inQuint: 'pow(x,5)',
		outQuint: '1 - pow(1-x,5)',
		inOutQuint: 'x<0.5 ? 16*pow(x,5) : 1 - pow(-2*x+2,5)/2',
		inExpo: 'x<=0 ? 0 : pow(2,10*x-10)',
		outExpo: 'x>=1 ? 1 : 1 - pow(2,-10*x)',
		inOutExpo: 'x<=0 ? 0 : (x>=1 ? 1 : (x<0.5 ? pow(2,20*x-10)/2 : (2-pow(2,-20*x+10))/2))',
		inBack: '2.70158*x*x*x - 1.70158*x*x',
		outBack: '1 + 2.70158*pow(x-1,3) + 1.70158*pow(x-1,2)',
		inOutBack: 'x<0.5 ? (pow(2*x,2)*((2.5949+1)*2*x-2.5949))/2 : (pow(2*x-2,2)*((2.5949+1)*(x*2-2)+2.5949)+2)/2',
		outBackBig: '1 + 4*pow(x-1,3) + 3*pow(x-1,2)',
		outElastic: 'x<=0 ? 0 : (x>=1 ? 1 : pow(2,-10*x)*sin((x*10-0.75)*2.0944)+1)',
		outBounce:
			'x<1/2.75 ? 7.5625*x*x : (x<2/2.75 ? 7.5625*(x-1.5/2.75)*(x-1.5/2.75)+0.75 : (x<2.5/2.75 ? 7.5625*(x-2.25/2.75)*(x-2.25/2.75)+0.9375 : 7.5625*(x-2.625/2.75)*(x-2.625/2.75)+0.984375))',
		spring: 'x>=1 ? 1 : 1 - exp(-7*x)*cos(14*x)',
		springSoft: 'x>=1 ? 1 : 1 - exp(-5*x)*cos(9*x)',
	}
	var MAGIC = [
		'None', 'SlowIn', 'SlowOut', 'SlowInSlowOut', 'VerySlowIn', 'VerySlowOut', 'VerySlowInVerySlowOut',
		'SpringIn', 'SpringOut', 'SpringInSpringOut', 'SmallSpringIn', 'SmallSpringOut', 'SmallSpringInSmallSpringOut',
		'AnticipateIn', 'OvershootOut', 'AnticipateInOvershootOut', 'BounceIn', 'BounceOut', 'BounceInBounceOut',
	]

	function checkEase(name) {
		if (!name || name === 'linear') return
		if (cav.E[name] || MAGIC.indexOf(name) >= 0) return
		var names = []
		for (var k in cav.E) names.push(k)
		fail('unknown easing "' + name + '". Use one of: linear, ' + names.join(', ') + ' (or a Magic Easing name like SlowOut)')
	}

	function applyEase(layer, attr, frame, name) {
		if (!name || name === 'linear') return
		if (cav.E[name]) api.magicEasing(layer, attr, frame, 'Custom', cav.E[name])
		else api.magicEasing(layer, attr, frame, name)
	}

	//@
	//@ ## Colours
	//@ Colours are '#rrggbb' strings. cav.rgb('#ff8800') -> [255,136,0]. cav.mix(a, b, t) blends two hex colours.
	cav.rgb = function (h) {
		if (typeof h !== 'string' || !/^#?[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$/.test(h)) fail('colour must look like "#rrggbb", got ' + JSON.stringify(h))
		h = h.replace('#', '')
		return [parseInt(h.substr(0, 2), 16), parseInt(h.substr(2, 2), 16), parseInt(h.substr(4, 2), 16)]
	}
	cav.mix = function (a, b, t) {
		var x = cav.rgb(a), y = cav.rgb(b)
		function h(n) {
			var s = Math.round(n).toString(16)
			return s.length < 2 ? '0' + s : s
		}
		return '#' + h(x[0] + (y[0] - x[0]) * t) + h(x[1] + (y[1] - x[1]) * t) + h(x[2] + (y[2] - x[2]) * t)
	}

	//@
	//@ ## Keyframes
	//@ cav.key(layer, attr, [[frame, value, ease?], ...])
	//@     attr: any attribute path. Shortcuts: 'position' and 'scale' take [x, y]
	//@     ('scale' also takes one number); colour attributes ('fill', 'material.materialColor',
	//@     'stroke.strokeColor') take '#rrggbb' and are keyed per channel for you.
	//@     'fill' = 'material.materialColor', 'strokeColor' = 'stroke.strokeColor'.
	//@     Scale is a multiplier: 1 = 100 %. Opacity is 0..100. Rotation is in degrees
	//@     ('rotation' means rotation.z; 'rotation.y' spins 2.5D layers).
	//@ cav.tween(layer, attr, f0, f1, from, to, ease?)  two keys. Repeated tweens on one
	//@     attribute HOLD between them (a key is placed at each tween's start), so there is no drift.
	//@ Keys that are not tweened interpolate across gaps: add a hold key before the next move.
	var SPLIT = {
		position: ['position.x', 'position.y'],
		scale: ['scale.x', 'scale.y'],
		pivot: ['pivot.x', 'pivot.y'],
		skew: ['skew.x', 'skew.y'],
		'generator.dimensions': ['generator.dimensions.x', 'generator.dimensions.y'],
		'generator.radius': ['generator.radius.x', 'generator.radius.y'],
	}
	var ALIAS = { fill: 'material.materialColor', color: 'material.materialColor', strokeColor: 'stroke.strokeColor', 'stroke.color': 'stroke.strokeColor', x: 'position.x', y: 'position.y', rotate: 'rotation.z', rotation: 'rotation.z' }
	cav.ALIAS = ALIAS

	function isColorAttr(attr) {
		return /(^|\.)(materialColor|strokeColor|backgroundColor|color|Color)$/.test(attr)
	}

	// Layer arguments may be an id string, or an object from cav.glyphs / cav.duplicator-style
	// helpers that carries its layer as .group or .id.
	function lid(layer, fn) {
		if (layer && typeof layer === 'object') {
			if (typeof layer.group === 'string') return layer.group
			if (typeof layer.id === 'string') return layer.id
		}
		if (typeof layer !== 'string' || !api.layerExists(layer)) fail(fn + ': layer ' + JSON.stringify(layer) + ' does not exist (use the id a create helper returned, e.g. "basicShape#3")')
		return layer
	}
	cav.id = function (layer) {
		return lid(layer, 'cav.id')
	}

	function checkLayer(layer, fn) {
		return lid(layer, fn)
	}

	cav.key = function (layer, attr, keys) {
		layer = checkLayer(layer, 'cav.key')
		attr = ALIAS[attr] || attr
		if (!keys || !keys.length || !Array.isArray(keys[0])) fail('cav.key(layer, attr, keys): keys must be [[frame, value, ease?], ...]')
		keys.forEach(function (k) {
			checkEase(k[2])
			if (typeof k[0] !== 'number' || isNaN(k[0])) fail('cav.key: frame must be a number, got ' + JSON.stringify(k[0]))
		})
		if (isColorAttr(attr) && typeof keys[0][1] === 'string') {
			;['r', 'g', 'b'].forEach(function (ch, ci) {
				cav.key(
					layer,
					attr + '.' + ch,
					keys.map(function (k) {
						return [k[0], cav.rgb(k[1])[ci], k[2]]
					}),
				)
			})
			return layer
		}
		var parts = SPLIT[attr]
		keys.forEach(function (k) {
			var d = {}
			var v = k[1]
			if (parts && Array.isArray(v)) {
				d[parts[0]] = v[0]
				d[parts[1]] = v[1]
			} else if (attr === 'scale' && typeof v === 'number') {
				d['scale.x'] = v
				d['scale.y'] = v
			} else if (parts) {
				fail('cav.key: ' + attr + ' needs [x, y] values' + (attr === 'scale' ? ' or one number' : '') + ', got ' + JSON.stringify(v))
			} else {
				d[attr] = v
			}
			api.keyframe(layer, Math.round(k[0]), d)
		})
		keys.forEach(function (k) {
			if (!k[2]) return
			;(parts || [attr]).forEach(function (a) {
				applyEase(layer, a, Math.round(k[0]), k[2])
			})
		})
		return layer
	}

	cav.tween = function (layer, attr, f0, f1, from, to, ease) {
		if (f1 <= f0) fail('cav.tween: f1 (' + f1 + ') must be after f0 (' + f0 + ')')
		return cav.key(layer, attr, [
			[f0, from, ease],
			[f1, to],
		])
	}

	//@ cav.anim(layer, {attr: [[frame, value, ease?], ...], ...})   several attributes at once
	cav.anim = function (layer, spec) {
		for (var a in spec) cav.key(layer, a, spec[a])
		return layer
	}

	//@
	//@ ## Creating layers   (every helper returns the layer id)
	//@ Common options o: {parent, x, y, scale, rotation, opacity, pivot:[x,y], fill:'#hex',
	//@   stroke:{color, width, cap, dash:[on,off], trim:true}, noFill:true, in, out, name}
	//@   in/out: first and last visible frame. Children of a group are hidden outside
	//@   the group's in/out, which makes clean scene cuts.
	//@ cav.group(name, o)
	//@ cav.rect(name, w, h, o)            o.radius: corner radius
	//@ cav.circle(name, r, o) / cav.ellipse(name, rx, ry, o)
	//@ cav.polygon(name, sides, r, o) / cav.star(name, points, r, o)   o.inner: inner radius
	//@ cav.line(name, [x1,y1], [x2,y2], o)   a stroked path (default stroke white 4 px)
	//@ cav.path(name, [[x,y], ...], o)     o.closed, o.smooth; or pass function(p){p.moveTo(..)...}
	//@ cav.text(name, string, size, o)     o.font ('Inter'), o.style ('Bold'), o.color, o.align
	//@     ('center' | 'left' | 'right'), o.valign ('center' | 'top' | 'bottom'), o.spacing (letter),
	//@     o.lineSpacing. A missing font falls back to Inter/Helvetica with a warning.
	//@ cav.image(path, name, o)            footage layer at native pixel size
	//@ cav.plane(name, color, o)           a rectangle that covers the whole comp
	//@ cav.create(type, name, o)           any layer type (see `cav types`), e.g. 'duplicator', 'stagger'
	//@ cav.set(layer, o)                   apply the common options to an existing layer
	//@ cav.attr(layer, {attrPath: value})  plain api.set with a clear error on bad paths
	cav.create = function (type, name, o) {
		api.select([])
		var l
		try {
			l = api.create(type, name || type)
		} catch (e) {
			fail('cannot create layer type "' + type + '": ' + e.message + '. List valid types with `cav types <word>`.')
		}
		api.select([])
		if (!l) fail('cannot create layer type "' + type + '". List valid types with `cav types <word>`.')
		return o ? cav.set(l, o) : l
	}

	function primitive(type, name, o) {
		api.select([])
		var l = api.primitive(type, name || type)
		api.select([])
		return l
	}

	var PAIR_TYPES = { double2: 1, int2: 1 }
	cav.attr = function (layer, d) {
		layer = checkLayer(layer, 'cav.attr')
		var out = {}
		for (var k in d) {
			var key = ALIAS[k] || k
			var v = d[k]
			// A single number on a two-value attribute sets it to (0, 0) without an error.
			if (typeof v === 'number') {
				try {
					var def = api.getAttributeDefinition(layer, key)
					if (def && PAIR_TYPES[def.type]) v = [v, v]
				} catch (e) {}
			}
			out[key] = v
		}
		try {
			api.set(layer, out)
		} catch (e) {
			fail('cav.attr(' + layer + '): ' + e.message + '. List attributes with `cav layer ' + layer + ' --attrs`.')
		}
		return layer
	}

	var ALIGN = { left: 0, center: 1, centre: 1, right: 2 }
	var VALIGN = { top: 0, center: 1, centre: 1, bottom: 2 }

	cav.set = function (l, o) {
		o = o || {}
		l = checkLayer(l, 'cav.set')
		if (o.name) api.rename(l, o.name)
		if (o.parent) {
			o.parent = checkLayer(o.parent, 'cav.set parent')
			api.parent(l, o.parent)
			// api.parent keeps the world transform; reset the local transform.
			if (o.x === undefined) o.x = 0
			if (o.y === undefined) o.y = 0
			if (o.scale === undefined) o.scale = 1
			if (o.rotation === undefined) o.rotation = 0
		}
		var d = {}
		if (o.x !== undefined || o.y !== undefined) {
			d['position.x'] = o.x || 0
			d['position.y'] = o.y || 0
		}
		if (o.scale !== undefined) {
			var s = Array.isArray(o.scale) ? o.scale : [o.scale, o.scale]
			d['scale.x'] = s[0]
			d['scale.y'] = s[1]
		}
		if (o.rotation !== undefined) d.rotation = o.rotation
		if (o.opacity !== undefined) d.opacity = o.opacity
		if (o.pivot) {
			d['pivot.x'] = o.pivot[0]
			d['pivot.y'] = o.pivot[1]
		}
		if (o.fill) d['material.materialColor'] = o.fill
		api.set(l, d)
		if (o.noFill) api.setFill(l, false)
		if (o.stroke) {
			api.setStroke(l, true)
			var st = o.stroke
			var sd = { 'stroke.strokeColor': st.color || '#ffffff', 'stroke.width': st.width === undefined ? 2 : st.width }
			if (st.cap !== undefined) sd['stroke.capStyle'] = st.cap === 'round' ? 1 : st.cap === 'square' ? 2 : st.cap
			// dashPattern is a string such as "12, 8".
			if (st.dash) sd['stroke.dashPattern'] = Array.isArray(st.dash) ? st.dash.join(', ') : String(st.dash)
			if (st.trim) sd['stroke.trim'] = true
			api.set(l, sd)
		}
		if (o.in !== undefined) api.setInFrame(l, o.in)
		if (o.out !== undefined) api.setOutFrame(l, o.out)
		return l
	}

	cav.group = function (name, o) {
		return cav.set(cav.create('group', name), o)
	}

	cav.rect = function (name, w, h, o) {
		o = o || {}
		var l = primitive('rectangle', name)
		api.set(l, { 'generator.dimensions': [w, h], 'generator.cornerRadius': o.radius || 0, 'material.materialColor': o.fill || '#ffffff' })
		return cav.set(l, o)
	}

	cav.ellipse = function (name, rx, ry, o) {
		o = o || {}
		var l = primitive('ellipse', name)
		api.set(l, { 'generator.radius': [rx, ry === undefined ? rx : ry], 'material.materialColor': o.fill || '#ffffff' })
		return cav.set(l, o)
	}

	cav.circle = function (name, r, o) {
		return cav.ellipse(name, r, r, o)
	}

	cav.polygon = function (name, sides, r, o) {
		o = o || {}
		var l = primitive('polygon', name)
		// generator.radius is a double2: one number would silently set it to (0, 0).
		api.set(l, { 'generator.sides': sides, 'generator.radius': [r, r], 'material.materialColor': o.fill || '#ffffff' })
		return cav.set(l, o)
	}

	cav.star = function (name, points, r, o) {
		o = o || {}
		var l = primitive('star', name)
		api.set(l, {
			'generator.sides': points,
			'generator.radius': r,
			'generator.useInnerRadius': true,
			'generator.innerRadius': o.inner || r * 0.45,
			'material.materialColor': o.fill || '#ffffff',
		})
		return cav.set(l, o)
	}

	cav.plane = function (name, color, o) {
		var c = cav.comp()
		o = o || {}
		o.fill = color || o.fill || '#ffffff'
		return cav.rect(name, c.width + 4, c.height + 4, o)
	}

	cav.path = function (name, pts, o) {
		o = o || {}
		var p = new cavalry.Path()
		if (typeof pts === 'function') {
			pts(p)
		} else {
			if (!pts || pts.length < 2) fail('cav.path needs at least 2 points [[x,y], ...]')
			p.moveTo(pts[0][0], pts[0][1])
			for (var i = 1; i < pts.length; i++) {
				if (o.smooth && i < pts.length - 1) {
					var mx = (pts[i][0] + pts[i + 1][0]) / 2, my = (pts[i][1] + pts[i + 1][1]) / 2
					p.quadTo(pts[i][0], pts[i][1], mx, my)
				} else {
					p.lineTo(pts[i][0], pts[i][1])
				}
			}
			if (o.closed) p.close()
		}
		api.select([])
		var l = api.createEditable(p, name)
		api.select([])
		if (!o.closed && !o.fill && typeof pts !== 'function') o.noFill = true
		if (o.fill) api.set(l, { 'material.materialColor': o.fill })
		cav.set(l, o)
		return l
	}

	cav.line = function (name, a, b, o) {
		o = o || {}
		if (!o.stroke) o.stroke = { color: '#ffffff', width: 4 }
		o.noFill = true
		return cav.path(name, [a, b], o)
	}

	var FALLBACK_FONTS = ['Inter', 'Helvetica Neue', 'Helvetica', 'Arial', 'Segoe UI', 'Lato']
	cav.font = function (family, style) {
		style = style || 'Regular'
		if (cavalry.fontExists(family, style)) return { font: family, style: style }
		if (cavalry.fontExists(family, 'Regular')) {
			console.warn('cav: font style "' + family + ' ' + style + '" is missing; styles: ' + cavalry.getFontStyles(family).join(', '))
			return { font: family, style: 'Regular' }
		}
		for (var i = 0; i < FALLBACK_FONTS.length; i++) {
			var f = FALLBACK_FONTS[i]
			if (cavalry.fontExists(f, style)) {
				console.warn('cav: font "' + family + '" is not installed; using ' + f + ' ' + style)
				return { font: f, style: style }
			}
		}
		return { font: 'Lato', style: 'Regular' }
	}

	cav.text = function (name, str, size, o) {
		o = o || {}
		var l = cav.create('textShape', name)
		var ha = typeof o.align === 'number' ? o.align : ALIGN[o.align || 'center']
		var va = typeof o.valign === 'number' ? o.valign : VALIGN[o.valign || 'center']
		if (ha === undefined) fail('cav.text align must be left, center or right')
		api.set(l, {
			text: String(str),
			fontSize: size || 48,
			autoWidth: true,
			autoHeight: true,
			'material.materialColor': o.color || o.fill || '#ffffff',
			horizontalAlignment: ha,
			verticalAlignment: va === undefined ? 1 : va,
		})
		api.set(l, { font: cav.font(o.font || 'Inter', o.style || 'Bold') })
		if (o.spacing !== undefined) api.set(l, { letterSpacing: o.spacing })
		if (o.lineSpacing !== undefined) api.set(l, { lineSpacing: o.lineSpacing })
		delete o.fill
		return cav.set(l, o)
	}

	cav.image = function (path, name, o) {
		if (!api.filePathExists(path)) fail('cav.image: file not found: ' + path + ' (use an absolute path)')
		api.select([])
		var f = api.addAssetToComp(api.loadAsset(path, false))
		if (Array.isArray(f)) f = f[0]
		api.select([])
		if (name) api.rename(f, name)
		return cav.set(f, o || {})
	}

	//@
	//@ ## Effects and structure
	//@ cav.filter(layer, type, attrs)   type e.g. 'blurFilter', 'glowFilter', 'dropShadowFilter'
	//@ cav.blur(layer, amount)          fast blur filter; returns the filter id (animate 'amount')
	//@ cav.mask(maskShape, target)      the shape masks the target (shape is hidden automatically)
	//@ cav.order(layer, below)          put layer directly below another layer in the stack
	//@ cav.connect(from, fromAttr, to, toAttr)   api.connect with a clear error
	cav.filter = function (layer, type, attrs) {
		layer = checkLayer(layer, 'cav.filter')
		var f = cav.create(type, api.getNiceName(layer) + ' ' + type)
		if (attrs) cav.attr(f, attrs)
		api.connect(f, 'id', layer, 'filters')
		return f
	}

	cav.blur = function (layer, amount) {
		return cav.filter(layer, 'blurFilter', { amount: [amount, amount] })
	}

	cav.mask = function (maskShape, target) {
		maskShape = checkLayer(maskShape, 'cav.mask')
		target = checkLayer(target, 'cav.mask')
		api.connect(maskShape, 'id', target, 'masks')
		return target
	}

	cav.order = function (layer, below) {
		api.reorder(layer, below)
		return layer
	}

	cav.connect = function (from, fromAttr, to, toAttr) {
		from = checkLayer(from, 'cav.connect')
		to = checkLayer(to, 'cav.connect')
		try {
			api.connect(from, fromAttr, to, toAttr)
		} catch (e) {
			fail('cannot connect ' + from + '.' + fromAttr + ' -> ' + to + '.' + toAttr + ': ' + e.message)
		}
		return to
	}

	//@
	//@ ## Motion recipes   (f = start frame; all return the layer)
	//@ cav.fadeIn(l, f, dur=10) / cav.fadeOut(l, f, dur=10)
	//@ cav.pop(l, f, {dur=14, from=0, to=1, ease='outBack'})        scale pop-in with overshoot
	//@ cav.slideIn(l, f, {dx=0, dy=-60, dur=18, ease='outExpo', fade=true})   moves from (x+dx, y+dy) to its current place
	//@ cav.slideOut(l, f, {dx=0, dy=60, dur=14, ease='inBack', fade=true})
	//@ cav.wipeIn(l, f, {dur=18, from='left'})   reveals a rect-like layer by scaling from one edge
	//@ cav.drawOn(l, f, dur=30, ease='inOutCubic')   trim-path draw-on (layer needs a stroke)
	//@ cav.shake(l, f, amp=20, dur=18, seed=1)  decaying shake; put it on a group ("rig") so it adds up
	//@ cav.punch(l, f, amount=0.08, dur=12)   quick scale bump for beat accents
	//@ cav.stagger(layers, f, step, fn)       calls fn(layer, frame, index) with frame = f + i*step
	// The value of an attribute at frame f (api.get reads at the playhead).
	var VECTOR_PARTS = { position: ['position.x', 'position.y'], scale: ['scale.x', 'scale.y'], rotation: ['rotation.z'] }
	function isAnimated(l, attr) {
		var parts = VECTOR_PARTS[attr] || [attr]
		for (var i = 0; i < parts.length; i++) {
			try {
				if (api.isAnimatedAttribute(l, parts[i])) return true
			} catch (e) {
				return true // unknown: take the safe, slower path
			}
		}
		return false
	}

	// Before its first key or after its last key, an attribute holds that key's value. Read it
	// from the keyframe node (data.numValue at timeOffset). Returns undefined when f falls
	// between keys or the key holds no plain number.
	function heldKey(l, part, f) {
		var ids = api.getKeyframeIdsForAttribute(l, part) || []
		var first = null, last = null
		for (var i = 0; i < ids.length; i++) {
			var k = { t: api.get(ids[i], 'timeOffset'), v: (api.get(ids[i], 'data') || {}).numValue }
			if (!first || k.t < first.t) first = k
			if (!last || k.t > last.t) last = k
		}
		var k2 = !first ? null : f <= first.t ? first : f >= last.t ? last : null
		return k2 && typeof k2.v === 'number' ? k2.v : undefined
	}
	function fastValueAt(l, attr, f) {
		try {
			var parts = VECTOR_PARTS[attr]
			if (!parts) return api.isAnimatedAttribute(l, attr) ? heldKey(l, attr, f) : api.get(l, attr)
			if (attr === 'rotation') return undefined
			var out = {}
			for (var i = 0; i < parts.length; i++) {
				var v = api.isAnimatedAttribute(l, parts[i]) ? heldKey(l, parts[i], f) : api.get(l, parts[i])
				if (typeof v !== 'number') return undefined
				out[parts[i].split('.')[1]] = v
			}
			return out
		} catch (e) {
			return undefined
		}
	}

	function valueAt(l, attr, f) {
		l = lid(l, 'cav.valueAt')
		// Moving the playhead makes Cavalry re-evaluate the whole scene, which is slow in big
		// scenes. A value without keys is the same on every frame, so read it directly.
		if (!isAnimated(l, attr)) return api.get(l, attr)
		var held = fastValueAt(l, attr, f)
		if (held !== undefined) return held
		var cur = api.getFrame()
		api.setFrame(Math.round(f))
		var v = api.get(l, attr)
		api.setFrame(cur)
		return v
	}
	cav.valueAt = valueAt

	cav.fadeIn = function (l, f, dur) {
		l = lid(l, 'cav.fadeIn')
		return cav.tween(l, 'opacity', f, f + (dur || 10), 0, 100, 'out')
	}

	cav.fadeOut = function (l, f, dur) {
		l = lid(l, 'cav.fadeOut')
		var cur = valueAt(l, 'opacity', f)
		return cav.tween(l, 'opacity', f, f + (dur || 10), cur > 0 ? cur : 100, 0, 'in')
	}

	cav.pop = function (l, f, o) {
		l = lid(l, 'cav.pop')
		o = o || {}
		var dur = o.dur || 14
		return cav.tween(l, 'scale', f, f + dur, o.from === undefined ? 0 : o.from, o.to === undefined ? 1 : o.to, o.ease || 'outBack')
	}

	cav.slideIn = function (l, f, o) {
		l = lid(l, 'cav.slideIn')
		o = o || {}
		var dur = o.dur || 18
		var p = valueAt(l, 'position', f)
		var dx = o.dx || 0, dy = o.dy === undefined ? -60 : o.dy
		cav.tween(l, 'position', f, f + dur, [p.x + dx, p.y + dy], [p.x, p.y], o.ease || 'outExpo')
		if (o.fade !== false) cav.tween(l, 'opacity', f, f + Math.max(4, Math.round(dur * 0.6)), 0, 100, 'out')
		return l
	}

	cav.slideOut = function (l, f, o) {
		l = lid(l, 'cav.slideOut')
		o = o || {}
		var dur = o.dur || 14
		var p = valueAt(l, 'position', f)
		var dx = o.dx || 0, dy = o.dy === undefined ? 60 : o.dy
		cav.tween(l, 'position', f, f + dur, [p.x, p.y], [p.x + dx, p.y + dy], o.ease || 'inBack')
		if (o.fade !== false) cav.tween(l, 'opacity', f + Math.round(dur * 0.4), f + dur, 100, 0, 'in')
		return l
	}

	cav.wipeIn = function (l, f, o) {
		l = lid(l, 'cav.wipeIn')
		o = o || {}
		var dur = o.dur || 18
		var bb = api.getBoundingBox(l, false)
		var from = o.from || 'left'
		var sc = valueAt(l, 'scale', f)
		var p = valueAt(l, 'position', f)
		// Move the pivot to the edge we grow from, and move the position to keep the layer in place.
		var px = from === 'left' ? -bb.width / 2 : from === 'right' ? bb.width / 2 : 0
		var py = from === 'bottom' ? -bb.height / 2 : from === 'top' ? bb.height / 2 : 0
		api.set(l, { 'pivot.x': px, 'pivot.y': py, 'position.x': p.x + px * sc.x, 'position.y': p.y + py * sc.y })
		var axis = from === 'left' || from === 'right' ? 'scale.x' : 'scale.y'
		var full = axis === 'scale.x' ? sc.x : sc.y
		return cav.tween(l, axis, f, f + dur, 0, full, o.ease || 'outExpo')
	}

	cav.drawOn = function (l, f, dur, ease) {
		l = checkLayer(l, 'cav.drawOn')
		api.set(l, { 'stroke.trim': true, 'stroke.trimStart': 0 })
		return cav.tween(l, 'stroke.trimEnd', f, f + (dur || 30), 0, 100, ease || 'inOutCubic')
	}

	cav.rng = function (seed) {
		var a = (seed || 1) >>> 0
		return function () {
			a |= 0
			a = (a + 0x6d2b79f5) | 0
			var t = Math.imul(a ^ (a >>> 15), 1 | a)
			t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
			return ((t ^ (t >>> 14)) >>> 0) / 4294967296
		}
	}

	cav.shake = function (l, f, amp, dur, seed) {
		l = lid(l, 'cav.shake')
		amp = amp === undefined ? 20 : amp
		dur = dur || 18
		var r = cav.rng(seed || 1)
		var p = valueAt(l, 'position', f)
		var keys = [[f - 1, [p.x, p.y]]]
		for (var t = f; t < f + dur; t += 2) {
			var k = amp * Math.pow(1 - (t - f) / dur, 2)
			keys.push([t, [p.x + (r() * 2 - 1) * k, p.y + (r() * 2 - 1) * k]])
		}
		keys.push([f + dur, [p.x, p.y]])
		return cav.key(l, 'position', keys)
	}

	cav.punch = function (l, f, amount, dur) {
		l = lid(l, 'cav.punch')
		amount = amount === undefined ? 0.08 : amount
		dur = dur || 12
		var s = valueAt(l, 'scale', f)
		return cav.key(l, 'scale', [
			[f - 1, [s.x, s.y]],
			[f, [s.x * (1 + amount), s.y * (1 + amount)], 'outCubic'],
			[f + dur, [s.x, s.y]],
		])
	}

	cav.stagger = function (layers, f, step, fn) {
		if (layers && !Array.isArray(layers) && Array.isArray(layers.chars)) layers = layers.chars
		if (!Array.isArray(layers)) fail('cav.stagger(layers, f, step, fn): layers must be an array of layer ids (or a cav.glyphs result)')
		var n = 0
		for (var i = 0; i < layers.length; i++) {
			if (!layers[i]) continue
			fn(layers[i], Math.round(f + n * step), n)
			n++
		}
		return layers
	}

	//@
	//@ ## Impacts and transitions
	//@ cav.flash(f, {color='#ffffff', dur=6, peak=60})   full-frame flash plane, returns its id (keep it short and partial)
	//@ cav.ring(f, x, y, {r0=10, r1=400, dur=24, width=12, color})   expanding shockwave ring
	//@ cav.burst(f, x, y, {count=12, dist=260, size=10, dur=24, color, seed})  radial particle burst (group id)
	//@ cav.zoomThrough(rig, f0, f1, [tx, ty], {scale=12, ease='inExpo'})
	//@     scales a rig group so the point (tx, ty) flies at the camera and fills the frame.
	//@     Baked with keys every 2 frames so the target stays centred.
	cav.flash = function (f, o) {
		o = o || {}
		var l = cav.plane('flash', o.color || '#ffffff', { opacity: 0 })
		var dur = o.dur || 6
		cav.key(l, 'opacity', [
			[f - 1, 0],
			[f, o.peak || 60, 'outQuad'],
			[f + dur, 0],
		])
		api.setInFrame(l, Math.max(0, f - 1))
		api.setOutFrame(l, f + dur + 1)
		return l
	}

	cav.ring = function (f, x, y, o) {
		o = o || {}
		var dur = o.dur || 24
		var l = cav.circle('ring', o.r0 || 10, { x: x, y: y, noFill: true, stroke: { color: o.color || '#ffffff', width: o.width || 12 } })
		cav.key(l, 'generator.radius', [
			[f, [o.r0 || 10, o.r0 || 10], 'outExpo'],
			[f + dur, [o.r1 || 400, o.r1 || 400]],
		])
		cav.key(l, 'stroke.width', [
			[f, o.width || 12, 'outCubic'],
			[f + dur, 0],
		])
		cav.key(l, 'opacity', [
			[f - 1, 0],
			[f, 100],
			[f + dur, 0],
		])
		api.setInFrame(l, Math.max(0, f - 1))
		api.setOutFrame(l, f + dur + 1)
		return l
	}

	cav.burst = function (f, x, y, o) {
		o = o || {}
		var n = o.count || 12, dist = o.dist || 260, dur = o.dur || 24, size = o.size || 10
		var r = cav.rng(o.seed || 7)
		var g = cav.group('burst', { x: x, y: y })
		for (var i = 0; i < n; i++) {
			var a = (i / n) * Math.PI * 2 + (r() - 0.5) * 0.4
			var d = dist * (0.6 + r() * 0.6)
			var p = cav.circle('burst_' + i, size * (0.6 + r() * 0.8), { parent: g, fill: o.color || '#ffffff' })
			cav.key(p, 'position', [
				[f, [0, 0], 'outExpo'],
				[f + dur, [Math.cos(a) * d, Math.sin(a) * d]],
			])
			cav.key(p, 'scale', [
				[f, 1, 'in'],
				[f + dur, 0],
			])
		}
		api.setInFrame(g, Math.max(0, f - 1))
		api.setOutFrame(g, f + dur + 1)
		return g
	}

	cav.zoomThrough = function (rig, f0, f1, target, o) {
		rig = lid(rig, 'cav.zoomThrough')
		o = o || {}
		var s1 = o.scale || 12
		var ease = cav.E[o.ease || 'inExpo']
		if (!ease) fail('cav.zoomThrough: unknown ease ' + o.ease)
		// Evaluate the easing expression in JS to bake the keys.
		var fn = new Function('x', 'var pow=Math.pow,exp=Math.exp,sin=Math.sin,cos=Math.cos; return ' + ease)
		var keys = [], pk = []
		for (var f = f0; f <= f1; f += 2) {
			var t = fn((f - f0) / (f1 - f0))
			var s = 1 + (s1 - 1) * t
			keys.push([f, s])
			// World position of the target = rigPos + s * target. Move it from where it is
			// to the centre as t goes 0 -> 1: rigPos = target * (1 - t) - s * target.
			pk.push([f, [target[0] * (1 - t) - s * target[0], target[1] * (1 - t) - s * target[1]]])
		}
		if ((f1 - f0) % 2) {
			keys.push([f1, s1])
			pk.push([f1, [-target[0] * s1, -target[1] * s1]])
		}
		cav.key(rig, 'scale', keys)
		cav.key(rig, 'position', pk)
		return rig
	}

	//@
	//@ ## Kinetic type
	//@ cav.glyphs(name, string, size, o) -> {group, chars:[ids], width}
	//@     one text layer per character, placed with real advances, grouped. Same o as cav.text
	//@     plus parent/x/y for the group. x is the word's centre unless o.align is 'left' or
	//@     'right'. Use it to animate letters one by one.
	//@ cav.cascade(glyphs, f, {step=2, dy=-80, dur=18, ease='outBack', rotate=0, fade=true})
	//@     letters drop in one after the other.
	//@ cav.typeOn(glyphs, f, {rate=2})   letters appear one per `rate` frames (typewriter).
	cav.glyphs = function (name, str, size, o) {
		o = o || {}
		var font = cav.font(o.font || 'Inter', o.style || 'Bold')
		var g = cav.group(name, { parent: o.parent, x: o.x, y: o.y })
		var sp = o.spacing || 0
		var advs = [0]
		var ref = cavalry.measureText('H', font.font, font.style, size).width
		for (var i = 1; i <= str.length; i++) {
			// Advance = width of prefix+H minus width of H: keeps kerning and trailing spaces.
			advs.push(cavalry.measureText(str.substr(0, i) + 'H', font.font, font.style, size).width - ref + sp * i)
		}
		var total = advs[str.length]
		var align = o.align || 'center'
		var ox = align === 'left' ? 0 : align === 'right' ? -total : -total / 2
		var chars = []
		for (i = 0; i < str.length; i++) {
			if (str[i] === ' ') continue
			chars.push(
				cav.text(name + '_' + i, str[i], size, {
					font: font.font,
					style: font.style,
					color: o.color,
					parent: g,
					x: ox + (advs[i] + advs[i + 1]) / 2,
					y: 0,
				}),
			)
		}
		return { group: g, chars: chars, width: total }
	}

	cav.cascade = function (gl, f, o) {
		o = o || {}
		var step = o.step === undefined ? 2 : o.step, dur = o.dur || 18, dy = o.dy === undefined ? -80 : o.dy
		return cav.stagger(gl.chars, f, step, function (c, fr) {
			var p = valueAt(c, 'position', fr)
			cav.tween(c, 'position', fr, fr + dur, [p.x, p.y + dy], [p.x, p.y], o.ease || 'outBack')
			if (o.rotate) cav.tween(c, 'rotation', fr, fr + dur, o.rotate, 0, 'outCubic')
			if (o.fade !== false) cav.tween(c, 'opacity', fr, fr + Math.max(3, Math.round(dur / 2)), 0, 100, 'out')
		})
	}

	cav.typeOn = function (gl, f, o) {
		o = o || {}
		var rate = o.rate || 2
		return cav.stagger(gl.chars, f, rate, function (c, fr) {
			cav.key(c, 'opacity', [
				[fr - 1, 0],
				[fr, 100],
			])
		})
	}


	//@
	//@ ## Native features (fewer layers, faster scripts; all verified in Cavalry 2.7.2)
	//@ A behaviour connected to an attribute REPLACES its value (it does not add to it).
	//@ cav.duplicator(name, source, {type, count, size, radius, seed, path, parent}) -> duplicator id
	//@     type: 'grid' (count [cols, rows], size [w, h]), 'circle' (count, radius), 'linear' (count,
	//@     size = total length), 'random' (count, size [w, h], seed), 'path' (count, path: layer id).
	//@     The source is hidden; the duplicator draws the copies. The source's OWN position/scale/
	//@     rotation keys are ignored: to animate each copy, put the shape in a group and pass the group.
	//@ cav.staggerTime(target, spread, {reverse}) -> stagger id
	//@     Delays copies of a duplicator or sub-mesh one after another: copy 0 starts first and the
	//@     last copy starts `spread` frames later (reverse: true flips the order).
	//@ cav.textCascade(textLayer, f, {step=3, dur=16, dy=-120, scale=0.4, rotate=-20, fade=true, ease='outBack'})
	//@     Per-letter animation on ONE text layer (Sub-Mesh + Stagger). Returns the sub-mesh id.
	//@ cav.counter(textLayer, f0, f1, from, to, {prefix, suffix, decimals=0, ease='outCubic'})
	//@     Drives the text with an animated number (String Generator). Returns the generator id.
	//@ cav.oscillate(layer, attr, {min, max, freq=1})   sine wave on an attribute (freq in cycles per second)
	//@ cav.wiggle(layer, attr, {min, max, freq=1, seed})  smooth noise on an attribute
	//@     attr can be one channel ('position.y', 'rotation.z', 'scale.x'), or a duplicator's
	//@     'shapePosition.y', 'shapeScale.y', 'shapeRotation', 'shapeOpacity'.
	//@ cav.sound(path, layer, attr, {min=0, max=1, low, high, smooth=1})   drive an attribute live
	//@     from the music's level (Cavalry's Sound behaviour): min when quiet, max at the
	//@     loudest. low/high limit it to a frequency range in Hz (20-150 = kick and bass).
	//@     Returns the Sound layer. Use it for continuous pulsing; key the big hits to the grid.
	//@ cav.gradient(layer, ['#hex', '#hex', ...], {type='linear'|'radial', rotation=0})  gradient fill
	//@     On text it spans the whole word. Animate its 'generator.offset.x' for a shimmer.
	//@ cav.motionBlur(samples=16)   turns on comp motion blur AND per-layer blur on every layer
	//@     (call it last, after all layers exist; the comp switch alone renders sharp).
	//@ cav.pro(type) -> true when a layer type needs a Cavalry Pro licence (forge, particles, camera,
	//@     glow, javaScript...). Starter licences cannot save or render those.
	var DIST = { grid: 'gridDistribution', circle: 'circleDistribution', linear: 'linearDistribution', random: 'randomDistribution', path: 'pathDistribution' }

	cav.pro = function (type) {
		try {
			return !!api.isProLayerType(type)
		} catch (e) {
			return false
		}
	}

	cav.duplicator = function (name, source, o) {
		o = o || {}
		source = checkLayer(source, 'cav.duplicator')
		var type = o.type || 'grid'
		if (!DIST[type]) fail('cav.duplicator type must be one of: ' + Object.keys(DIST).join(', '))
		var d = cav.create('duplicator', name)
		api.connect(source, 'id', d, 'shapes')
		api.set(source, { hidden: true })
		api.setGenerator(d, 'generator', DIST[type])
		var set = {}
		function pair(v, def) {
			if (v === undefined) v = def
			return Array.isArray(v) ? v : [v, v]
		}
		if (type === 'grid') {
			var c = pair(o.count, 3), sz = pair(o.size, 200)
			set['generator.count.x'] = c[0]
			set['generator.count.y'] = c[1]
			set['generator.size.x'] = sz[0]
			set['generator.size.y'] = sz[1]
		} else if (type === 'random') {
			var rs = pair(o.size, 500)
			set['generator.count'] = o.count || 50
			set['generator.size.x'] = rs[0]
			set['generator.size.y'] = rs[1]
			if (o.seed !== undefined) set['generator.seed'] = o.seed
		} else {
			set['generator.count'] = o.count || 3
			if (type === 'circle') set['generator.radius'] = o.radius || 200
			if (type === 'linear') set['generator.size'] = o.size || 200
		}
		api.set(d, set)
		if (type === 'path') {
			if (!o.path) fail("cav.duplicator type 'path' needs o.path (a path layer id)")
			api.connect(o.path, 'id', d, 'generator.inputShape')
		}
		if (o.parent) cav.set(d, { parent: o.parent })
		return d
	}

	cav.staggerTime = function (target, spread, o) {
		o = o || {}
		target = checkLayer(target, 'cav.staggerTime')
		var st = cav.create('stagger', api.getNiceName(target) + ' stagger')
		// shapeTimeOffset shows each copy at frame + value, so later copies need negative values.
		// Stagger sorts minimum/maximum, so the direction comes from the sign of strength.
		api.set(st, { minimum: 0, maximum: Math.abs(spread), strength: o.reverse ? 100 : -100 })
		api.connect(st, 'id', target, 'shapeTimeOffset')
		return st
	}

	cav.textCascade = function (text, f, o) {
		o = o || {}
		text = checkLayer(text, 'cav.textCascade')
		var dur = o.dur || 16, step = o.step === undefined ? 3 : o.step
		var sm = cav.create('subMesh', api.getNiceName(text) + ' letters')
		var ease = o.ease || 'outBack'
		var dy = o.dy === undefined ? -120 : o.dy
		cav.key(sm, 'shapePosition.y', [[f, dy, ease], [f + dur, 0]])
		if (o.scale !== false) {
			var s0 = o.scale === undefined ? 0.4 : o.scale
			cav.key(sm, 'shapeScale', [[f, [s0, s0], ease], [f + dur, [1, 1]]])
		}
		if (o.rotate !== false) cav.key(sm, 'shapeRotation', [[f, o.rotate === undefined ? -20 : o.rotate, 'outCubic'], [f + dur, 0]])
		if (o.fade !== false) cav.key(sm, 'shapeOpacity', [[f, 0, 'out'], [f + Math.max(4, Math.round(dur / 2)), 100]])
		api.connect(sm, 'id', text, 'deformers')
		var n = cav.textOf(text).replace(/\s/g, '').length
		cav.staggerTime(sm, step * Math.max(1, n - 1), { reverse: o.reverse })
		return sm
	}

	// The text attribute is an object {text, overrides}; this returns the plain string.
	cav.textOf = function (layer) {
		var v = api.get(layer, 'text')
		return v && typeof v === 'object' ? String(v.text) : String(v)
	}

	cav.counter = function (text, f0, f1, from, to, o) {
		o = o || {}
		text = checkLayer(text, 'cav.counter')
		var sg = cav.create('stringGenerator', api.getNiceName(text) + ' number')
		api.set(sg, { 'generator.precision': o.decimals || 0, 'generator.length': 1, prefix: o.prefix || '', suffix: o.suffix || '' })
		cav.key(sg, 'generator.number', [[f0, from, o.ease || 'outCubic'], [f1, to]])
		api.connect(sg, 'id', text, 'text')
		return sg
	}

	cav.oscillate = function (layer, attr, o) {
		o = o || {}
		layer = checkLayer(layer, 'cav.oscillate')
		var osc = cav.create('oscillator', api.getNiceName(layer) + ' oscillator')
		api.set(osc, { minimum: o.min === undefined ? -10 : o.min, maximum: o.max === undefined ? 10 : o.max, frequency: o.freq || 1, stagger: o.stagger === undefined ? 1 : o.stagger })
		cav.connect(osc, 'id', layer, ALIAS[attr] || attr)
		return osc
	}

	cav.wiggle = function (layer, attr, o) {
		o = o || {}
		layer = checkLayer(layer, 'cav.wiggle')
		var nz = cav.create('noise', api.getNiceName(layer) + ' wiggle')
		var d = { 'generator.minimum': o.min === undefined ? -10 : o.min, 'generator.maximum': o.max === undefined ? 10 : o.max, 'generator.frequency': o.freq || 1 }
		if (o.seed !== undefined) d['generator.seed'] = o.seed
		api.set(nz, d)
		cav.connect(nz, 'id', layer, ALIAS[attr] || attr)
		return nz
	}

	cav.sound = function (path, layer, attr, o) {
		o = o || {}
		layer = checkLayer(layer, 'cav.sound')
		if (!api.filePathExists(path)) fail('cav.sound: file not found: ' + path + ' (use an absolute path)')
		var min = o.min === undefined ? 0 : o.min, max = o.max === undefined ? 1 : o.max
		var snd = cav.create('sound', o.name || api.getNiceName(layer) + ' sound')
		// The file is a connection from the audio asset, not a value.
		cav.connect(api.loadAsset(path, false), 'id', snd, 'file')
		// With autoNormalise the level runs 0..1 and the output is offset + strength/100 * level.
		var d = { autoNormalise: true, offset: min, strength: (max - min) * 100, smoothingFrames: o.smooth || 1 }
		if (o.low !== undefined || o.high !== undefined) d.frequencyRange = { x: o.low || 20, y: o.high || 20000 }
		api.set(snd, d)
		cav.connect(snd, 'id', layer, ALIAS[attr] || attr)
		return snd
	}

	cav.gradient = function (layer, colors, o) {
		o = o || {}
		layer = checkLayer(layer, 'cav.gradient')
		if (!colors || colors.length < 2) fail('cav.gradient needs at least two colours')
		colors.forEach(cav.rgb)
		var g = cav.create('gradientShader', api.getNiceName(layer) + ' gradient')
		if (o.type === 'radial') api.setGenerator(g, 'generator', 'radialGradientShader')
		api.setGradientFromColors(g, 'generator.gradient', colors)
		if (o.rotation !== undefined && o.type !== 'radial') api.set(g, { 'generator.rotation': o.rotation })
		api.connect(g, 'id', layer, 'material.colorShaders')
		if (api.getLayerType(layer) === 'textShape') api.set(g, { screenSpace: true })
		return g
	}

	cav.motionBlur = function (samples) {
		var comp = api.getActiveComp()
		api.set(comp, { motionBlur: true, motionBlurSamples: samples || 16, shutterAngle: 180 })
		var n = 0
		api.getCompLayers(false).forEach(function (id) {
			try {
				api.set(id, { motionBlur: 1 })
				n++
			} catch (e) {}
		})
		return n
	}

	//@
	//@ ## Inspecting
	//@ cav.bbox(layer)   world bounding box {left, right, top, bottom, width, height, centre}
	//@     In the run that created a layer, Cavalry 2.8.0 keeps a child's world box at its first
	//@     value even when an animated parent moves; read it again in a later `cav run`.
	//@ cav.keys(layer)   {attr: [frames]} for every animated attribute
	cav.bbox = function (l) {
		l = checkLayer(l, 'cav.bbox')
		return api.getBoundingBox(l, true)
	}

	cav.keys = function (l) {
		l = checkLayer(l, 'cav.keys')
		var out = {}
		api.getAnimatedAttributes(l).forEach(function (a) {
			out[a] = api.getKeyframeTimes(l, a)
		})
		return out
	}

	//@
	//@ ## Production guardrails
	//@ cav.below(a, b) / cav.above(a, b)     explicit stacking relative to a sibling
	//@ cav.precomp(name, build, options)    build in a new comp; restore comp/frame on error too
	//@ cav.sksl(layer, code, {inputs})      attach a SkSL filter; inputs map names to numbers or {type,value}
	//@ cav.axes(text, {wght:900, ROND:50})   use the selected font's actual fvar axis tags/ranges
	//@ cav.driver(layer, attr, expression, {start,end,from=0,to=1})   JavaScript with a two-key n0 clock
	//@ cav.copyDriver(layer, attr, expression)   typed per-copy JavaScript; destination-aware rotation and hex colours
	//@ cav.lookAt(layer, target, {attr='rotation', offset=0})    connect target and orientation offset
	cav.below = cav.order
	cav.above = function (layer, other) {
		layer = checkLayer(layer, 'cav.above')
		other = checkLayer(other, 'cav.above')
		if (layer === other) return layer
		if (api.getParent(layer) !== api.getParent(other)) fail('cav.above: layers must be siblings')
		var parent = api.getParent(other),
			siblings = parent ? api.getChildren(parent) : api.getCompLayers(true),
			i = siblings.indexOf(other)
		if (i < 0) fail('cav.above: sibling order unavailable')
		if (i === 0) {
			api.reorder(layer, other)
			api.reorder(other, layer)
		} else if (siblings[i - 1] !== layer) api.reorder(layer, siblings[i - 1])
		return layer
	}
	cav.precomp = function (name, build, o) {
		o = o || {}
		if (typeof build !== 'function') fail('cav.precomp requires a build function')
		var original = api.getActiveComp(),
			frame = api.getFrame(),
			settings = cav.comp(),
			sub
		try {
			sub = api.createComp(name)
			api.set(sub, {
				resolution: [o.width || settings.width, o.height || settings.height],
				fps: o.fps || settings.fps,
				startFrame: o.start === undefined ? settings.start : o.start,
				endFrame: o.end === undefined ? settings.end : o.end,
				backgroundColor: { r: 0, g: 0, b: 0, a: 0 },
			})
			api.setActiveComp(sub)
			build(sub)
		} finally {
			api.setActiveComp(original)
			api.setFrame(frame)
		}
		api.select([])
		var reference = api.createCompReference(sub)
		api.rename(reference, name)
		api.select([])
		return reference
	}
	cav.sksl = function (layer, code, o) {
		layer = checkLayer(layer, 'cav.sksl')
		o = o || {}
		var inputs = o.inputs || {},
			names = Object.keys(inputs)
		var specs = names.map(function (name) {
			if (!/^[A-Za-z_]\w*$/.test(name) || name === 'layer' || name === 'resolution')
				fail('cav.sksl: invalid/reserved input ' + name)
			var spec = inputs[name]
			if (spec === null || spec === undefined) fail('cav.sksl: missing specification for ' + name)
			var type = typeof spec === 'number' ? 'double' : spec.type
			if (['double', 'double2', 'double3', 'double4', 'color', 'shader'].indexOf(type) < 0)
				fail('cav.sksl: unsupported input type ' + type)
			return { name: name, type: type, value: typeof spec === 'number' ? spec : spec.value }
		})
		var f = cav.create('skslFilter', api.getNiceName(layer) + ' SkSL')
		specs.forEach(function (spec) {
			var name = spec.name,
				type = spec.type,
				value = spec.value
			var children = api.getAttrChildren(f, 'inputs')
			var attr = children.filter(function (a) {
				return (api.getCustomAttributeName(f, a) || api.getAttributeNiceName(f, a)) === name
			})[0]
			if (attr) {
				if (api.getAttrType(f, attr) !== type) fail('cav.sksl: existing input type differs for ' + name)
			} else {
				api.addDynamic(f, 'inputs', type)
				children = api.getAttrChildren(f, 'inputs')
				attr = children[children.length - 1]
				api.renameAttribute(f, attr, name)
			}
			if (value !== undefined) {
				var d = {}
				d[attr] = value
				api.set(f, d)
			}
			// Strip only declarations of added inputs, never built-ins such as shader layer.
			code = String(code).replace(
				/\buniform\s+((?:(?:lowp|mediump|highp)\s+)?\w+)\s+([^;]+);/g,
				function (declaration, type, variables) {
					var kept = variables.split(',').filter(function (v) {
						return !new RegExp('^\\s*' + name + '\\s*(?:$|\\[)').test(v)
					})
					return kept.length ? 'uniform ' + type + ' ' + kept.join(',') + ';' : ''
				}
			)
		})
		api.set(f, { code: String(code) })
		api.connect(f, 'id', layer, 'filters')
		return f
	}
	cav.axes = function (layer, values) {
		layer = checkLayer(layer, 'cav.axes')
		var font = api.get(layer, 'font'),
			axes = (globalThis.CAV_FONT_AXES || {})[font.font]
		if (!axes)
			fail('cav.axes: unambiguous fvar metadata unavailable for ' + font.font + '; add the font folder to CAV_FONT_DIR')
		var children = api.getAttrChildren(layer, 'fontAxes'),
			mapped = Object.create(null),
			d = {}
		if (children.length !== axes.length) fail('cav.axes: native axis count differs from the font metadata')
		axes.forEach(function (axis, i) {
			if (api.getAttributeNiceName(layer, children[i]) !== axis.name)
				fail('cav.axes: native axis order/name differs from fvar; refusing numeric index ' + i)
			mapped[axis.tag] = { attr: children[i], min: axis.min, max: axis.max }
		})
		Object.keys(values).forEach(function (tag) {
			var axis = mapped[tag],
				value = values[tag]
			if (!axis) fail('cav.axes: unknown axis ' + tag)
			if (typeof value !== 'number' || !isFinite(value)) fail('cav.axes: finite value required for ' + tag)
			if (value < axis.min || value > axis.max)
				console.warn('cav.axes: ' + tag + ' clamped to ' + axis.min + '..' + axis.max)
			d[axis.attr] = Math.max(axis.min, Math.min(axis.max, value))
		})
		api.set(layer, d)
		return layer
	}
	cav.driver = function (layer, attr, expression, o) {
		o = o || {}
		layer = checkLayer(layer, 'cav.driver')
		var start = o.start === undefined ? cav.comp().start : o.start,
			end = o.end === undefined ? cav.comp().end : o.end
		if (!(end > start)) fail('cav.driver: end must follow start')
		var js = cav.create('javaScript', api.getNiceName(layer) + ' driver')
		api.set(js, { expression: String(expression) })
		cav.key(js, 'array.0', [
			[start, o.from === undefined ? 0 : o.from],
			[end, o.to === undefined ? 1 : o.to],
		])
		cav.connect(js, 'id', layer, ALIAS[attr] || attr)
		return js
	}
	cav.copyDriver = function (layer, attr, expression) {
		layer = checkLayer(layer, 'cav.copyDriver')
		attr = ALIAS[attr] || attr
		var type = api.getAttrType(layer, attr)
		if (!type) fail('cav.copyDriver: unknown destination attribute ' + attr)
		if (isColorAttr(attr)) type = 'color'
		var code = 'var v=(' + expression + ');\n'
		if (type === 'double' || type === 'int')
			code +=
				'if(v&&typeof v==="object"&&' +
				JSON.stringify(attr === 'shapeRotation') +
				')v=v.z;\nif(typeof v!=="number"||!isFinite(v))throw Error("scalar driver requires a finite number");\n'
		if (type === 'double3')
			code +=
				'if(typeof v==="number")v={x:0,y:0,z:v};\nif(!v||![v.x,v.y,v.z].every(function(n){return typeof n==="number"&&isFinite(n)}))throw Error("rotation driver requires {x,y,z}");\n'
		if (type === 'color')
			code +=
				'if(typeof v==="string"&&/^#[0-9a-f]{6}([0-9a-f]{2})?$/i.test(v)){var h=v.slice(1);v={r:parseInt(h.slice(0,2),16),g:parseInt(h.slice(2,4),16),b:parseInt(h.slice(4,6),16),a:h.length===8?parseInt(h.slice(6,8),16):255};}\nif(!v||![v.r,v.g,v.b,v.a].every(function(n){return typeof n==="number"&&isFinite(n)}))throw Error("colour driver requires {r,g,b,a}");\n'
		var js = cav.create('javaScript', api.getNiceName(layer) + ' per-copy driver')
		api.set(js, { expression: code + 'v' })
		cav.connect(js, 'id', layer, attr)
		return js
	}
	cav.lookAt = function (layer, target, o) {
		o = o || {}
		layer = checkLayer(layer, 'cav.lookAt')
		target = checkLayer(target, 'cav.lookAt')
		var aim = cav.create('lookAt', 'Look At ' + api.getNiceName(target))
		api.connect(target, 'id', aim, 'target')
		api.set(aim, { offset: o.offset || 0 })
		api.connect(aim, 'id', layer, o.attr || 'rotation')
		return aim
	}

	globalThis.cav = cav
})()
