# Cav for coding agents

Build and render motion graphics in the Cavalry desktop app using the `cav` command-line tool. The plugin provides a skill that checks your setup, reads the CLI's guides, and drives scenes through the local cav-bridge script. Use it for logo stings, kinetic type, lower thirds, charts, UI walkthroughs, and animation synchronized to music.

## Requirements

- Cavalry 2.4 or newer on macOS or Windows; native behavior has been tested on Cavalry 2.8 on macOS.
- The `cav` CLI and ffmpeg for video and audio workflows.
- Cavalry running with **Scripts > cav-bridge** open.

The plugin does not bundle Cavalry, the CLI executable, or ffmpeg. The included skill asks before installing a missing CLI. Follow the [installation guide](https://github.com/rock3r/cav#install), run `cav doctor`, then ask your coding agent to create a new Cavalry scene. The CLI's `cav guide` explains the workflow, and `cav help <command>` documents each command.

## What runs and where data goes

The agent runs `cav` shell commands. Scene inspection, JavaScript execution, and rendering use the bridge in your local Cavalry app, by default on `127.0.0.1:8723`. The CLI keeps bridge authentication and operation records under your local `~/.cav` directory. Rendered images and videos are written to your filesystem.

The plugin also starts a local MCP server, `cav mcp`, over stdin and stdout. Its `show_review` tool shows the `cav review` page for a render inside the chat, where you can play it and leave notes for the agent. It reads only the videos the agent asks it to show, and writes the notes next to the video. It needs cav 1.2 or newer; with an older CLI the server does not start, and everything else works.

Installation and updates fetch public CLI releases and checksums from GitHub. Setup can download public Cavalry documentation for offline search. This plugin includes no hosted MCP server or remote account integration. Your coding agent's normal model-provider and tool permissions still apply.

## Using the skill

Ask for a motion graphic in Cavalry. The skill starts with `cav doctor`, follows the embedded CLI guides, previews changes with contact sheets, and checks the scene before rendering. New work uses a disposable scene; replacing existing work requires explicit authorization. If a native operation times out, retain its operation ID and inspect or resume it rather than submitting the same work again.

## Support and license

Report issues at [GitHub Issues](https://github.com/rock3r/cav/issues). Cav collects no data; see the [privacy policy](privacy.md). The plugin and CLI are MIT licensed; see [LICENSE](LICENSE). Cavalry is made by Scene Group, now part of Canva. Cav is a community plugin and is not affiliated with Cavalry, Scene Group, or Canva.
