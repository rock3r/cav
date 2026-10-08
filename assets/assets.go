// Package assets embeds the files that the cav binary installs or sends to Cavalry.
package assets

import "embed"

//go:embed bridge/cav-bridge.js
var BridgeJS []byte

//go:embed helpers/cav-helpers.js
var HelpersJS []byte

//go:embed apiref/api.json
var APIRef []byte

//go:embed apiref/layertypes.json
var LayerTypes []byte

// Guide holds the usage guides that `cav guide` prints (workflow, design, music, traps,
// native features).
//
//go:embed guide/*.md
var Guide embed.FS

// Python holds the helpers cav runs through uv for local models (scores, music, images).
//
//go:embed python/*.py
var Python embed.FS

// ReviewPage is the single-page review tool that `cav review` serves.
//
//go:embed review/index.html
var ReviewPage []byte

// ReviewAppShim is the script `cav mcp` adds to the review page so that it runs as an MCP
// App: it answers the page's requests through the chat host instead of the network.
//
//go:embed review/mcp-app.js
var ReviewAppShim []byte

// Diagnostics holds metadata and measured sampling scripts.
//
//go:embed diagnostics/*.js
var Diagnostics embed.FS

// BoardPlace is the native job of cav board place.
//
//go:embed board/place.js
var BoardPlace string
