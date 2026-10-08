# Production: plan, source, music and review

These commands work around the Cavalry scene: planning before you build, finding and
making assets and music, and getting feedback after you render. They need no Cavalry
bridge, so you can use them before Cavalry is open.

Every job works without API keys. With a key, cav uses a better service and says which
one it used. `cav config` shows the order per job; `cav config check` (or
`cav doctor --services`) tests every service with one free call.

## 1. Set up the services (once)

```bash
cav config                      # jobs, their order, and where each key lives
cav config check                # one free call per service; a missing key is a skip
cav config check --live         # also one small paid call per keyed service (asks first)
cav config set-key openai op://Private/OpenAI/api key    # or env:NAME, keychain:svc/acct
```

Local models run on this machine. mflux runs open image models (default FLUX.2 klein 4B)
through uv on Apple silicon; it is not in the default order because its first run
downloads several GB, so ask for it: `--service mflux`, or `cav config order image ...`.
ComfyUI and ACE-Step are servers the user runs:
`cav config endpoint comfyui http://127.0.0.1:8188 <workflow-api.json>` (the workflow holds
`{{prompt}}`, `"{{width}}"`, `"{{height}}"` and `"{{seed}}"`), and
`cav config endpoint acestep http://127.0.0.1:8001` for ACE-Step 1.5 (`uv run acestep-api`
in its repository; MIT licence, so its music can be used commercially).

Veo (video) and Lyria (music) use the Gemini key, so `cav config set-key gemini ...` sets
all three. Give one of them its own key with `cav config set-key veo <source>`.

fal.ai is one key for many image models. cav uses FLUX.2 (`fal-ai/flux-2`) unless you pick
another fal model id: `cav config model fal fal-ai/flux-2-pro`, `fal-ai/ideogram/v3`
(good with text in the image), `fal-ai/bytedance/seedream/v4.5/text-to-image` or
`xai/grok-imagine-image`. With reference images, cav sends them to the model's edit
endpoint (Ideogram takes them as style references). References go inline as data URIs, so
keep them small (a few hundred KB each). Models take a limited number: Grok Imagine 3,
FLUX.2 4, Seedream 10, Ideogram 10 MB in total. cav sends the first ones that fit and says
which it left out.

A key source says where the key lives; cav never prints or stores the key. If 1Password is
locked, only the services that read it fail, with a fix line. Never ask the user to paste a
key into the chat: ask them to run `cav config set-key` themselves.

| Job | Free | With a key |
|---|---|---|
| `image` (frames, stills) | greybox cards; mflux (Apple silicon), comfyui (your server) | gemini, openai, openrouter, fal |
| `image.alpha` (transparent PNG) | none | openai, recraft |
| `image.vector` (SVG) | vtracer, potrace (if installed) | recraft |
| `video` (moving animatic shots) | none (the animatic holds each frame) | veo |
| `ref` (reference images) | openverse, wikimedia | pexels, unsplash |
| `sfx` (sound search) | openverse, indexed folders | freesound |
| `music` | `cav music render` (a written score); acestep (your server); search with `cav sfx` | elevenlabs, stability, lyria |
| `ears` (critique of a track) | none | gemini, qwen-omni (your server) |

## 2. Licences come first

Set the project's licence once: `cav credits licence nc-ok` (non-commercial use) or
`cav credits licence commercial`. Searches then hide assets the project cannot use.
Every downloaded or generated asset is recorded in `.cav/manifest.json` with its licence,
author and credit line. Before you hand over a piece:

```bash
cav credits check               # NC in commercial work, share-alike, missing credits
cav credits -o CREDITS.txt      # the credit lines the licences require
```

`cav check` reports the same licence problems.

## 3. Plan: mood board, storyboard, animatic

```bash
cav ref search "neon city night"            # pick refs; each hit has a service:id ref
cav ref get openverse:<id>                  # into moodboard/, with licence and credit
cav ref arena https://www.are.na/<user>/<channel>   # the user's Are.na board, reference only
cav board mood                              # renders/moodboard.png: look at it
cav board init --bpm 120 --seconds 16 --shots 6
# edit storyboard.json: "what" per shot, "style" (prompt, refs, palette)
cav board frames                            # one frame per shot into board/
cav board sheet                             # renders/board.png: look at it
cav board motion --only s2,s5               # optional: moving clips for chosen shots (Veo)
cav board animatic --audio music.wav        # renders/animatic.mp4, cut on the beat
cav review renders/animatic.mp4             # get the user's notes before building
```

- Time shots in beats, like the plan table in `cav guide`. Put cuts on bar starts
  (multiples of 4 in 4/4). With real music, set `"grid": "build/grid.json"` from
  `cav beats music.wav --json -o build/grid.json` so beats follow the track.
- Generated frames share `style.refs`, so the look stays consistent across shots.
- `cav board motion` starts each clip from the shot's frame, so make and approve the frames
  first. It uses Veo 3.1 Lite by default (`cav config model veo veo-3.1-fast-generate-preview`
  for Fast). Veo makes 4, 6 or 8 seconds at 24 fps: cav picks the shortest length that
  covers the shot, the animatic cuts the clip to the shot, and a longer shot holds the
  clip's last frame. Veo adds its own sound; the animatic drops it and plays only the
  music. Each clip costs money, so animate only the shots where movement changes the
  decision, and name them with `--only` (or pass `--all`).
- Greybox cards are text placeholders. For a greybox made of real shapes, build it in
  Cavalry, render it with `cav frame <n> -o board/s3.png`, and set that shot's `"frame"`.
  `cav board frames` keeps frames that exist (`--force` remakes them).

## 4. Assets

```bash
cav gen image "paper-cut mountains at dusk" -o assets/mountains.png
cav gen image "flat logo mark, a fox head" --alpha -o assets/fox.png     # transparent
cav gen image "line icon of a rocket" --vector -o assets/rocket.svg       # SVG
cav gen vector assets/fox.png -o assets/fox.svg                          # trace a PNG
```

Load an image into Cavalry with `cav.image('/abs/path.png', 'name')`. Turn an SVG into
editable shapes with `api.convertSVGToLayers('/abs/path.svg')`, which returns the new
layer ids; animate them like any shape.

## 5. Music and sound

```bash
cav sfx search whoosh --max-seconds 2       # free sounds; licence and length per hit
cav sfx get freesound:<id>                  # into sfx/, with credit
cav sfx index ~/Sounds/Sonniss-GDC-2026     # add a library you downloaded
cav sfx gen "glassy riser into a hit" --seconds 2       # generated (ElevenLabs key)
cav music gen "warm synthwave, builds to a drop" --board storyboard.json --takes 2
cav music gen "warm synthwave" --board storyboard.json --service lyria   # Gemini key
cav listen music/<take>.mp3 --board storyboard.json --brief "builds to a drop on s4"
```

Lyria (`lyria-3.5`) has no length or section fields. cav writes them into the prompt: the
total length, "instrumental only", and one `[m:ss - m:ss]` line per shot. Lyria follows
them loosely, so check the cuts with `cav listen --board`. cav asks for WAV.
`cav config model lyria lyria-3-clip-preview` makes a 30-second MP3 clip instead.

### Write the music

`cav music render score.json [--stems]` plays a score you write, locally and without a key.
The score puts notes and hits on a beat grid:

```json
{"bpm": 112, "beats": 32, "swing": 0.05,
 "tracks": [
  {"name": "kick", "instrument": "kick", "hits": [0, 1, 2, 3]},
  {"name": "bass", "instrument": "bass", "notes": [[0, 0.5, 45, 0.8]], "duck": true},
  {"name": "pad", "instrument": "pad", "notes": [[0, 4, 57, 0.5]], "reverb": 0.3, "gain": -8},
  {"name": "hit", "instrument": "sample:/abs/sfx/impact.wav", "hits": [16]},
  {"name": "lead", "instrument": "vst3:/Library/Audio/Plug-Ins/VST3/Surge XT.vst3", "notes": [],
   "preset": "/abs/presets/lead.vstpreset", "params": {"cutoff": 0.4}}],
 "master": {"lufs": -14}}
```

- A note is `[beat, length in beats, MIDI note, velocity 0-1]`. Beat 0 is the start.
- Instruments: `bass`, `sub`, `pad`, `pluck`, `lead`, `keys`, `bell`; drums `kick`,
  `snare`, `hat`, `openhat`, `clap`; `sample:<path>`; `vst3:<path>` or `au:<path>`.
- Per track: `gain` (dB), `pan` (-1 to 1), `reverb` and `delay` (0-1), and `duck` to dip
  under the kick. `humanize` sets timing and velocity drift.
- A plugin track plays its default sound unless you give it a `preset`: a `.vstpreset`
  (VST3), or a file with the bytes of pedalboard's `plugin.raw_state` (VST3 or AU). `params`
  sets parameters by their pedalboard names; an unknown name lists the real ones. macOS loads
  an AU only from `/Library/Audio/Plug-Ins/Components` or `~/Library/Audio/Plug-Ins/Components`.
- `master.lufs` sets the loudness; the limiter keeps the true peak under -1 dBTP.
  `master.reference` matches the tone to a reference track (Matchering); the loudness and
  peak targets still apply after it. A very sparse mix (a lone kick) cannot reach -14 LUFS
  under that peak: the result has a `warning`. Fill the mix out, or set `master.lufs` lower.
- Write the score from the storyboard: shots start on beats, so cuts land on the music.

### Judge the music

You cannot hear. `cav listen` gives you the parts you can judge:

- **Numbers:** length against the piece, tempo, sections, and loudness (aim near
  -14 LUFS and a true peak of at most -1 dBFS for web).
- **Cuts:** each storyboard cut against the nearest downbeat, in frames.
- **A picture:** the spectrogram view with the cuts drawn green on a downbeat, red off it.
  Look at it.
- **Scores from local models**, with `--score`: Meta's Audiobox Aesthetics (production
  quality, enjoyment, complexity, usefulness, 0-10) and, with `--brief`, how well the audio
  matches the brief (LAION CLAP). The first run downloads about 2 GB.
- **A critique from an audio model**, when the `ears` job has a service. It is a model's
  opinion; say so when you pass it on.

Compare takes with `cav listen a.mp3 b.mp3 --score --board storyboard.json` and let the user
choose. Then sync with `cav beats` and `cav guide music`, and render with
`cav render --audio`.

### Finish the music in REAPER

`cav score storyboard.json --video renders/final.mp4 --audio music/take1.mp3` writes
`score/project.rpp`: the tempo, a marker per shot, the render on a video track and each take
on its own track. The user opens it in REAPER to pick a take, trim, fade and fix the sync by
ear. `cav score render score/project.rpp` renders it to a WAV for `cav render --audio`.

## 6. Review

```bash
cav review renders/final.mp4 &              # serves http://127.0.0.1:8790; keep it running
cav review wait renders/final.mp4           # blocks until the user presses Send to agent
# fix the notes, re-render to the same file, then:
cav review resolve c_01 --note "start of the wipe moved 4 frames earlier"
cav review export renders/final.mp4         # every note
```

- Give the user the URL. In the Claude Code desktop app, open it in the browser pane with a
  `.claude/launch.json` entry such as
  `{"name": "cav review", "runtimeExecutable": "cav", "runtimeArgs": ["review"], "port": 8790}`.
  In the Codex app, use the in-app browser. Otherwise any browser works.
- The page steps exact frames, shuttles with J/K/L, sets ranges with I/O, and draws
  arrows, boxes, ellipses and freehand. It shows the audio waveform under the timeline.
- When you re-render to the same file, the page loads the new render and keeps the earlier
  one. W cycles the compare modes against it: an A/B wipe, a difference image, and an onion
  skin of the frames around this one.
- Each note has a frame or range, text, drawings and a snapshot PNG with the drawing
  burned in. Look at the snapshot: it shows what the reviewer saw.
- `wait` exits with code 3 when its timeout passes with nothing sent. Run it again.
- `cav review` needs to listen on 127.0.0.1. A sandbox that blocks local ports must run
  it outside.
- The cavalry plugin for Claude Code and Codex adds a hook (`cav review hook`): when the
  reviewer presses Send to agent and no `cav review wait` picks the notes up, the next prompt
  or the end of your turn tells you once. Then run `cav review wait`.
- In Claude Code, the optional cav-review plugin also shows the open notes above the prompt,
  links to the page, and can start a turn when notes arrive.
