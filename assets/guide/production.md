# Production: plan, source, score and review

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
cav config set-key openai op://Private/OpenAI/api key    # or env:NAME, keychain:svc/acct
```

A key source says where the key lives; cav never prints or stores the key. If 1Password is
locked, only the services that read it fail, with a fix line. Never ask the user to paste a
key into the chat: ask them to run `cav config set-key` themselves.

| Job | Free | With a key |
|---|---|---|
| `image` (frames, stills) | greybox cards | gemini, openai, openrouter |
| `image.alpha` (transparent PNG) | none | openai, recraft |
| `image.vector` (SVG) | vtracer, potrace (if installed) | recraft |
| `ref` (reference images) | openverse, wikimedia | pexels, unsplash |
| `sfx` (sound search) | openverse, indexed folders | freesound |
| `music` | none: search with `cav sfx` | elevenlabs, stability |
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
cav board mood                              # renders/moodboard.png: look at it
cav board init --bpm 120 --seconds 16 --shots 6
# edit storyboard.json: "what" per shot, "style" (prompt, refs, palette)
cav board frames                            # one frame per shot into board/
cav board sheet                             # renders/board.png: look at it
cav board animatic --audio music.wav        # renders/animatic.mp4, cut on the beat
cav review renders/animatic.mp4             # get the user's notes before building
```

- Time shots in beats, like the plan table in `cav guide`. Put cuts on bar starts
  (multiples of 4 in 4/4). With real music, set `"grid": "build/grid.json"` from
  `cav beats music.wav --json -o build/grid.json` so beats follow the track.
- Generated frames share `style.refs`, so the look stays consistent across shots.
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
cav listen music/<take>.mp3 --board storyboard.json --brief "builds to a drop on s4"
```

You cannot hear. `cav listen` gives you the parts you can judge:

- **Numbers:** length against the piece, tempo, sections, and loudness (aim near
  -14 LUFS and a true peak of at most -1 dBFS for web).
- **Cuts:** each storyboard cut against the nearest downbeat, in frames.
- **A picture:** the spectrogram view with the cuts drawn green on a downbeat, red off it.
  Look at it.
- **A critique from an audio model**, when the `ears` job has a service. It is a model's
  opinion; say so when you pass it on.

Compare takes with `cav listen` and let the user choose. Then sync with `cav beats` and
`cav guide music`, and render with `cav render --audio`.

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
  arrows, boxes, ellipses and freehand.
- Each note has a frame or range, text, drawings and a snapshot PNG with the drawing
  burned in. Look at the snapshot: it shows what the reviewer saw.
- `wait` exits with code 3 when its timeout passes with nothing sent. Run it again.
- `cav review` needs to listen on 127.0.0.1. A sandbox that blocks local ports must run
  it outside.
