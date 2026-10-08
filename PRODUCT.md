# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

The primary user is a motion designer who works with a coding agent. The agent builds and renders motion graphics in Cavalry through the `cav` CLI. The designer reviews the agent's renders alone: many quick passes over short videos (logo stings, lower thirds, kinetic type, launch films), each followed by notes that the agent fixes before the next render.

## Product Purpose

`cav` lets a coding agent drive the Cavalry desktop app from a shell: run scripts, render frames and contact sheets, check scenes, sync to music beats and render video. The review page closes the loop between the designer and the agent. The designer steps through a render frame by frame, draws on the frame, writes notes on a frame or a range, and sends them to the agent. The agent reads the notes, fixes them, re-renders to the same file and resolves each note. Success is a short, exact feedback loop: the agent gets the frame, the drawing and the words it needs, and the designer never has to describe a moment in prose.

## Positioning

The notes are written for an agent, not for a person. Each note carries an exact frame or range, a W3C media fragment, the drawn shapes as data and a PNG snapshot of the frame with the drawing burned in. The review file is plain JSON next to the video, and the agent reads it through `cav review wait`, `cav review export` or the `review_notes` MCP tool.

## Operating Context

- `cav review <video>` serves the page on `http://127.0.0.1:8790` in a desktop browser. It plays an all-intra proxy, so every seek lands on an exact frame.
- `cav mcp` serves the same page inside a chat as an MCP App: a sandboxed frame with no network, often 600 to 800 pixels wide and about 600 pixels tall, with a 640-pixel preview sent inline. The Claude desktop app's chat draws it; its Code tab does not.
- A re-render to the same file reloads the page. Compare modes show it against an earlier render: A/B wipe, difference, and onion skin.
- Notes live in `<video>.review.json`, with snapshots in `review/<video>/`.

## Capabilities and Constraints

- The page is one self-contained HTML file with inline CSS and JavaScript (`assets/review/index.html`). It has no dependencies, loads nothing from the network, and must work under the MCP Apps default CSP: inline script and style only, images and media from `data:`.
- The chat shim (`assets/review/mcp-app.js`) depends on the page's element ids, its `fetch` calls and its `src` attributes. The page's HTTP API is fixed by `internal/review/server.go`.
- Frame-exact stepping, J/K/L shuttle, I/O ranges, drawing tools (arrow, box, ellipse, pen, with colours), notes with resolve, reply and delete, Send to agent, compare modes, an audio waveform and a light/dark theme with a manual override are all existing features.

## Brand Commitments

The page belongs to the cav identity: ink (`#16181f`) and cav orange (`#ff6a3d` in the logo, `#ff4d2e` in the launch video). The logo is a rounded ink square with an orange curve between a light dot and a light diamond (`plugins/cavalry/assets/logo.svg`). The rest of the visual world is open.

## Evidence on Hand

Real renders to review live in `launch-video/renders/` (for example `pass4.mp4`, 960×540, 60 fps, 3600 frames). There are no customer quotes, metrics or third-party proof, and none may be invented.

## Product Principles

1. The frame is the subject. Everything else serves looking at, and pointing at, one exact frame.
2. Exact over approximate. Every note is tied to a frame or range the agent can act on without guessing.
3. Fast for a solo expert. Repeat actions take one key or one click; nothing explains itself twice.
4. Fits where it is opened. The same page works in a full browser window and in a small chat frame.

## Accessibility & Inclusion

- Keyboard first: every action reachable from the keyboard, with shortcuts shown where they matter.
- Fully usable by mouse or trackpad alone, without knowing any shortcut.
- Touch, including drawing and scrubbing with a finger or pencil on an iPad.
- WCAG AA contrast in both the light and the dark theme.
