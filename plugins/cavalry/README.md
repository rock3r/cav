# Cavalry for coding agents

Build and render motion graphics in the Cavalry desktop app using the `cav` command-line tool. The plugin provides a skill that checks your setup, reads the CLI's guides, and drives scenes through the local cav-bridge script. Use it for logo stings, kinetic type, lower thirds, charts, UI walkthroughs, and animation synchronized to music.

## Requirements

- Cavalry 2.4 or newer on macOS or Windows; native behavior has been tested on Cavalry 2.8 on macOS.
- The `cav` CLI and ffmpeg for video and audio workflows.
- Cavalry running with **Scripts > cav-bridge** open.

The plugin does not bundle Cavalry, the CLI executable, or ffmpeg. The included skill asks before installing a missing CLI. Follow the [installation guide](https://github.com/rock3r/cav#install), run `cav doctor`, then ask your coding agent to create a new Cavalry scene. The CLI's `cav guide` explains the workflow, and `cav help <command>` documents each command.

## What runs and where data goes

The agent runs `cav` shell commands. Scene inspection, JavaScript execution, and rendering use the bridge in your local Cavalry app, by default on `127.0.0.1:8723`. The CLI keeps bridge authentication and operation records under your local `~/.cav` directory. Rendered images and videos are written to your filesystem.

Installation and updates fetch public CLI releases and checksums from GitHub. Setup can download public Cavalry documentation for offline search. This plugin includes no hosted MCP server or remote account integration. Your coding agent's normal model-provider and tool permissions still apply.

## Using the skill

Ask for a motion graphic in Cavalry. The skill starts with `cav doctor`, follows the embedded CLI guides, previews changes with contact sheets, and checks the scene before rendering. New work uses a disposable scene; replacing existing work requires explicit authorization. If a native operation times out, retain its operation ID and inspect or resume it rather than submitting the same work again.

## Support and license

Report issues at [rock3r/cav](https://github.com/rock3r/cav/issues). The plugin and CLI are MIT licensed; see [LICENSE](LICENSE). Cavalry is made by Scene Group, now part of Canva. This community project is not affiliated with them.
