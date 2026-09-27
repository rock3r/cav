// Package assets embeds the files that the cav binary installs or sends to Cavalry.
package assets

import _ "embed"

//go:embed bridge/cav-bridge.js
var BridgeJS []byte

//go:embed helpers/cav-helpers.js
var HelpersJS []byte

//go:embed apiref/api.json
var APIRef []byte

//go:embed apiref/layertypes.json
var LayerTypes []byte
