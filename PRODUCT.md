# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

The primary user is a motion designer working with a coding agent. The agent builds and
renders in Cavalry through the `cav` CLI. The designer makes repeated, brief review
passes over logo stings, lower thirds, kinetic type and launch videos. After each pass,
the agent fixes the notes before rendering again.

## Product Purpose

`cav` lets an agent run scripts, render frames and sheets, check scenes, use music beats
and render video in the Cavalry desktop app. On the review page, the designer steps
through exact frames, draws on them and writes notes for a frame or range. The agent
reads those notes, fixes the scene, renders to the same file and resolves each note.
Each note gives the agent a frame, drawing and description of the requested change.

## Positioning

Each note supplies data the agent can act on: an exact frame or range, a W3C media
fragment, drawing shapes and a PNG of the marked frame. Plain JSON stores the notes
beside the video. The agent reads them through `cav review wait`, `cav review export`
or the `review_notes` MCP tool.

## Operating Context

- `cav review <video>` serves the page at `http://127.0.0.1:8790` in a desktop browser.
  An all-intra proxy lets each seek reach an exact frame.
- `cav mcp` serves the same page as an MCP App in a chat frame without network access.
  The frame is often 600 to 800 pixels wide and about 600 pixels tall. It receives a
  640-pixel preview inline. Claude desktop chat displays it; the Code tab does not.
- Rendering again to the same file reloads the page. A/B wipe, difference and onion-skin
  modes compare it with an earlier render.
- Notes live in `<video>.review.json`, with snapshots in `review/<video>/`.

## Capabilities and Constraints

- The page uses one self-contained HTML file with inline CSS and JavaScript
  (`assets/review/index.html`). It has no dependencies or network requests. It must
  work under the default MCP Apps CSP: inline script and style, with images and media
  from `data:`.
- The chat shim (`assets/review/mcp-app.js`) depends on the page's element ids, its `fetch` calls and its `src` attributes. The page's HTTP API is fixed by `internal/review/server.go`.
- The page already supports exact frame steps, J/K/L shuttle, I/O ranges, coloured
  drawing tools, note resolve/reply/delete and Send to agent. It also has compare modes,
  an audio waveform and light/dark themes with manual override.

## Brand Commitments

The cav identity uses ink (`#16181f`) and orange (`#ff6a3d` in the logo and `#ff4d2e`
in the launch video). The logo has a rounded ink square, an orange curve, a light dot
and a light diamond (`plugins/cavalry/assets/logo.svg`). Other visual choices remain open.

## Evidence on Hand

Real review renders are in `launch-video/renders/`. For example, `pass4.mp4` has a
960×540 frame, 60 fps and 3600 frames. The project has no customer quotes, metrics or
third-party proof. Do not invent any.

## Product Principles

1. Show the current frame clearly and let the designer mark an exact area on it.
2. Attach every note to a frame or range that the agent can identify.
3. Keep repeated actions to one key or click when possible.
4. Make the page work in a full browser and a small chat frame.

## Accessibility & Inclusion

- Users can reach every action by keyboard, with relevant shortcuts shown.
- Users can complete the review with a mouse or trackpad without shortcuts.
- Users can draw and scrub by touch, including with a finger or pencil on an iPad.
- Both themes must meet WCAG AA contrast.
