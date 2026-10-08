// cav board place: add storyboard frames to the active composition as image layers.
// Input: shots = [{id, path, start, end, width, height}], with start and end in seconds
// from beat 0 and width and height the image's pixel size (0 when unknown).
// Everything runs in this one job, so the frames are timed and sized from the
// composition they are placed in.

// shotFrames is the first and last visible frame of a shot. Beat 0 lands on the
// composition's start frame. The last frame is one before the frame where the shot ends,
// so shots that touch hand off with no overlap or gap. api.setOutFrame takes it inclusive.
function shotFrames(start, end, fps, first) {
	var a = Math.max(0, Math.round(start * fps))
	var b = Math.max(a, Math.round(end * fps) - 1)
	return [first + a, first + b]
}

// fitScale fits a w x h image inside the composition, letterboxed like the animatic.
function fitScale(w, h, cw, ch) {
	if (!(w > 0 && h > 0 && cw > 0 && ch > 0)) return 0
	return Math.min(cw / w, ch / h)
}

var comp = api.getActiveComp(),
	res = api.get(comp, 'resolution')
var info = {
	fps: api.get(comp, 'fps'),
	width: res.x,
	height: res.y,
	start: api.get(comp, 'startFrame'),
	end: api.get(comp, 'endFrame'),
}

// Reuse an asset already loaded from the same file, reloading it in case the frame was
// remade, so shots that share a frame share one asset.
var assets = {}
api.getAssetWindowLayers(false).forEach(function (a) {
	if (api.isFileAsset(a)) assets[api.getAssetFilePath(a)] = a
})
function asset(path) {
	if (assets[path]) api.reloadAsset(assets[path])
	else assets[path] = api.loadAsset(path, false)
	return assets[path]
}

// shaderOf finds the image shader that draws a footage layer.
function shaderOf(footage) {
	var all = api.getCompLayers(false)
	for (var i = 0; i < all.length; i++) {
		if (api.getLayerType(all[i]) !== 'imageShader') continue
		var outs = api.getOutConnections(all[i], 'id')
		for (var j = 0; j < outs.length; j++) if (outs[j].split('.')[0] === footage) return all[i]
	}
	return null
}

var placed = [],
	skipped = []
shots.forEach(function (s) {
	var f = shotFrames(s.start, s.end, info.fps, info.start)
	var old = null,
		built = null
	// false lists nested layers too, so a placeholder or built shot inside a group counts.
	api.getCompLayers(false).forEach(function (l) {
		if (api.getNiceName(l) !== s.id) return
		if (api.getLayerType(l) === 'footageShape') old = old || l
		else built = built || l
	})
	// Another kind of layer with the shot's name is the built shot replacing its placeholder.
	if (built) {
		skipped.push({ id: s.id, layer: built, type: api.getLayerType(built) })
		return
	}
	// An existing placeholder is updated in place: it gets the new frame and timing, and
	// keeps its parent, transforms, masks and stack position.
	var sh = old && shaderOf(old)
	if (sh) {
		api.connect(asset(s.path), 'id', sh, 'image', true)
		api.setInFrame(old, f[0])
		api.setOutFrame(old, f[1])
		placed.push({ id: s.id, layer: old, in: f[0], out: f[1], scale: 0, updated: true })
		return
	}
	if (!api.filePathExists(s.path)) throw new Error(s.id + ': frame not found: ' + s.path)
	api.select([])
	var l = api.addAssetToComp(asset(s.path))
	if (Array.isArray(l)) l = l[0]
	api.select([])
	api.rename(l, s.id)
	var scale = fitScale(s.width, s.height, info.width, info.height)
	if (scale > 0) api.set(l, { 'scale.x': scale, 'scale.y': scale })
	api.setInFrame(l, f[0])
	api.setOutFrame(l, f[1])
	placed.push({ id: s.id, layer: l, in: f[0], out: f[1], scale: scale, updated: false })
})
return { comp: info, placed: placed, skipped: skipped }
