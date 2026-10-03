# Changelog

## Unreleased

- Persist render/check/run operations with phase-aware recovery, total wait budgets,
  duplicate prevention, read-only status and explicit unknown-outcome reconciliation.
- End waits promptly on refused bridge connections or changed sessions; expose unvalidated
  render artifacts and preserve partial output without retrying native work.
- Add cheap structural performance warnings, bounded checks, chronological measured
  profiling, partial progress and truthful failed/skipped coverage.
- Preserve signed macOS Cav.app bundles across install/update; require Developer ID,
  Hardened Runtime, timestamps, notarization, stapling and final artifact verification
  for release builds. Keep development credential-free and other platform layouts intact.

## 1.0.2

- The project is now called `cav`, and the repository moved to
  [rock3r/cav](https://github.com/rock3r/cav). Old links redirect, and `cav update` from 1.0.0
  and 1.0.1 still works.
- The plugin marketplace is now called `cav`: install the plugin with
  `/plugin install cavalry@cav` (Claude Code) or `codex plugin add cavalry@cav` (Codex). If
  you added the old `cavalry-skill` marketplace, remove it and add `rock3r/cav`.

## 1.0.1

Fixes found by installing 1.0.0 from the release and using it through the Claude Code and
Codex plugins.

- Inside an agent's sandbox, `cav` said the bridge was "not running" when the sandbox had
  blocked the connection. It now says `blocked: a sandbox` and explains the fix: allow
  `127.0.0.1:8723` and `~/.cav`, or use `cav relay`. `cav doctor` also checks that it can
  write its jobs folder.
- Through `cav relay`, the "job running" line came after the result, so agents waited for
  jobs that had already finished. The line now comes first, as in a direct run.
- `cav doctor` exited 0 even when it reported a problem. It now exits 2 when only the bridge is
  not running and 1 for other problems. With `--json` it prints one object.
- The install from the release, `cav setup` and the offline commands were tested on Windows 11
  (x64). Driving Cavalry on Windows is still not tested.

## 1.0.0

The first public release. It lets you, or a coding agent, build, check and render motion
graphics in Cavalry from a terminal.

### What is in it

| Part | What it does |
|---|---|
| `cav` CLI | Runs JavaScript inside a running Cavalry. Renders frames, contact sheets and MP4 videos (with audio). Inspects scenes and layers. Finds the tempo and beats of a music track. Searches the Cavalry API and docs offline. Prints guides on how to work (`cav guide`). Every command has `--json` output and fixed exit codes. |
| cav-bridge | A Cavalry UI script that the CLI talks to on `127.0.0.1:8723`. Requests need a secret token from `~/.cav/token`. |
| Helper library | The global `cav` inside scripts: shapes, text, keys and easing, entrances and exits, impacts, kinetic type, transitions, beat grids, and native features (duplicators, stagger, filters, 2.5D, particles). |
| `cav check` | Finds problems a viewer would notice before you render: long still stretches, small, clipped or hidden text, text that spills out of its button, off-frame layers, and empty frames at the start or end. |
| `cavalry` skill and plugins | A short entry point for agents. Packaged for Claude Code, Agent Plugins and Codex, with marketplace entries. |
| Sandbox relay | `cav relay` forwards commands through a folder, for agents whose sandbox blocks `127.0.0.1`. |
| Installers | One command on macOS and Windows. They check each download against the release checksums. `cav update` asks before it installs a new release. |

### Requirements

- Cavalry 2.4 or newer. Tested on 2.7.2 and 2.8.0.
- macOS. Windows builds and installers are included but **not tested** on Windows.
- ffmpeg, for videos with audio and for beat detection.
- Some native features need a Cavalry Pro licence. `cav.pro(type)` tells you.

### Measured results

In a blind evaluation on Cavalry 2.8.0 with a mid-size model (GLM-5.3 Flash), agents that
used `cav` passed 8 of 11 tasks, and agents that used the upstream cavalry-mcp server passed
3 of 11. The `cav` agents used about a third of the tokens. The skill did not improve on the
CLI alone, so this release moves its guidance into `cav guide`. Details: `docs/evaluation.md`.

### Known limits

- The bridge runs on Cavalry's single script thread. One slow script delays every other
  request.
- Starting or restarting the bridge needs the Cavalry window (Scripts menu).
- On Cavalry 2.8.0, inside the script run that creates a layer, a child's world bounding box
  does not follow an animated parent. Read it again in a later `cav run`.
- Connecting the Spring behaviour from a script had no visible effect in our tests. Use
  `spring` or `outBack` easing instead.
