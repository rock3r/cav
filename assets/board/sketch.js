// cav board frames, sketch: render one normalized sketch SVG to a PNG in the scratch
// composition, then remove what it imported.
// Input: sample = {svg, out, scale, comp}
//   svg    absolute path of the normalized SVG (cav writes it with viewBox "0 0 W H", where
//          W x H is the composition size)
//   out    absolute path of the PNG without ".png" (Cavalry adds the extension)
//   scale  render scale in percent
//   comp   the id of the scratch composition cav made for this run
// The job refuses to run when another composition is active, so it never draws into the
// user's own work.

if (api.getActiveComp() !== sample.comp) throw new Error('the scratch comp changed; cav draws sketches only in its own scratch comp')

var before = api.getCompLayers(true)
var ids = []
try {
	ids = api.convertSVGToLayers(sample.svg)
	if (!ids || !ids.length) throw new Error('Cavalry imported nothing from ' + sample.svg)
	// Measured in Cavalry 2.8.0: an SVG with viewBox "0 0 W H" lands 1:1 in a W x H
	// composition, so SVG point (x, y) is the composition point (x - W/2, H/2 - y). cav
	// writes that viewBox, so nothing is moved or scaled here.
	api.setFrame(api.get(sample.comp, 'startFrame'))
	api.renderPNGFrame(sample.out, sample.scale)
	if (!api.filePathExists(sample.out + '.png')) throw new Error('Cavalry did not write ' + sample.out + '.png')
} finally {
	// Delete the import's top-level group (it holds the shapes) and anything else that is
	// new at the top level, then any imported layer that is still there.
	var drop = []
	api.getCompLayers(true).forEach(function (id) {
		if (before.indexOf(id) < 0) drop.push(id)
	})
	;(ids || []).forEach(function (id) {
		if (drop.indexOf(id) < 0) drop.push(id)
	})
	drop.forEach(function (id) {
		try {
			if (api.layerExists(id)) api.deleteLayer(id)
		} catch (e) {}
	})
}
return { png: sample.out + '.png', layers: ids.length }
