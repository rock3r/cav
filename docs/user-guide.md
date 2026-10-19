# cav user guide

`cav` builds and renders motion graphics in [Cavalry](https://cavalry.studio) from a
terminal. A person or coding agent writes JavaScript files. `cav` runs them in the open
Cavalry app, makes contact sheets, checks common mistakes and renders video.

Running `cav` requires macOS or Windows, Cavalry 2.4 or newer, and ffmpeg. The project
tested native scene work on macOS with Cavalry 2.7.2 and 2.8.0. Windows installation and
offline commands were tested. Windows native scene work remains untested. 2.5D cameras,
physics and particles require Cavalry Pro.

## Install

**1. Install the CLI.** On macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/rock3r/cav/main/install.sh | sh
```

On Windows, in PowerShell:

```powershell
irm https://raw.githubusercontent.com/rock3r/cav/main/install.ps1 | iex
```

On macOS, the installer keeps the signed Cav.app in `~/.local/bin/.cav-bundles` and
links `~/.local/bin/cav` to its executable. See
[macOS distribution](NOTARIZATION.md) for migration from old updaters.

The installer places `cav` in `~/.local/bin` on macOS or `%LOCALAPPDATA%\cav\bin`
on Windows. It checks release checksums and runs `cav setup`. Setup creates the secret
token in `~/.cav/token`, installs the bridge script and checks Cavalry and ffmpeg.
`cav setup` can be run again.

If ffmpeg is missing, install it with `brew install ffmpeg` (macOS) or
`winget install Gyan.FFmpeg` (Windows).

**2. Start the bridge.** Open Cavalry and click **Scripts > cav-bridge**. Keep the bridge
window open while using `cav`. Cavalry asks whether to trust the script on its first run.
Start the bridge again after each Cavalry restart.

**3. Check everything.**

```bash
cav doctor
```

Each successful check says `ok`. A `FAIL` line includes a `fix:` instruction.

**4. Optional: build the docs index.** `cav docs update` downloads about 520 Cavalry
documentation pages for offline `cav docs <words>` searches. Setup does this unless
it received `--no-docs`.

## Your first animation

Create a scene. `cav` refuses to replace unsaved scene changes.

```bash
cav scene new --width 1920 --height 1080 --fps 60 --seconds 4
```

Save this as `title.js`:

```js
var c = cav.comp()
var beat = cav.beats(120)                 // beat.beat(n) is the frame of beat n (beat 0 is frame 0)
cav.plane('bg', '#0b0d12')
var title = cav.text('title', 'LAUNCH DAY', 140, { font: 'Inter', style: 'Black', color: '#f5f7fa', y: 40 })
var bar = cav.rect('underline', 520, 10, { fill: '#ff4d2e', y: -60, radius: 5 })

cav.slideIn(title, beat.beat(0), { dy: -80, dur: 20, ease: 'outBack' })
cav.wipeIn(bar, beat.beat(1), { dur: 18, from: 'left' })
for (var b = 2; b <= 6; b++) cav.punch(title, beat.beat(b))   // a small hit on every beat
cav.slideOut(title, beat.beat(7), { dy: 60, dur: 16 })
cav.fadeOut(bar, beat.beat(7), 12)
return { title: title, bar: bar }
```

Run the script, inspect frames, check the scene and render it:

```bash
cav run title.js          # prints the returned layer ids
cav sheet                 # renders 12 frames into renders/sheet.png
cav check                 # lists problems a viewer would notice
cav scene save out/title.cv
cav render -o out/title.mp4
```

For this example, `cav check` reports no movement during about half the piece. A slow
underline movement could address that finding if it fits the design.

Open `renders/sheet.png` after every change. Check for missing layers, wrong positions
and wrong colours.

## How scripts work

Each script runs inside a Cavalry function. `return` sends a value to the terminal.
Two libraries are available:

- `api` is Cavalry's own scripting API. `cav api <words>` searches it offline, for example
  `cav api keyframe`.
- `cav` is the helper library. `cav helpers` prints all of it; `cav helpers slideIn` prints
  one entry.

Follow these four rules:

- `(0, 0)` is the centre of the comp, and `+y` points **up**. The top edge is `cav.comp().top`.
- Frames are whole numbers, opacity is 0 to 100, scale is a multiplier (1 = 100 %), rotation
  is in degrees.
- Variables do not survive between `cav run` calls. Look layers up again with
  `cav.find('name')`, or store ids on `globalThis`.
- Prefer the helpers to raw `api` calls. They avoid several Cavalry behaviours that fail
  silently, such as colour keys given as hex strings or keys on `rotation`. Use
  `rotation.z` for rotation keys.

A long script can exceed the CLI wait budget. `scene open`, `frame`, `sheet`, `run`,
`render` and `check` print operation IDs. Inspect and resume the same operation:

```sh
cav operation status <operation-id> --json
cav operation resume <operation-id> --timeout 30m --json
```

`cav job wait <job-id>` waits for one raw job. Waiting for preparation metadata does
not continue a render. Keep the scene and inputs unchanged during recovery. See
[operation recovery](recovery.md) for unknown outcomes, partial checks and stale records.

## Everyday commands

| Task | Command |
|---|---|
| Inspect existing bridge state | `cav status` |
| Retrieve scene metadata and structure | `cav scene info`, `cav tree` |
| Inspect a layer (position, bounding box, keys) | `cav layer <id>`, `cav layer <id> --attrs` |
| Change the comp size, length or background | `cav scene comp --seconds 8 --bg '#101014'` |
| Make a 9:16 or 1:1 copy of a comp | `cav scene reframe --to 9:16` (`--dry-run` first; the original comp is not changed) |
| Render one frame | `cav frame 90 -o renders/f90.png` |
| Review chosen frames | `cav sheet 0,30,60,90`, or every 30th frame: `cav sheet 0-600:30` |
| See how one move travels (path, easing, overshoot) | `cav onion 0-60` writes `renders/onion.png` |
| Find problems before rendering | `cav check` |
| Render with music | `cav render -o out/final.mp4 --audio music.wav` |
| Look at a track, or check a render against it | `cav spectrogram music.wav`, `cav sync out/final.mp4` |
| Find a layer type id | `cav types gradient` |
| Search the docs | `cav docs stagger` |
| Read how to work, or motion recipes | `cav guide`, `cav guide design` |
| Find the guide section for a symptom | `cav guide --search "rotation keys ignored"` |

Every command accepts `--json` for machine-readable output.

## Syncing to music

`cav beats` finds tempo, beats and downbeats in a track. A downbeat is the first beat
of a bar. The command prints their frame numbers:

```bash
cav beats music.wav --fps 60 -o grid.json
```

Place cuts and major hits on downbeats, and smaller accents on other beats. Inspect
the timed frames:

```bash
cav sheet 0-960:36 --bpm 100
```

With `--bpm`, each tile shows its beat number.

Detection can return half or twice the expected tempo. If it does, run again with
`--bpm <what you expect>`. Without an audio file,
`cav beats --bpm 120 --seconds 16` creates an exact grid.

With a file, `cav beats` also lists the sections of the track (where the music changes), sudden
rises and falls in loudness, silences, and the strongest accents with their frequency band.

These commands give an agent visual and measured audio information:

```bash
cav spectrogram music.wav --fps 60            # renders/spectrogram.png
cav sync renders/final.mp4                    # renders/sync.png and a report
```

`cav spectrogram` draws frequencies, bars, sections, loudness and low, mid and high
onsets on one time axis. `cav sync` finds cuts and visual hits in a rendered video.
It measures each against beats in frames and lists strong musical events without a
visual hit. The sync image adds per-frame visual change below the spectrogram.

For continuous music-driven movement, `cav.sound` connects Cavalry's Sound behaviour
to an attribute: `cav.sound('/abs/music.wav', layer, 'scale.y', {min: 1, max: 1.4, low: 20, high: 150})`.

## Planning, assets, music and review

Planning, asset, music and review commands work without the Cavalry bridge.
`cav board place` needs it to add storyboard frames to the open scene. Read
`cav guide production` for full command details.

**Services and keys.** Each job type has a service order: frames, transparent assets,
SVG, references, sound search, music and audio critique. cav chooses the first available
service. Every job type also has a path without keys:

```bash
cav config                                          # the order per job, and key sources
cav config set-key gemini op://Private/Gemini/api key
cav config check                                    # one free call per service
cav config check --live                             # plus one small paid call each
```

Keys can come from `env:NAME`, `keychain:service/account`,
`op://vault/item/field` (read with `op read`) or `oauth` through
`cav config login freesound`. cav does not print or store key values.

On macOS, `keychain:service/account` reads a generic password from the login keychain. On
Windows, it reads a generic credential from Credential Manager. The target name is the
service, and the user name is the account. Save one with `cmdkey`; it asks for the key:

```bash
cmdkey /generic:cav /user:openai /pass
cav config set-key openai keychain:cav/openai
```

cav reports missing entries and refused reads on macOS and Windows. Linux does not
support these keychain sources.

**Licences.** Set the project's allowed use with `cav credits licence nc-ok` or
`cav credits licence commercial`. cav records downloaded and generated assets in
`.cav/manifest.json`. `cav credits` prints credit lines. `cav credits check` and
`cav check` report licence problems.

**Storyboard and animatic.**

```bash
cav ref search "neon city night" && cav ref get openverse:<id>   # into moodboard/
cav ref add https://www.youtube.com/watch?v=...  # a web video: embedded, never downloaded
cav board mood                                   # moodboard.json and renders/moodboard.png
cav board mood take <id> "what to take from it"  # the line shown under the item
cav board init --bpm 120 --seconds 16 --shots 6  # then describe each shot in storyboard.json
cav board frames                                 # generated with a key, else your board/<id>.svg sketches, else greybox
cav board sheet                                  # renders/board.png
cav board animatic --audio music.wav             # renders/animatic.mp4, cut on the beat
cav board place                                  # frames as placeholder layers (needs the bridge)
```

**Assets.** `cav gen image "<prompt>"` makes a still. `--alpha` requests a transparent
PNG, and `--vector` requests an SVG. `cav gen vector logo.png` traces a PNG. On Apple
silicon, mflux runs FLUX.2 klein 4B without a key. Its first run downloads about 16 GB.
After a first `--service mflux` run, cav includes it after keyed services by default.
`cav config endpoint comfyui` connects a user-run ComfyUI workflow.

**Music and sound.** `cav sfx search whoosh` finds free sounds.
`cav sfx get <ref>` downloads one with its credit, and `cav sfx index <folder>` adds a
user-owned library. `cav sfx gen "<prompt>"` generates a sound effect with an ElevenLabs
key. On Apple silicon, `--service sa3` makes it locally with Stable Audio 3 instead: no
key, nothing uploaded, the first run downloads about 1.9 GB of models. Once those are
downloaded, a plain `cav sfx gen` without a key uses sa3 rather than failing.

The local library search matches file names. `cav sfx search --by-sound` ranks the local
files by what they sound like instead. It runs earshot's CLAP model locally through uv;
the first run downloads about 1.4 GB of models, and after that it works offline.
`cav sfx search --service bank` searches the sound bank that earshot ships: 201 short
synthesized sounds for motion graphics. They render locally in about half a second with
no models and no network; the first use downloads 4 MB of earshot source. After that
first use the bank joins every plain search. `cav sfx get bank:<id>` renders a take.
`--seconds 1.5` sets an exact length, and `--seed 7` gives another take of the same
sound. The manifest records the licence: MIT for the synthesized sounds, and CC0 with
the author and link for the ones fitted from a recording.

With a key, `cav music gen` makes takes based on storyboard shots. On Apple silicon,
`--service sa3` makes them locally with Stable Audio 3: no key, nothing uploaded, an
exact `--seconds` length, and a random seed per take that the manifest records. The
first run downloads about 1.9 GB of models (the `medium` model, about 6.5 GB, runs to
6:20; `cav config model sa3 medium`); after that sa3 joins the default order, so a
plain `cav music gen` works with no key at all. Takes are under the Stability AI
Community License: free commercial use below $1M annual revenue, recorded in the
manifest. A user-run ACE-Step
1.5 server can do this without a key. A user-run YuE2 server uses the storyboard's
length and tempo. It does not follow the shots. YuE2 runs through `tools/yue2-server`
or YuE2-Turbo on Apple silicon or an NVIDIA GPU. Turbo is fastest on a large NVIDIA
GPU with vLLM. In one test, a 60-second take took 71 s on an RTX 3080 Ti.
It took about 8 minutes on an M1 Max. cav records YuE2 takes as non-commercial; individual creators
have an extra permission under YuE2's terms. See `cav guide production`.

`--ref <track>` uses an existing track. Stable Audio and ACE-Step receive its audio.
YuE2 makes an instrumental cover from a melody that SheetSage2 transcribes on the server.
Lyria and ElevenLabs receive a text description. cav records cover sources with each take,
and `cav credits` lists them. Takes are instrumental unless `--lyrics song.txt` supplies
sections such as `[Verse]` and `[Chorus]`. YuE2 and ACE-Step can sing them; other
services refuse the option. YuE2 uses lyrics to set song length and can sing them to
the `--ref` melody.

`cav music render score.json` plays and masters a written beat-grid score. It supports
built-in synths, drums, samples and VST3/AU instruments.
`cav listen <track> --board storyboard.json` reports length, tempo, loudness, cut timing,
an image and a written critique. The critique needs a Gemini key or Qwen3-Omni through
OpenRouter:
`cav config endpoint qwen-omni https://openrouter.ai/api/v1 qwen/qwen3.8-omni-flash`, then
`cav config set-key qwen-omni <source>` with an OpenRouter key (qwen-omni does not use
the openrouter key on its own).
`--score` adds local quality scores. `cav listen` can compare several tracks in one
report. `cav score` creates a REAPER project for listening and final edits.

**Review.** `cav review renders/final.mp4` serves a page at http://127.0.0.1:8790.
Use arrow keys for frame steps, J/K/L for shuttle and I/O for ranges. Draw on frames
and add notes. "Send to agent" delivers open notes to `cav review wait`. Notes and
marked snapshots live in `renders/final.review.json` and its review folder. Rendering
to the same video path reloads the page. W cycles through wipe, difference and
onion-skin comparisons with the earlier render.

**Brief.** `cav brief init` creates `brief.json` with title, goal, audience, tone,
length, tempo, format, deliverables, required and avoided content, music and references.
The agent completes it with the user. `cav brief show` lists problems and empty fields.
`cav board init` and `cav scene new` read size, fps and length from the brief and report
the values used. A command flag overrides the brief. `--no-brief` ignores it.

`cav review` also supports `brief.json`, `moodboard.json` and `storyboard.json` before
slow or paid work. Mood board items can play clips and YouTube or Vimeo videos. The
agent's "take from it" line appears below each item. The grid contains brief fields,
mood board references or storyboard shots. Set Approve or Redo for brief and storyboard
items. Set Keep, More like this or Drop for mood board items. Open an item to draw or
write a note. The Approve button works when no item is Redo or Drop and no note is open.
Approval is stored in the stage's review JSON. Changing an item makes its old verdict
outdated and removes approval. The user must review it again; the agent cannot approve
for them. `cav review wait` returns on approval, and
`cav review export storyboard.json` reports the approval state.

### Review in the chat

`cav mcp` serves the review page inside supported chats as an
[MCP App](https://modelcontextprotocol.io/extensions/apps). It uses stdin and stdout.
The `cavalry` plugin registers it in Claude Code, Codex and Antigravity. It requires
cav 1.2 or newer. With an older CLI, the server does not start. Other plugin features
still work.

Without the plugin, register the server once. Duplicate registration shows its tools twice.

In Claude Code:

```bash
claude mcp add cav -- cav mcp
```

In Codex:

```bash
codex mcp add cav -- cav mcp
```

This writes the following to `~/.codex/config.toml`. The same configuration can be added by hand:

```toml
[mcp_servers.cav]
command = "cav"
args = ["mcp"]
```

In the chat of the Claude desktop app, add the server to
`~/Library/Application Support/Claude/claude_desktop_config.json`. Quit the app first:
it writes this file when it quits, and drops changes made while it runs. Give the full
path to `cav`, because the app may not search `~/.local/bin`:

```json
{
  "mcpServers": {
    "cav": { "command": "/Users/you/.local/bin/cav", "args": ["mcp"] }
  }
}
```

Keep other configuration keys. In Claude desktop chat, the server may start outside
the project folder. Give the agent the video's full path.

Ask the agent to show a render's review. It calls `show_review`. A chat with MCP Apps
displays the page, where the user can play, step, draw, write notes and press
"Send to agent". Notes go to the same `<video>.review.json` used by `cav review wait`,
`export`, `resolve` and the review hook. Supported chats also prepare a message telling
the agent to read notes. Claude desktop puts that message in its input box with a
caution; the user sends it.

The agent can call `review_notes` to receive sent notes and their snapshots. The tool
marks the send received, as `cav review wait` does. Chats without shell access need
this tool because they cannot run `cav review wait`.

The chat frame has no network access. The server therefore sends a lower-quality preview
inline, at most 640 pixels wide and 24 MB. It caches the preview in
`~/.cav/cache/review-preview`. Review longer renders in the browser. Use the chat for
short checks and notes, and `cav review` for detailed frame review. A/B comparison in
chat works only with earlier renders also shown there.

Some chats do not display MCP Apps. In October 2026, Claude desktop chat displayed
the page. Its Code tab ran the tool without displaying it. In that case, the
agent still receives open notes as text and structured data. The server resolves a
relative video path from its starting folder.

Antigravity's MCP App display remains untested. If it cannot display the page,
`show_review` still returns open notes and the video path as text. It does not return
a page URL. Run `cav review` with the video path in a shell for the browser page.
Antigravity prefixes these tools with `cavalry_cav`. The skill does not depend on
those tool names.

## Using cav with a coding agent

`cav guide` prints the scene guide. `cav guide flow` prints production stages. Other
topics cover traps, entrances, impacts, type, transitions and beat timing. The
`cavalry` skill tells the agent when to use cav, how to check setup and where to read
the guides. For a silent failure, `cav guide --search <symptom>` returns matching
sections across guides, capped at about 4 KB.

### Your own flow

`cav guide flow` prints the active stages. The standard flow contains brief, mood board,
storyboard, music and animatic, and build. A project `flow.md` can replace them. cav
checks the project file first, then `~/.cav/flow.md`, then its built-in standard flow.
A flow file replaces all standard stages. Each stage starts with `## name`:

```markdown
## style frames
Make three frames before anything else. Show them to me.
outputs: style/*.png
review: no
use: cav gen image
```

- `outputs:` lists the files the stage writes, separated by commas. A path is relative to the
  project. `*` matches inside one folder. A path that ends in `/` is a folder.
- `review: yes` or `review: no` says whether the user approves the stage in `cav review`.
  The approval belongs to the first output, which must then be one `.json` file.
- `use:` is a note for the agent, for example the commands to use. cav prints it and never
  runs it.

cav ignores lines inside code blocks and `<!-- -->` comments. Other text gives the agent
instructions. If a flow file has an error, `cav guide flow` still prints it and warns
on stderr with the line number. To adapt the standard flow, copy
`assets/flow/standard.md` from the source. `cav guide flow` also prints that file.

`cav plan` checks each stage against the current project's files and reports one state:

- `done`: the outputs exist and, if the stage has a review, the user approved them.
- `review`: the outputs exist and lack approval. The line names shots marked redo and open
  notes, and says when an edit removed the user's approval.
- `check`: cav cannot tell. The review file does not load, or the flow asks for a review that
  cannot hold an approval. Examples include a video, folder or pattern as the first output.
- `notes`: a stage without a review has open notes.
- `todo`: an output is missing.
- `skipped`: no output exists for this stage although a later stage already has output.
- `unknown`: the stage has no `outputs:` line, so cav cannot check it. It is never `done`.

The last line names the next step. `cav plan --json` returns the same facts as data.
The command only reads local state. It writes no file, uses no paid service and blocks
no other command.

Install the skill in Claude Code:

```text
/plugin marketplace add rock3r/cav
/plugin install cavalry@cav
```

In Codex:

```bash
codex plugin marketplace add rock3r/cav
```

```bash
codex plugin add cavalry@cav
```

In the Antigravity CLI, install the plugin from a clone of this repository:

```bash
git clone https://github.com/rock3r/cav
agy plugin install ./cav/plugins/cavalry
```

Check the plugin with `agy plugin validate ./cav/plugins/cavalry`. It reports one skill, one
MCP server and one hook. The project has not yet tested `agy plugin install` with the
repository URL. The plugin is in a subfolder, and Antigravity does not document how it
reads another marketplace.

`agy plugin install` installs the plugin for the CLI only, in
`~/.gemini/antigravity-cli/plugins/`. The Antigravity app and the Antigravity IDE read plugins
from other folders. For them, copy the plugin folder into `~/.gemini/config/plugins/` (all
workspaces) or into `.agents/plugins/` at the root of one workspace:

```bash
mkdir -p ~/.gemini/config/plugins
cp -R ./cav/plugins/cavalry ~/.gemini/config/plugins/cavalry
```

The Antigravity plugin contains three parts:

- the `cavalry` skill;
- the `cav mcp` server (from `mcp_config.json`), with the `show_review` and `review_notes`
  tools;
- the review hook (from `hooks.json`). When the reviewer presses "Send to agent" and no agent
  reads the notes, the hook tells the agent once. Before a model call, it adds a short
  message for the agent. When the agent wants to stop, it asks the agent to continue and
  read the notes. The hook needs a `cav` that knows `cav review hook --host antigravity`.
  With an older `cav`, the hook does nothing.

Antigravity limits:

- Only the Antigravity CLI runs plugin hooks. The Antigravity app and the Antigravity IDE run
  only the hooks in a workspace or global `hooks.json`, so there the review hook does not run.
  Use `cav review wait`, or the `review_notes` tool when the plugin is installed for the app
  or IDE as shown above.
- The plugin has been checked only with `agy plugin validate`. An end-to-end test in
  Antigravity remains pending.

For another agent, copy `plugins/cavalry/skills/cavalry` into its skills folder.

In tests, a mid-size model did as well with the CLI alone as with the CLI and the old,
longer skill. The measured results in the README explain the shorter skill.

### Agents in a sandbox

Some agent sandboxes block `127.0.0.1` and writes outside the project. `cav` can use a
shared folder and relay:

1. Outside the sandbox, start a relay on a folder the agent can write:

   ```bash
   cav relay --spool /path/to/project/.cav/spool
   ```

2. Inside the project, create a file named `.cav-spool` that contains that path, or set
   `CAV_SPOOL=/path/to/project/.cav/spool` for the agent.

The agent then runs normal `cav` commands. Each command goes into the folder. The relay
runs it outside the sandbox and returns the output. `--restricted` blocks scripts from
starting other programs. It helps prevent mistakes. Scripts still have Cavalry API file
access, so this restriction cannot enforce a security boundary.

## Troubleshooting

| What you see | Why | What to do |
|---|---|---|
| `FAIL bridge  not running on 127.0.0.1:8723` | The bridge script is not open in Cavalry. | In Cavalry: **Scripts > cav-bridge**. Keep its window open. |
| `listening ... but not answering` | Cavalry is busy with a long script or render, or a dialog box is open. | Wait for the job, or close the dialog in Cavalry, then try again. |
| `running v1.0.0; v1.1.0 is installed` | The `cav` CLI was updated. Cavalry still runs the old bridge, which still works. | Close the cav-bridge window and start it again from the Scripts menu. |
| `cav and cav-bridge do not match` or `speaks protocol` | The running bridge and `cav` use different request formats. `cav` will not send jobs. | Run `cav setup`, then close the cav-bridge window and start it again from the Scripts menu. |
| Exit code 3, pending/unknown | The job took longer than `--timeout`. | `cav operation status <id>`, then `cav operation resume <id>`. Do not retry the original command. |
| Exit code 4, bridge lost/session changed | The original result is unresolved. | Stop automatic waiting, preserve partial output, and inspect the operation. File growth is not completion; reconcile before a new render. |
| `unknown job` | The id is wrong, or the result is older than a day. | Check the id. `cav job wait` without an id waits for the most recent job. |
| "Cavalry is busy (not answering)" during a job | A Cavalry operation is blocking the app, for example deleting hundreds of layers. | Keep waiting. `cav` gives up only after 120 s of silence. |
| A layer appears in the wrong place | `+y` is up, and `(0, 0)` is the centre. | Check positions with `cav layer <id>`. |
| Text at the wrong position after a slide | Text is centred on its `x` by default. | Pass `align: 'left'` when `x` is the left edge. |
| Physics or particles do not move in a sheet | They simulate only when frames render in order. | Use a step-1 range: `cav sheet 0-59:1`. |
| `blocked: a sandbox` | An agent's sandbox stops `cav` from reaching `127.0.0.1` or writing `~/.cav`. | Allow `cav` to reach `127.0.0.1:8723` and write `~/.cav`, or use a relay (see "Agents in a sandbox"). |
| `waiting for cav relay` | The agent is in spool mode and the relay is missing. | Start `cav relay --spool <folder>` outside the sandbox. |
| `cav rect` or `cav text` is an unknown command | Helpers run inside scripts. The terminal does not expose them as commands. | Write the call in a `.js` file and run it with `cav run`. |

The Cavalry JavaScript Editor and `cav` share one script thread. A long script from
either tool delays the other.

## Updating

```bash
cav version --check       # tells you whether a newer release exists, changes nothing
cav update                # shows the release, asks, installs, then runs cav setup
```

`cav doctor` reports new releases on its `update` line with the fix `cav update`.
It checks GitHub at most once an hour, including after failed checks. It stores the
answer in `~/.cav/update-check.json`. A failed update check does not fail `cav doctor`.
Set `CAV_NO_UPDATE_CHECK=1` to disable it.

`cav` asks before updating. If `cav doctor` reports an older running bridge after an
update, restart its Cavalry window. Update the Claude Code plugin with
`/plugin marketplace update cav`. To update the Antigravity plugin, pull the latest changes in
the local clone, then run `agy plugin install ./cav/plugins/cavalry` again. If the plugin
was copied for the Antigravity app or IDE, replace that copy too: delete
`~/.gemini/config/plugins/cavalry` (or `.agents/plugins/cavalry`) and copy the folder again.

## Uninstalling

1. Delete the `cav` binary: `~/.local/bin/cav` on macOS, `%LOCALAPPDATA%\cav\bin\cav.exe` on
   Windows (or the folder configured with `CAV_BIN_DIR`).
2. Delete `~/.cav` (the token, job results and docs cache).
3. Delete `cav-bridge.js` from Cavalry's Scripts folder: macOS
   `~/Library/Application Support/Cavalry/Scripts`, Windows `%APPDATA%\Cavalry\Scripts`.
4. In Claude Code, remove the plugin with `/plugin uninstall cavalry@cav`. In the Antigravity
   CLI, run `agy plugin uninstall cavalry`. If the plugin was copied for the Antigravity app or
   IDE, also delete `~/.gemini/config/plugins/cavalry` or `.agents/plugins/cavalry` in the
   workspace.

## Reference

Exit codes:

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Error (for example, the script threw) |
| 2 | The bridge cannot be reached |
| 3 | Wait budget expired; native outcome pending or unknown |
| 4 | Bridge lost/refused or operation session changed; preserve partial output and inspect |

Environment variables:

| Variable | Effect |
|---|---|
| `CAV_HOME` | Where `cav` keeps its token, job results and caches (default `~/.cav`). |
| `CAV_SPOOL` | Work through a spool folder and `cav relay`. |
| `CAV_LOG` | Append a JSON line for every `cav` call to this file. |
| `CAV_OUT_DIR` | Default folder for sheets and frames (default `renders`). |
| `CAV_BRIDGE_PORT` | The bridge port (default 8723). |
| `CAV_SCRIPTS_DIR` | Cavalry's Scripts folder, if it is not in the usual place. |
| `CAV_NO_KEYS` | Any value makes every key missing: cav reads no environment variable, keychain entry or 1Password item, and uses only free services. |
| `CAV_NO_UPDATE_CHECK` | Any value stops `cav doctor` from asking GitHub for a newer release. |

Use `cav check --quick --json` for structural performance warnings. Add `--profile`
for sampled measurements or `--profile-render` for low-resolution PNG timings.
Default checks have limits. Read `failures`, `skipped`, `complete` and `clean` before
calling a scene reviewed. Inspect a sheet for blank starts and ends. For known traps,
such as missing fonts or SkSL input declarations, `cav check` prints the relevant guide
section.

Quick structural checks include referenced pre-comps and their composition paths.
For later frames, use
`cav check --profile-frames 840,1560,1908 --max-warmup 2000 --timeout 30m --json`.
Simulation targets render at 10 percent after chronological warm-up from comp start.
Reports separate warm-up cost from sample timings. The extra-frame allowance defaults
to 600. Begin with the small default sample and repeat measurements before claiming
a performance improvement.

Docs search gives priority to the longest full page title at the query start. For
example, `cav docs "Look At duplicator"` ranks Look At sections first, then uses
query relevance. API search still ranks function names.


## Long scenes and launch-video checks

```sh
cav scene new --range 0-3599 --fps-preset smooth
cav scene info
cav check --quick --comp section
cav frames 0-599 --keep-every 10 --comp section -o out/preview
cav render --save --chunk-frames 600 --audio track.wav -o out/final.mp4
cav seams --video out/final.mp4 --allow-cuts 600,1200
```

`scene info` reports layer nodes, comp nodes and key counts. `countsComplete` reports
whether its limited scan finished. Key-volume and shader warnings identify risks for
measurement. They do not establish which node slowed a render.

For frame, sheet, frames, check and seams, `--comp` accepts a node ID or unique comp
name. The command restores the selected comp's playhead and the original comp and
playhead after success or native failure. Duplicate names cause an error. Comp switching
can mark a scene unsaved, so save before chunk rendering.

`frames` sorts requested frames and evaluates intervening frames from comp start. It
keeps every Nth requested frame. By default, it allows 10000 evaluations and at most
1000 saved PNGs. Other frames reuse one warm-up PNG. Increase `--max-evaluated` when
a longer preview needs it. Sparse `frame` and `sheet` calls skip intervening frames.

`seams` compares frames f-1 and f at layer in/out and opacity-key boundaries. A
finished full-comp video keeps simulation state; its frame zero must match comp start.
Native simulation sampling evaluates frames in order. Changed-pixel fractions and
boxes identify boundaries to review. Intended cuts remain visible with `review:false`.
Reports show omitted metadata and boundary limits. Use `--comp` for referenced sections.
The command cannot infer reference time remapping. Pixel boxes refer to sampled images.

Chunk rendering requires a saved named scene, ffmpeg and ffprobe. Each new chunk
replays from comp start within `--max-warmup`. cav checks completed segments by frame
count and hash before joining them, adding audio and publishing. It hashes the source
scene again before every chunk. Staging remains beside the destination for recovery.

After a timeout, run `operation resume ID` on the same render. If Cavalry or its bridge
restarted, inspect the prior outcome and reopen the saved scene. Then run
`operation resume ID --restart-chunk --acknowledge-unknown-outcome`. This requires a
different idle bridge session. cav keeps evidence from uncertain jobs, reuses checked
segments and assigns a new ID to reconciled chunk work. It does not cancel or
automatically resubmit native work. The limit is 1000 chunks.

`cav helpers` and `cav guide native` explain guarded SkSL, pre-comps, font axes,
two-key drivers, typed per-copy drivers, connected Look At and stack-order helpers.
`cav version` checks the installed bridge file offline. `cav status` reports warnings
from its probe of the running bridge.
