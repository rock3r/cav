# Production: plan, source, music and review

These commands work around the Cavalry scene: planning before you build, finding and
making assets and music, and getting feedback after you render. They need no Cavalry
bridge, so you can use them before Cavalry is open. `cav board place` is the exception: it
adds the storyboard to the open scene.

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
rembg cuts the background out of an image with BiRefNet (`birefnet-general`), on any
platform with uv. Its first run downloads the model, about 1 GB, into `~/.rembg`. It is
last in the `image.alpha` order: OpenAI and Recraft make the transparency while they
generate, which keeps soft edges and shadows, and they need no second step.
ComfyUI, ACE-Step and YuE2 are servers the user runs:
`cav config endpoint comfyui http://127.0.0.1:8188 <workflow-api.json>` (the workflow holds
`{{prompt}}`, `"{{width}}"`, `"{{height}}"` and `"{{seed}}"`), and
`cav config endpoint acestep http://127.0.0.1:8001` for ACE-Step 1.5 (`uv run acestep-api`
in its repository; MIT licence, so its music can be used commercially), and
`cav config endpoint yue2 http://127.0.0.1:8002` for YuE2 (the server in cav's
`tools/yue2-server`; see "YuE2" below for setup and its licence).

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
| `image.alpha` (transparent PNG) | rembg (through uv): cuts out an image that an `image` service made | openai, recraft |
| `image.vector` (SVG) | vtracer, potrace (if installed) | recraft |
| `video` (moving animatic shots) | none (the animatic holds each frame) | veo |
| `ref` (reference images) | openverse, wikimedia | pexels, unsplash |
| `sfx` (sound search) | openverse, indexed folders | freesound |
| `music` | `cav music render` (a written score); acestep, yue2 (your servers); search with `cav sfx` | elevenlabs, stability, lyria |
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
cav board place                             # the frames as placeholder layers in Cavalry
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
- `cav board place` starts the build. It adds each frame to the active composition as an
  image layer named by its shot id (`s1`, `s2`, ...), shown from its start beat to its end
  beat at the composition's frame rate, and scaled to fit. Time 0 of the music is the
  composition's first frame, so beat 0 sits at the storyboard's `offset` (or the grid's
  first beat), as in the music. Find a placeholder with
  `cav.find('s3')`. When you build the shot, delete its placeholder and give the shot's
  group the same name. Run `cav board place` again after you change beats or frames: it
  updates the image layers in place (their parent, transforms and masks stay; delete a
  placeholder to have it fitted again) and keeps any shot whose name a built layer
  already uses.
  `--only s2,s3` places some shots; `--comp NAME` targets another composition.

## 4. Assets

```bash
cav gen image "paper-cut mountains at dusk" -o assets/mountains.png
cav gen image "flat logo mark, a fox head" --alpha -o assets/fox.png     # transparent
cav gen image "line icon of a rocket" --vector -o assets/rocket.svg       # SVG
cav gen vector assets/fox.png -o assets/fox.svg                          # trace a PNG
cav gen cutout photo.png -o assets/fox.png                               # remove a background
```

`--alpha` with rembg works in two steps: the first usable `image` service (for example
Gemini) makes the image, then rembg cuts it out on this machine. Ask for a plain,
even background in the prompt; the cutout is cleaner. Without an image key there is
nothing to cut out, so cav says so. `cav gen cutout` works on any image you have, with
rembg or Recraft (OpenAI cannot cut out an image it did not make). A cutout keeps the
licence and credit of the image it came from. An image that is not in the manifest gives
a cutout with an unknown licence, so `cav credits check` asks about it.

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
cav music gen "toy piano pop, more playful" --ref music/pixel-pop-a.mp3 --takes 2 # variations
cav music gen "English piano pop, warm female voice, 88 BPM" --lyrics song.txt --service yue2  # a song
cav listen music/<take>.mp3 --board storyboard.json --brief "builds to a drop on s4"
```

Lyria (`lyria-3.5`) has no length or section fields. cav writes them into the prompt: the
total length, "instrumental only", and one `[m:ss - m:ss]` line per shot. Lyria follows
them loosely, so check the cuts with `cav listen --board`. cav asks for WAV.
`cav config model lyria lyria-3-clip-preview` makes a 30-second MP3 clip instead.

The music is instrumental unless you give `--lyrics`; see "Songs with lyrics" below.

`--ref <track>` (repeatable) makes variations of a track you have, `--takes` takes for each
reference. Where the reference goes depends on the service:

| Service | What it gets | Result |
|---|---|---|
| Stable Audio | the audio (audio-to-audio) | a variation that keeps the reference's structure; `--keep` (0-1, default 0.4) sets how much |
| ACE-Step | the audio, as a style reference | a new track with the reference's sound |
| YuE2 | the audio, for a cover: SheetSage2 on the server transcribes its melody | an instrumental cover: the reference's melody on an instrument, in the prompt's style |
| Lyria, ElevenLabs | a description of the audio, written by the audio model ("ears" job) | a new track written from words: a similar style, not a variation |

cav says on stderr (and in `refs` with `--json`) which way each reference went. For a real
variation, pick Stable Audio or ACE-Step with `--service`.

Every service sent takes with true peaks above 0 dBFS in tests. cav lowers the level of
such a take to about -1.5 dBTP (gain only, no limiter) and says by how much; `--raw-level`
keeps the level the service sent. Then set the loudness in your mix or with
`cav render --audio`.

### Music services

What each service did in tests on 2026-10-09, one or two takes each. Treat these as
observations, not guarantees.

| Service | Length | Storyboard plan | Reference track | Watch out for |
|---|---|---|---|---|
| Stable Audio | exact (matched the storyboard) | sections go into the prompt; with `--ref`, the reference's arc wins over the plan | sent as audio: MP3 or WAV, 6-190 s (cav converts and cuts) | follows the reference's arc, not the storyboard's: a reference with a quiet intro gives a take with no early hit |
| Lyria | a prompt hint only (3 s too long) | one `[m:ss - m:ss]` line per shot; the drop landed on its cut | described in words | sends MP3 unless asked for WAV; cav asks, and the WAV came as 24-bit 48 kHz |
| ElevenLabs | not kept (7 s too long with music_v1) | composition plan; section names are the shot ids only | described in words | music needs a paid plan (HTTP 402 on free); an API key can have its own credit cap below the plan's (HTTP 401 "exceeds your API key quota"); moderation refuses a plan or prompt that names a brand, product or artist |
| ACE-Step | `audio_duration` | sections go into the prompt | sent as audio, as a style reference; `--keep` does nothing | a server you run |
| YuE2 | no length setting: cav, or its server, cuts the planned score at a phrase (see "YuE2" below) | not used: YuE2 knows only its own section names | a cover of its melody (see "YuE2 covers" below); in words when the server cannot transcribe | a server you run; the music is non-commercial for a company (see below) |

How `--keep` behaved on one 74 s reference (one take each): 0.2 drifted to its own tempo
and structure; 0.4 kept the reference's tempo and its groove, breakdown and re-drop
within a fraction of a second, with new parts on top; 0.6 stayed closest, down to the
same loudness range. Start at 0.4, and go lower for a looser take.

A description written by the audio model changes from run to run: the same reference was
described once as "F major electropop" and once as "A minor 1980s synthwave". Read the
description cav prints, and if it is off, write the style into the prompt yourself.
Keep brand and product names out of prompts and the storyboard's music `prompt` fields:
ElevenLabs refuses them, and they do not help any music model.

### Songs with lyrics

`--lyrics song.txt` makes a song with vocals. Only the servers you run sing: YuE2 and
ACE-Step. ElevenLabs, Stable Audio and Lyria get "no vocals" in every request, and cav
refuses `--lyrics` for them. Write the lyrics as plain text, with one section tag on its
own line before each section and a blank line between sections:

```text
[Verse]
Neon fades along the lane
Footsteps keep the time of rain

[Chorus]
Let the day come into view
Every road begins with you
```

Put the language, the voice and the tempo in the prompt, for example "English, warm
piano pop, expressive female voice, 88 BPM". The two services set the length in
different ways:

- ACE-Step keeps `--seconds` (or the storyboard's length). If the lyrics are too long for
  that length, the song can stop in the middle of a line. It takes at most 4096
  characters of lyrics; YuE2 takes 16000.
- YuE2 sings all the lyrics, and the lyrics set the length. cav does not send
  `--seconds`: YuE2's score has the vocal notes but not the words, so the server cannot
  cut it without stopping in the middle of a sung line. For a shorter song, write fewer
  or shorter sections, then trim or fade the take. cav prints how long it came out.
  In one take on 2026-10-11, a verse and a chorus of four lines each came out at 49.8 s
  (asked for 30 s), in 241 s on the M1 Max.

With `--ref` as well, YuE2 makes a sung cover: it sings your lyrics to the melody that
SheetSage2 transcribed from the track. The rules in "YuE2 covers" below still apply.

A take with lyrics has the same licence terms as an instrumental take from that service.

### YuE2

YuE2 is an open music model. It has no HTTP server of its own, so cav has one:
`tools/yue2-server/yue2_server.py` in the cav repository. It loads YuE2 once and keeps it in
memory. Install YuE2 from https://github.com/multimodal-art-projection/YuE and let it
download its weights (about 7 GB). Then start the server with that checkout's Python, and
point cav at it:

```bash
uv run --project ~/src/YuE --no-sync tools/yue2-server/yue2_server.py --device mps          # Apple silicon
uv run --project ~/src/YuE --no-sync tools/yue2-server/yue2_server.py --device cuda --budget 12  # 12 GB GPU
cav config endpoint yue2 http://127.0.0.1:8002
cav config check
cav music gen "warm synthwave, gated drums, builds to a drop, 112 BPM" --seconds 60 --service yue2
```

YuE2 makes songs with vocals. With `--lyrics`, the server runs YuE2 as it is and sings
them (see "Songs with lyrics"). Without lyrics, the server makes an instrumental the way the YuE2
repository's instrumental helper does: YuE2 plans a score, the helper moves the vocal
notes to an instrument, and YuE2 generates the audio from the changed score. In one take,
YuE2 planned no vocal notes at all for an instrumental style.

YuE2 has no length setting, and for an instrumental its planner wrote 92 to 240 s whatever cav asked for. The
audio follows the planned score, though: in 5 uncut takes it was within 3 s of the
score's length. So the server cuts the score before it generates: it keeps the start up
to a phrase (every four bars) or a section, then the outro. Asked for 60 s, the two cut
takes came out at 64.8 s (cut score 57.4 s) and 60.4 s (cut score 61.1 s), so trim or fade
a take to the storyboard.
cav prints the length, and says when YuE2 stopped a part at its token limit
(`truncated`).

Speed, measured on 2026-10-11 with `cav music gen ... --seconds 60` through the server:

| Machine | Server flags | Result |
|---|---|---|
| RTX 3080 Ti 12 GB, WSL2 Ubuntu | `--device cuda --budget 12` | 64.8 s of audio in 71 s |
| M1 Max 64 GB | `--device mps --budget 24` | 60.4 s of audio in 476 s (about 8 minutes) |

On the M1, most of the time goes to generating audio tokens (about 10 per second) and to
the 32 synthesis steps. An uncut take of 90 s took about 10 minutes there. Earlier runs
of YuE2's own command line, with the same flags, made 35.7 s of audio in 198 s on the M1
(peak memory 15.5 GB) and 59.2 s in 47.3 s on the 3080 Ti.

The weights are CC BY-NC 4.0, with a permission for individuals: personal users, creators
and musicians may use and sell what they make. A company that uses the weights
commercially needs a licence from the YuE2 authors. cav cannot tell the two apart, so it
records each take with YuE2's terms as non-commercial. A project set to
`cav credits licence nc-ok` accepts it. Otherwise `cav music gen` warns, and
`cav credits check` lists the take with those terms. If you work as an individual
creator, the take is yours to use; set the project to `nc-ok` or accept the warning.

#### YuE2-Turbo

cav also works with [YuE2-Turbo](https://github.com/NoizAI/YuE2-Turbo), NOIZ's serving
layer for YuE2 (Apache 2.0). cav's server speaks a subset of Turbo's job API, so the same
`yue2` service talks to either one.

Turbo is fastest on a large NVIDIA GPU on Linux, with vLLM. NOIZ tested it on an RTX 5090
(32 GB): with its default settings it uses about 18 GB at rest, writes a 60-second song in
about 10 seconds, and runs several jobs at once. Without vLLM, Turbo also runs on a 12 GB
GPU and on an Apple silicon Mac, with the low-memory settings below. There it is about as
fast as cav's server, so either one works.

Turbo has no instrumental mode and no length cut, and it refuses the extra fields that
cav's server takes. cav then does that work itself, with the same steps as its server:

1. A first job plans a song from your prompt. cav downloads its score
   (`GET /v1/jobs/{id}/score`) and does not use its audio.
2. cav moves the vocal notes to the instrument voice, and cuts the score to about
   `--seconds` (the same cut as its server).
3. A second job generates the audio from that score, sent as `abc`.

A cover (`--ref`, see "YuE2 covers" below) works the same way on Turbo. The first job is
Turbo's own `POST /v1/covers`: SheetSage2 transcribes the track, and cav takes the melody
from that job's score instead of a plan. Turbo turns covers off when
`YUE2_SHEETSAGE_DEVICE=off`, and then cav describes the track in words.

So each instrumental take costs two jobs on Turbo. At Turbo's speed, the extra job adds
about 10 to 40 seconds. A song with `--lyrics` needs neither the instrumental steps nor a
cut, so it is one ordinary job on Turbo, as on cav's server. The take sounds the same as on cav's server for the same planned score: cav's Go
code writes the same score as the server's Python code, byte for byte, in cav's tests.

Turbo needs an API key of 16 characters or more, on every job route. Start it with the key,
then give cav the same key:

```bash
# On the Turbo machine, in a YuE2-Turbo checkout installed as its README says:
export YUE2_API_KEY="$(openssl rand -hex 32)"
YUE2_HOST=0.0.0.0 yue2-serve      # port 8000; put HTTPS in front beyond a trusted network
# On the machine with cav:
export YUE2_API_KEY=<the same key>  # or cav config set-key yue2 <source>, such as a keychain item
cav config endpoint yue2 http://<turbo-host>:8000
cav config check                  # "YuE2-Turbo ready at ...; cav makes each instrumental in two jobs"
```

On a 12 GB GPU or a Mac, install Turbo without its vLLM extra, and start it with these
settings. They keep one job at a time and do not keep the models in memory between songs:

```bash
export YUE2_BACKEND=torch YUE2_RESIDENT_MODELS=false YUE2_MEMORY_BUDGET_GIB=12 \
  YUE2_AR_CONCURRENCY=1 YUE2_NAR_BATCH_SIZE=1 YUE2_AR_NAR_OVERLAP=false
# On a Mac, also: YUE2_DEVICE=mps YUE2_BACKEND=torch-eager, and a larger budget (24 on 64 GB).
```

Turbo's `server` extra installs vLLM. Without it, install the server's other packages by
hand. Covers also need `torchaudio`, and `setuptools` older than 81, because `pretty_midi`
imports `pkg_resources`. On macOS 27, Turbo's pinned scipy 1.15.3 does not load: upgrade
scipy. SheetSage2 downloads its parent model, `m-a-p/MERT-v2-FullSong` (2.4 GB), the
first time. With `YUE2_LOCAL_FILES_ONLY=true`, download it first.

Turbo does not say which model it loaded. cav records `YuE2-3B` with each take. If Turbo
runs another model (`YUE2_MODEL`), give its name as the endpoint's model, so cav records
that instead: `cav config endpoint yue2 http://<turbo-host>:8000 <model name>`.

`cav config check` tells the two servers apart. cav's server lists `"extensions":
["instrumental"]` in `/health/ready`, and Turbo lists none. For Turbo, the check also sends
the key to `/internal/load`, so a wrong key shows up there and not in the middle of a take.
cav never prints the key.

cav was tested against Turbo commit 7c88813 on 2026-10-12, with the low-memory settings
above:

| Machine | Take | Audio | Time |
|---|---|---|---|
| RTX 3080 Ti, 12 GB (WSL2) | instrumental, `--seconds 30` | 31.6 s | 127 s |
| RTX 3080 Ti | song, `--lyrics` | 70.9 s | 46 s |
| RTX 3080 Ti | cover, `--ref` (60 s track), `--seconds 30` | 32.1 s | 100 s |
| M1 Max, 64 GB | instrumental, `--seconds 30` | 32.4 s | 670 s |

A large GPU with vLLM was not tested. cav's tests also use a fake Turbo server, written
from Turbo's source (`src/yue2/service.py`), that refuses unknown fields as Turbo does.

### YuE2 covers

With `--ref`, YuE2 makes an instrumental cover of the track:

```bash
cav music gen "solo cello with warm piano accompaniment, intimate and gentle" --ref theme.mp3 --service yue2
```

cav uploads the track to the server's `POST /v1/covers`, the same route and fields as
YuE2-Turbo's (at most 40 MiB; cav converts a bigger file to MP3). SheetSage2, a
transcription model, turns the track's melody into a score, without chords. The server
moves the melody to an instrument, as for any instrumental take, and YuE2 plays it in the
prompt's style with a new accompaniment. It does not copy the singer, the sound or the
arrangement: only the tune. The cover is about as long as the track. `--seconds` cuts it
at a phrase, as for other takes.

SheetSage2 needs its own Python environment and about 2.7 GB of weights, next to YuE2's
(see `tools/yue2-server/README.md`). Without them, the server's covers are off. cav then
describes the track in words, as for Lyria, and says why. cav does the same when
SheetSage2 cannot transcribe a track.

Measured on 2026-10-11 on an M1 Max 64 GB (`--device mps`), with a 38.4 s reference
(Ode to Joy, rendered with `cav music render`): SheetSage2 transcribed it on MPS in 18 s,
and the cover came back with 37.8 s of audio in 177 s. When SheetSage2 transcribed the
cover again, it found the same 62-note melody as in the reference. One take, so treat
this as one observation. A transcription is never exact: SheetSage2 can miss quiet or
busy parts of a song.

A cover plays someone else's song. cav records the reference track with the take
(`generated.coverOf` in `.cav/manifest.json`), adds it to the take's licence note, and
`cav credits` lists a credit line for it: `Cover of “theme” (melody from theme.mp3)`. Put
the song's real title and writers in that line. To use the cover, you need the
songwriter's permission or a cover licence, whatever YuE2's terms allow. SheetSage2's
weights are CC BY-NC 4.0 as well, with no permission like YuE2's for selling what it
makes.

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
ear. The video track is silent, so only the takes are heard. `cav score render
score/project.rpp` renders it to a WAV for `cav render --audio`.

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
  or the end of your turn tells you once. Then run `cav review wait`. In the Antigravity CLI
  the plugin adds the same hook (`cav review hook --host antigravity`); it tells you before a
  model call or at the end of your turn.
- In Claude Code, the optional cav-review plugin also shows the open notes above the prompt,
  links to the page, and can start a turn when notes arrive.
- When `cav mcp` runs (the cavalry plugin starts it; see `cav help mcp`), the `show_review`
  tool shows the page inside the chat instead, with a small preview of the render, in hosts
  that draw MCP Apps. Other hosts show only the result: then give the user the browser page
  with `cav review`. The notes go to the same review file, so `cav review wait` reads them
  as usual. Without a shell, read them with the `review_notes` tool: it returns the sent
  notes with their snapshot images and marks the send received.
