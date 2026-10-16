# Cav for coding agents

The `cav` command-line tool builds and renders motion graphics in the Cavalry desktop app.
This plugin supplies a skill for setup, CLI guidance and scene work through the local
cav-bridge script. It supports logo stings, kinetic type, lower thirds, charts, UI
walkthroughs and motion timed to music.

## Requirements

- Cavalry 2.4 or newer on macOS or Windows; native behavior has been tested on Cavalry 2.8 on macOS.
- The `cav` CLI and ffmpeg for video and audio workflows.
- Cavalry running with **Scripts > cav-bridge** open.

Install Cavalry, the `cav` CLI and ffmpeg separately. The skill asks before installing a
missing CLI. Follow the [installation guide](https://github.com/rock3r/cav#install) and
run `cav doctor`. Then ask the agent to make a new scene. `cav guide` explains the
workflow, and `cav help <command>` explains individual commands.

## What runs and where data goes

The agent runs `cav` shell commands. The local Cavalry bridge handles scene inspection,
JavaScript and rendering on `127.0.0.1:8723` by default. The CLI stores bridge credentials
and operation records in `~/.cav`. Renders stay on the local filesystem.

The plugin starts `cav mcp` locally over stdin and stdout. `show_review` opens a render's
review page in a supported chat. The user can play it and send notes. `review_notes`
returns those notes and their snapshots to the agent. The server reads videos requested
by the agent and stores notes beside each video. It requires cav 1.2 or newer. With an
older CLI, the server fails to start. Other plugin functions remain available.

Installation and updates download public releases and checksums from GitHub. Setup can
download public Cavalry docs for offline search. The plugin has no hosted MCP service or
remote account connection. The agent still uses its normal model and tool permissions.

## Using the skill

Ask the agent to make a motion graphic in Cavalry. The skill runs `cav doctor`, reads the
CLI guides, inspects contact sheets and checks the scene before rendering. New work uses
a disposable scene. Replacing existing work requires explicit authorization. If a native
operation times out, keep its ID and inspect or resume it before submitting further work.

## Support and license

Report issues through [GitHub Issues](https://github.com/rock3r/cav/issues). See the
[privacy policy](privacy.md) for data handling. The plugin and CLI use the MIT licence;
see [LICENSE](LICENSE). Scene Group, now part of Canva, makes Cavalry. Cav is a community
plugin without affiliation with those companies.
