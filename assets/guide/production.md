# Production: plan, source, music and review

These commands support planning, asset creation, music and review. Most work before
Cavalry opens because they do not need its bridge. `cav board place` needs the bridge
because it adds storyboard frames to the open scene.

Every job has a path that requires no API key. When keys are available, cav selects a
configured service and reports its name. `cav config` shows the service order for each
job. `cav config check` or `cav doctor --services` makes one free check per service.

## 1. Set up the services (once)

```bash
cav config                      # jobs, their order, and where each key lives
cav config check                # one free call per service; a missing key is a skip
cav config check --live         # also one small paid call per keyed service (asks first)
cav config set-key openai op://Private/OpenAI/api key    # or env:NAME, keychain:svc/acct
```

Local models run on the user's machine. On Apple silicon, mflux runs open image models
through uv, using FLUX.2 klein 4B by default. One 16:9 still took about 20 s. The first
run downloads about 16 GB. The default `image` order uses mflux after that model reaches
the Hugging Face cache. It comes after keyed services and before comfyui, sketch and
greybox. Ask the user before running `cav gen image "<prompt>" --service mflux` for the
first download. With `--ref`, mflux uses all supplied reference images for an edit.
rembg removes an image background with BiRefNet (`birefnet-general`) on platforms with
uv. Its first run downloads about 1 GB into `~/.rembg`. It is last in the `image.alpha`
order. OpenAI and Recraft produce transparency during generation and need no second pass.
That can preserve soft edges and shadows.
The user runs ComfyUI, ACE-Step and YuE2 as local servers. Configure ComfyUI with
`cav config endpoint comfyui http://127.0.0.1:8188 <workflow-api.json>`.
The workflow holds `{{prompt}}`, `"{{width}}"`, `"{{height}}"` and `"{{seed}}"`.
Configure ACE-Step 1.5 with `cav config endpoint acestep http://127.0.0.1:8001`.
Run its MIT-licensed server with `uv run acestep-api` in its repository; its music can be used commercially.
Configure YuE2 with `cav config endpoint yue2 http://127.0.0.1:8002`.
Its server is in `tools/yue2-server`; see "YuE2" below for setup and licence terms.

ACE-Step downloads models on its first request, which can take several minutes. It selects
a planner by available memory. On a Mac with 64 GB, it selects the 4B model, adding
8.4 GB to the download. To use a smaller download, start it with
`ACESTEP_LM_MODEL_PATH=acestep-5Hz-lm-1.7B uv run acestep-api --lm-model-path acestep-5Hz-lm-1.7B`
(about 9.5 GB of models in all). On an M1 Max a 24 s take then took about 20 s.

A tested ComfyUI workflow makes an image from text with Stable Diffusion 1.5. Put
`v1-5-pruned-emaonly-fp16.safetensors` (2.1 GB, from Hugging Face
`Comfy-Org/stable-diffusion-v1-5-archive`) in ComfyUI's `models/checkpoints`, save this as
`sd15.json` and run `cav config endpoint comfyui http://127.0.0.1:8188 sd15.json`. Change
`ckpt_name` for another checkpoint. On an M1 Max a 1024x576 image took about 1 minute.
Stable Diffusion 1.5 was trained at 512 px. At cav's larger sizes, it can repeat or
stretch a subject. An SDXL checkpoint is better suited to those sizes.

```json
{
  "4": {"class_type": "CheckpointLoaderSimple", "inputs": {"ckpt_name": "v1-5-pruned-emaonly-fp16.safetensors"}},
  "5": {"class_type": "EmptyLatentImage", "inputs": {"width": "{{width}}", "height": "{{height}}", "batch_size": 1}},
  "6": {"class_type": "CLIPTextEncode", "inputs": {"text": "{{prompt}}", "clip": ["4", 1]}},
  "7": {"class_type": "CLIPTextEncode", "inputs": {"text": "blurry, low quality, text, watermark", "clip": ["4", 1]}},
  "3": {"class_type": "KSampler", "inputs": {"seed": "{{seed}}", "steps": 20, "cfg": 7, "sampler_name": "euler", "scheduler": "normal", "denoise": 1, "model": ["4", 0], "positive": ["6", 0], "negative": ["7", 0], "latent_image": ["5", 0]}},
  "8": {"class_type": "VAEDecode", "inputs": {"samples": ["3", 0], "vae": ["4", 2]}},
  "9": {"class_type": "SaveImage", "inputs": {"filename_prefix": "cav", "images": ["8", 0]}}
}
```

The `ears` job can run Qwen3-Omni through OpenRouter, as tested on 2026-10-13. Configure
the endpoint and give `qwen-omni` the OpenRouter key source. One critique cost less than
0.1 cent in that test.

```bash
cav config endpoint qwen-omni https://openrouter.ai/api/v1 qwen/qwen3.8-omni-flash
cav config set-key qwen-omni <source of your OpenRouter key>
```

Veo video and Lyria music use the Gemini key. `cav config set-key gemini ...` therefore
configures all three. To use a separate Veo key, run `cav config set-key veo <source>`.

One fal.ai key serves several image models. cav uses FLUX.2 (`fal-ai/flux-2`) by default.
Other model ids include `fal-ai/flux-2-pro`, `fal-ai/ideogram/v3` (good with text),
`fal-ai/bytedance/seedream/v4.5/text-to-image` and `xai/grok-imagine-image`.
Choose one with `cav config model fal fal-ai/flux-2-pro`. With reference images, cav sends them to the model's edit
endpoint (Ideogram takes them as style references). References go inline as data URIs, so
keep them small (a few hundred KB each). Each model has a limit: Grok Imagine takes 3 images,
FLUX.2 takes 4, Seedream takes 10, and Ideogram takes 10 MB in total. cav sends the first ones that fit and says
which it left out.

A key source identifies where cav reads a key. cav does not print or store its value.
If 1Password is locked, services that depend on it fail and print a fix. Ask the user
to run `cav config set-key` themselves. Keys must stay out of chat.

| Job | Free | With a key |
|---|---|---|
| `image` (frames, stills) | sketch (an agent-written SVG per shot, rendered by Cavalry), greybox cards; mflux (Apple silicon), comfyui (a user-run server) | gemini, openai, openrouter, fal |
| `image.alpha` (transparent PNG) | rembg (through uv): cuts out an image that an `image` service made | openai, recraft |
| `image.vector` (SVG) | vtracer, potrace (if installed) | recraft; ideogram only with `--service ideogram` |
| `video` (moving animatic shots) | none (the animatic holds each frame) | veo |
| `ref` (reference images) | openverse, wikimedia | pexels, unsplash |
| `sfx` (sound search) | openverse, indexed folders | freesound |
| `music` | `cav music render` (a written score); acestep, yue2 (user-run servers); search with `cav sfx` | elevenlabs, stability, lyria |
| `ears` (critique of a track) | none | gemini, qwen-omni (a user-run server, or OpenRouter) |

## 2. Licences come first

Set the project's licence with `cav credits licence nc-ok` for non-commercial use or
`cav credits licence commercial`. Searches then hide incompatible assets. cav records
each downloaded or generated asset in `.cav/manifest.json` with its licence, author and
credit line. Before delivery, run:

```bash
cav credits check               # NC in commercial work, share-alike, missing credits
cav credits -o CREDITS.txt      # the credit lines the licences require
```

`cav check` reports the same licence problems.

## 3. Plan: mood board, storyboard, animatic

`cav guide flow` defines stage order and skip conditions. The commands below implement
those stages.

```bash
cav brief init --title "Launch day sting" --goal "Announce the launch" --audience "Existing users" \
  --tone confident --tone playful --seconds 15 --bpm 120 --width 1080 --height 1920 --fps 30
cav brief show                              # problems stop here; "Still to fill" lists the gaps
cav review brief.json &                     # the user approves the brief before you plan
cav ref search "neon city night"            # pick refs; each hit has a service:id ref
cav ref get openverse:<id>                  # into moodboard/, with licence and credit
cav ref arena https://www.are.na/<user>/<channel>   # the user's Are.na board, reference only
cav ref add <youtube or vimeo link>        # a web video: embedded, never downloaded, reference only
cav board mood                              # moodboard.json + renders/moodboard.png: look at it
cav board mood take <id> "the blue/orange split"   # one take-from-it line per item
cav review moodboard.json &                 # the user: keep, more like this, drop
cav board init --shots 6                    # size, fps, length, tempo and title come from brief.json
# edit storyboard.json: "what" and "why" per shot, "style" (prompt, refs, palette)
cav board frames                            # one frame per shot into board/
cav board sheet                             # renders/board.png: look at it
cav review storyboard.json &                # the user judges the stills: a verdict per shot, then Approve
cav review wait storyboard.json             # returns when the user sends notes or approves
cav board motion --only s2,s5               # optional: moving clips for chosen shots (Veo)
cav board animatic --audio music.wav        # renders/animatic.mp4, cut on the beat
cav review renders/animatic.mp4             # get the user's notes before building
cav board place                             # the frames as placeholder layers in Cavalry
```

- Wait for storyboard approval before starting slow or paid work, including `cav board motion`,
  `cav music gen` and the animatic. Check with
  `cav review export storyboard.json --json`: its `approved` must be `true`. Fix every
  note and every shot marked redo first (see "Review stills" in section 6). The agent cannot
  approve for the user and must never edit `approved` by hand.
- Give every shot a `why` that states what the viewer learns or feels. The agent writes it.
  `cav board init` adds a placeholder, and `cav check` warns when the field remains empty
  or unchanged. Remove or combine a shot if it has no purpose. `cav board frames` and
  `motion` do not require this field.
- Set `"cut": true` when the user requested a hard cut into a shot. `cav check` compares
  frames around shot starts and flags sudden changes. It lists marked cuts as intended.
- Set `"hold": true` when a shot needs no movement. `cav check` reports spans longer
  than about 2 seconds with no moving layer. It lists marked holds as intended. Add
  slight movement when it helps the shot.
- Time shots in beats, like the plan table in `cav guide`. Put cuts on bar starts
  (multiples of 4 in 4/4). With real music, set `"grid": "build/grid.json"` from
  `cav beats music.wav --json -o build/grid.json` so beats follow the track.
- Generated frames use the same `style.refs` across shots.
- `cav board motion` starts each clip from its shot frame. Make and approve those frames
  first. It uses Veo 3.1 Lite by default (`cav config model veo veo-3.1-fast-generate-preview`
  for Fast). Veo makes 4, 6 or 8 seconds at 24 fps. cav picks the shortest clip that
  covers the shot. The animatic cuts the clip to the shot length or holds its last frame
  for a longer shot. Veo adds sound, which the animatic removes before playing the chosen
  music. Each clip costs money. Select shots whose movement matters with `--only`, or
  pass `--all`.
- Greybox cards contain text and do not show composition. Without an image service,
  draw a sketch for each shot (see "Sketch frames" below). For a greybox made of
  real shapes in Cavalry, build it, render it with `cav frame <n> -o board/s3.png`, and set
  that shot's `"frame"`. `cav board frames` keeps frames that exist (`--force` remakes them).
- `cav board place` adds each frame to the active composition as an
  image layer named by its shot id (`s1`, `s2`, ...). It shows the image from the shot's
  start beat to its end beat at the composition's frame rate, scaled to fit.
  Time 0 of the music is the composition's first frame. Beat 0 sits at the storyboard's
  `offset` (or the grid's first beat), as in the music. Find a placeholder with
  `cav.find('s3')`. After building a shot, delete its placeholder and give the shot's
  group the same name. Run `cav board place` again after changing beats or frames.
  It updates the image layers in place and keeps their parent, transforms and masks.
  Delete a placeholder to make cav fit it again. It keeps shots whose names built
  layers already use.
  `--only s2,s3` places some shots; `--comp NAME` targets another composition.

### Sketch frames

When no image generator is available, `cav board frames` can render a shot from an SVG written
by the agent. The SVG shows the shot's composition. The image order is gemini, openai,
openrouter, fal, mflux, comfyui, sketch, greybox. cav selects a sketch after all
generating services are unavailable during service selection. If a selected generator's
request fails, cav returns that error without switching to a sketch.

- Use sketches when no image key is available or when one shot needs exact control.
  Write one SVG per
  shot, `board/<id>.svg` next to the storyboard (`board/s1.svg`), with the storyboard's
  size as its canvas. The root is `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0
  1920 1080">`; use the width and height from `storyboard.json`. A different shape of
  canvas is refused.
- Draw the shot's main visual arrangement at its key moment. Use
  only `rect`, `circle`, `ellipse`, `line`, `polyline`, `polygon`, `path` and `g`, and
  fewer than about 100 shapes. Use flat fills from `style.palette`: the first colour is the
  background, the second is the foreground, and the accent goes only on the subject. Leave
  a lot of empty space. Show text as grey bars unless the shot is about the words. Show
  movement as a faint copy of the shape at its start position, and a thin solid line along
  its path. Cavalry's SVG import draws `stroke-dasharray` as a solid line, so do not rely on
  dashes.
- cav refuses scripts, event attributes (`onclick`), `image`, `foreignObject` and `style`
  elements. It also refuses links to other files, `url(` that does not point inside the file,
  and a `DOCTYPE`. Files must be at most 512 KiB, have at most 2000 elements, and nest no
  deeper than 32 levels. Use numbers for shape sizes and positions; percentages are refused.
  cav lists every problem with its line number and makes no frame until they are fixed.
- Run `cav board frames`. cav checks all the SVGs, then renders each one through Cavalry
  and writes `board/<id>.png`; the shot's `source` is `sketch`. Shots with no SVG get a
  greybox card, and cav lists them so the SVGs can be added before rerunning the command.
  `cav board frames --service sketch [--only s3]` renders only sketches and refuses when
  an SVG is missing.
- To revise a sketch, edit its SVG and run `cav board frames --only s3`. cav draws it
  again when its SVG is newer than its PNG; other frames are kept. The SVG stays in
  `board/` for editing. To redraw a sketch, save its SVG again or delete its PNG.
- Cavalry must be open with the bridge. cav draws in a scratch scene, then reopens the
  original scene. It refuses when the open scene has unsaved changes. Ask the user to save
  (`cav scene save`) first. An empty untitled scene is fine. Cavalry shows the scratch
  scene for a few seconds. If Cavalry is not open, the default order falls back to cards
  and says so.
- During review, redrawing a shot changes its frame. The user's verdict on that shot
  shows as outdated and the approval is removed. After the user approves, render only the
  shot that changed (`--only s3`); do not use `--force` on all shots.

Example, `board/s1.svg` for a logo that lands in the centre of a 1920x1080 board:

```xml
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1920 1080">
  <rect x="780" y="360" width="360" height="360" rx="48" fill="#f5f7fa"/>
  <circle cx="960" cy="540" r="90" fill="#ff4d2e"/>
  <rect x="660" y="800" width="600" height="28" rx="14" fill="#f5f7fa" opacity="0.4"/>
  <rect x="780" y="120" width="360" height="360" rx="48" fill="#f5f7fa" opacity="0.12"/>
  <line x1="960" y1="300" x2="960" y2="360" stroke="#f5f7fa" stroke-width="4" opacity="0.4"/>
</svg>
```

## 4. Assets

```bash
cav gen image "paper-cut mountains at dusk" -o assets/mountains.png
cav gen image "flat logo mark, a fox head" --alpha -o assets/fox.png     # transparent
cav gen image "line icon of a rocket" --vector -o assets/rocket.svg       # SVG
cav gen vector assets/fox.png -o assets/fox.svg                          # trace a PNG
cav gen vector assets/badge.png --service ideogram -o assets/badge.svg   # keeps a gradient; about $0.10
cav gen cutout photo.png -o assets/fox.png                               # remove a background
```

With rembg, `--alpha` first asks an available `image` service to make the image. rembg
then removes its background locally. Ask for a plain, even background to improve the
cutout. If only greybox is available, cav reports that there is no image to cut out.
`cav gen cutout` works on existing images through rembg or Recraft. OpenAI cannot cut out
an image it did not generate. A cutout or traced SVG keeps its source image's licence
and credit. If the image is absent from the manifest, its licence is unknown and
`cav credits check` reports it.

Some services return another format. Gemini returns JPEG, and Recraft returns WebP for
a cutout. cav converts output named `.png` to actual PNG. WebP cutouts keep transparency.

Load an image in Cavalry with `cav.image('/abs/path.png', 'name')`. Convert an SVG to
editable shapes with `api.convertSVGToLayers('/abs/path.svg')`. The call returns new
layer IDs, which scripts can animate.

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

Lyria (`lyria-3.5`) has no separate length or section fields. cav puts total length,
"instrumental only" and one `[m:ss - m:ss]` line per shot into the prompt. Lyria can
miss those timings. Check cuts with `cav listen --board`. cav requests WAV.
`cav config model lyria lyria-3-clip-preview` makes a 30-second MP3 clip instead.

Music is instrumental unless the command includes `--lyrics`. See "Songs with lyrics".

Repeat `--ref <track>` to use more than one source track. `--takes` sets the number of
takes per reference. Services use a reference in different ways:

| Service | What it gets | Result |
|---|---|---|
| Stable Audio | the audio (audio-to-audio) | a variation that keeps the reference's structure; `--keep` (0-1, default 0.4) sets how much |
| sa3 (local Stable Audio 3) | the audio as init audio, noised to σmax = 1 − `--keep` | a variation of the reference, trimmed or padded to `--seconds` (default: the reference's own length); Apple silicon, no key |
| ACE-Step | the audio, as a style reference | a new track with the reference's sound |
| YuE2 | the audio, for a cover: SheetSage2 on the server transcribes its melody | an instrumental cover: the reference's melody on an instrument, in the prompt's style |
| Lyria, ElevenLabs | a description of the audio, written by the audio model ("ears" job) | a new track written from the description in a similar style; variation of the source audio is unavailable |

cav reports reference handling on stderr and in `refs` with `--json`. Select Stable Audio
or ACE-Step with `--service` when the result must use the source audio.

In tests, every service returned takes with true peaks above 0 dBFS. cav reduces such
takes to about -1.5 dBTP using gain alone and reports the reduction. `--raw-level`
keeps the original level. Set final loudness in the mix or with `cav render --audio`.

### Music services

The table reports one or two takes per service tested on 2026-10-09. These are
observations from that sample.

| Service | Length | Storyboard plan | Reference track | Watch out for |
|---|---|---|---|---|
| sa3 (local Stable Audio 3) | exact: the take is trimmed to `--seconds`; the small models run to 120 s, `medium` to 380 s | sections go into the prompt after the model's TrackType prefix | sent as audio as init audio; `--keep` 0.4 becomes σmax 0.6 | Apple silicon only; the first run downloads 1.6 GB (`medium`: 4.9 GB), and the first --ref run adds the codec encoder (205 MB; `medium`: 1.6 GB); instrumentals only; takes are under the Stability AI Community License (free commercial use below $1M revenue); a 10 s take took about 2 s on an M-series Mac once loaded |
| Stable Audio | exact (matched the storyboard) | sections go into the prompt; with `--ref`, the reference's arc wins over the plan | sent as audio: MP3 or WAV, 6-190 s (cav converts and cuts) | the reference's arc takes precedence over the storyboard's; a reference with a quiet intro gives a take with no early hit |
| Lyria | a prompt hint only (3 s too long) | one `[m:ss - m:ss]` line per shot; the drop landed on its cut | described in words | sends MP3 unless asked for WAV; cav asks, and the WAV came as 24-bit 48 kHz |
| ElevenLabs | not kept (7 s too long with music_v1) | composition plan; section names are the shot ids only | described in words | music needs a paid plan (HTTP 402 on free); an API key can have its own credit cap below the plan's (HTTP 401 "exceeds your API key quota"); moderation refuses a plan or prompt that names a brand, product or artist |
| ACE-Step | exact: ACE-Step ends its takes 2-6 s early, so for an instrumental cav asks for 8 s more, then cuts the take to length and fades out the last 2 s (tested 2026-10-13) | the prompt gets a timeline, and an instrumental also gets one `[shot - what]` tag per shot in its lyrics; with 8 s shots at 120 BPM, 7 of 8 takes changed near the first cut; a cut at 4 s was ignored because it keeps its own 4-bar phrases | sent as audio, as a style reference; `--keep` does nothing | a user-run server; a song with `--lyrics` is not cut, so listen to its end |
| YuE2 | no length setting: cav, or its server, cuts the planned score at a phrase (see "YuE2" below) | not used: YuE2 knows only its own section names | a cover of its melody (see "YuE2 covers" below); in words when the server cannot transcribe | a user-run server; the music is non-commercial for a company (see below) |

On one 74 s reference, each `--keep` value produced one take. At 0.2, the take changed
tempo and structure. At 0.4, it kept the tempo and major sections within a fraction of
a second while adding new parts. At 0.6, it also kept a similar loudness range. Start
at 0.4 and lower it if the result needs more change.

Audio-model descriptions can vary. One reference was described as "F major electropop"
in one run and "A minor 1980s synthwave" in another. Read cav's description. If it
does not fit, state the desired style in the prompt.
Keep brand and product names out of prompts and the storyboard's music `prompt` fields:
ElevenLabs refuses them, and they do not help any music model.

### Songs with lyrics

`--lyrics song.txt` requests vocals from YuE2 or ACE-Step, which run on user-operated
servers. cav sends "no vocals" to ElevenLabs, Stable Audio and Lyria and refuses
`--lyrics` with those services. Use plain text lyrics. Put each section tag on its own
line and separate sections with a blank line:

```text
[Verse]
Neon fades along the lane
Footsteps keep the time of rain

[Chorus]
Let the day come into view
Every road begins with you
```

State the language, voice and tempo in the prompt. For example, use "English, warm
piano pop, expressive female voice, 88 BPM". The services handle length differently:

- ACE-Step keeps `--seconds` (or the storyboard's length). If the lyrics are too long for
  that length, the song can stop in the middle of a line. It takes at most 4096
  characters of lyrics; YuE2 takes 16000.
- YuE2 sings all the lyrics, and the lyrics set the length. cav does not send
  `--seconds`. YuE2's score contains vocal notes and lacks words. The server cannot cut it
  without stopping in the middle of a sung line. For a shorter song, write fewer or
  shorter sections, then trim or fade the take. cav prints how long it came out.
  In one take on 2026-10-11, a verse and a chorus of four lines each came out at 49.8 s
  (asked for 30 s), in 241 s on the M1 Max.

With `--ref` as well, YuE2 makes a sung cover: it sings the supplied lyrics to the melody
that SheetSage2 transcribed from the track. The rules in "YuE2 covers" below still apply.

A take with lyrics has the same licence terms as an instrumental take from that service.

### YuE2

YuE2 is an open music model. The cav repository supplies an HTTP server for it:
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

YuE2 can generate songs with vocals. With `--lyrics`, the server sings the supplied words.
Without lyrics, it follows the YuE2 repository's instrumental method. YuE2 plans a score,
the helper moves vocal notes to an instrument, and YuE2 generates audio from that score.
In one instrumental take, the plan contained no vocal notes.

YuE2 has no length setting. Its instrumental planner wrote 92 to 240 s regardless of the
requested duration. In five uncut takes, audio duration stayed within 3 s of the score.
The server therefore cuts the score before generation at a four-bar phrase or section,
then keeps the outro. Two takes requested at 60 s produced 64.8 s and 60.4 s of audio
from scores cut to 57.4 s and 61.1 s. Trim or fade takes to match the storyboard.
cav prints the length, and says when YuE2 stopped a part at its token limit
(`truncated`).

Speed, measured on 2026-10-11 with `cav music gen ... --seconds 60` through the server:

| Machine | Server flags | Result |
|---|---|---|
| RTX 3080 Ti 12 GB, WSL2 Ubuntu | `--device cuda --budget 12` | 64.8 s of audio in 71 s |
| M1 Max 64 GB | `--device mps --budget 24` | 60.4 s of audio in 476 s (about 8 minutes) |

On the M1, audio-token generation took about 10 tokens per second, followed by 32
synthesis steps. An uncut 90 s take took about 10 minutes. Earlier runs
of YuE2's own command line used the same flags. They made 35.7 s of audio in 198 s on the M1
(peak memory 15.5 GB) and 59.2 s in 47.3 s on the 3080 Ti.

The weights use CC BY-NC 4.0 with extra permission for individuals. Personal users,
creators and musicians can use and sell their outputs. Companies need a licence from
the YuE2 authors for commercial use of the weights. cav cannot determine the user's
status, so it records each take as non-commercial. A project set to
`cav credits licence nc-ok` accepts it. Otherwise `cav music gen` warns, and
`cav credits check` lists the take under those terms. An individual creator can set
the project to `nc-ok` or accept the warning, subject to YuE2's terms.

#### YuE2-Turbo

cav also supports [YuE2-Turbo](https://github.com/NoizAI/YuE2-Turbo), NOIZ's Apache 2.0
server for YuE2. cav's server implements part of Turbo's job API, so `yue2` can use
either server.

Turbo runs fastest on a large NVIDIA GPU under Linux with vLLM. NOIZ tested it on an RTX 5090
(32 GB). With its default settings, it uses about 18 GB at rest and writes a 60-second song
in about 10 seconds. It runs several jobs at once. Without vLLM, Turbo also runs on a 12 GB
GPU and on an Apple silicon Mac, with the low-memory settings below. There it is about as
fast as cav's server, so either one works.

Turbo has no instrumental mode or score-length cut. It also rejects the extra fields
accepted by cav's server. cav performs those steps itself:

1. A first job plans a song from the prompt. cav downloads its score
   (`GET /v1/jobs/{id}/score`) and does not use its audio.
2. cav moves the vocal notes to the instrument voice, and cuts the score to about
   `--seconds` (the same cut as its server).
3. A second job generates the audio from that score, sent as `abc`.

A cover (`--ref`, see "YuE2 covers" below) works the same way on Turbo. The first job is
Turbo's own `POST /v1/covers`: SheetSage2 transcribes the track, and cav takes the melody
from that job's score. It does not use a plan. Turbo turns covers off when
`YUE2_SHEETSAGE_DEVICE=off`, and then cav describes the track in words.

Each instrumental take uses two Turbo jobs. At Turbo's speed, the extra job adds
about 10 to 40 seconds. A song with `--lyrics` needs neither the instrumental steps nor a
cut, so it is one ordinary job on Turbo, as on cav's server. For the same planned score,
the take sounds the same as on cav's server. cav's tests show that its Go code writes the
same score as the server's Python code, byte for byte.

Turbo requires an API key of at least 16 characters on every job route. Configure the
same key in Turbo and cav:

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

Turbo's `server` extra installs vLLM. Without that extra, install the remaining server
packages manually. Covers need `torchaudio` and `setuptools` older than 81 because
`pretty_midi` imports `pkg_resources`. On macOS 27, upgrade Turbo's pinned scipy 1.15.3
because it does not load. SheetSage2 first downloads its 2.4 GB parent model,
`m-a-p/MERT-v2-FullSong`. Download it beforehand when `YUE2_LOCAL_FILES_ONLY=true`.

Turbo does not report its loaded model. cav records `YuE2-3B` with each take. If Turbo
runs another model (`YUE2_MODEL`), give its name as the endpoint's model, so cav records
that instead: `cav config endpoint yue2 http://<turbo-host>:8000 <model name>`.

`cav config check` distinguishes the servers. cav's server lists `"extensions": ["instrumental"]`
in `/health/ready`; Turbo lists none. For Turbo, the check sends the
key to `/internal/load` so it can report a wrong key before music generation.
cav never prints the key.

cav was tested against Turbo commit 7c88813 on 2026-10-12, with the low-memory settings
above:

| Machine | Take | Audio | Time |
|---|---|---|---|
| RTX 3080 Ti, 12 GB (WSL2) | instrumental, `--seconds 30` | 31.6 s | 127 s |
| RTX 3080 Ti | song, `--lyrics` | 70.9 s | 46 s |
| RTX 3080 Ti | cover, `--ref` (60 s track), `--seconds 30` | 32.1 s | 100 s |
| M1 Max, 64 GB | instrumental, `--seconds 30` | 32.4 s | 670 s |

The cav tests did not run Turbo on a large GPU with vLLM. They also use a fake Turbo
server based on `src/yue2/service.py`. It rejects unknown fields like Turbo.

### YuE2 covers

With `--ref`, YuE2 makes an instrumental cover from a source track:

```bash
cav music gen "solo cello with warm piano accompaniment, intimate and gentle" --ref theme.mp3 --service yue2
```

cav uploads the track through `POST /v1/covers` using YuE2-Turbo's route and fields.
The limit is 40 MiB; cav converts larger files to MP3. SheetSage2 transcribes the melody
without chords. The server assigns it to an instrument, and YuE2 adds accompaniment in
the requested style. The generated cover follows the tune and has roughly the source
track's length. `--seconds` cuts it at a phrase, as with other takes.

SheetSage2 needs a separate Python environment and about 2.7 GB of weights alongside YuE2's
(see `tools/yue2-server/README.md`). Without them, the server's covers are off. cav then
describes the track in words, as for Lyria, and says why. cav does the same when
SheetSage2 cannot transcribe a track.

One measurement on 2026-10-11 used an M1 Max with 64 GB (`--device mps`) and a 38.4 s
reference, Ode to Joy, made with `cav music render`. SheetSage2 transcribed it in 18 s.
The 37.8 s cover took 177 s to generate. A second transcription found the same 62-note
melody. This is one take. SheetSage2 can miss quiet or busy passages in other tracks.

A cover uses another songwriter's melody. cav records the reference track with the take
(`generated.coverOf` in `.cav/manifest.json`) and adds it to the take's licence note.
`cav credits` lists a credit line: `Cover of “theme” (melody from theme.mp3)`. Put
the song's real title and writers in that line. Using the cover requires the
songwriter's permission or a cover licence, subject to YuE2's terms. SheetSage2's
weights are CC BY-NC 4.0 as well, with no permission like YuE2's for selling what it
makes.

### Write the music

`cav music render score.json [--stems]` plays a written score locally without a key.
The score places notes and hits on a beat grid:

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
- A plugin track plays its default sound unless a `preset` is supplied: a `.vstpreset`
  (VST3), or a file with the bytes of pedalboard's `plugin.raw_state` (VST3 or AU). `params`
  sets parameters by their pedalboard names; an unknown name lists the real ones. macOS loads
  an AU only from `/Library/Audio/Plug-Ins/Components` or `~/Library/Audio/Plug-Ins/Components`.
- `master.lufs` sets the loudness; the limiter keeps the true peak under -1 dBTP.
  `master.reference` matches the tone to a reference track (Matchering); the loudness and
  peak targets still apply after it. A very sparse mix (a lone kick) cannot reach -14 LUFS
  under that peak: the result has a `warning`. Fill the mix out, or set `master.lufs` lower.
- Write the score from the storyboard: shots start on beats, so cuts land on the music.

### Judge the music

`cav listen` reports these checks for agents without audio playback:

- **Numbers:** length against the piece, tempo, sections, and loudness (aim near
  -14 LUFS and a true peak of at most -1 dBFS for web).
- **Cuts:** each storyboard cut against the nearest downbeat, in frames.
- **A picture:** the spectrogram view with the cuts drawn green on a downbeat, red off it.
  Look at it.
- **Scores from local models**, with `--score`: Meta's Audiobox Aesthetics (production
  quality, enjoyment, complexity, usefulness, 0-10) and, with `--brief`, how well the audio
  matches the brief (LAION CLAP). The first run downloads about 2 GB.
- **A critique from an audio model**, when the `ears` job has a service. It is a model's
  opinion; label it as such when passing it on.

Compare takes with `cav listen a.mp3 b.mp3 --score --board storyboard.json`. Let the user
choose, then sync with `cav beats` and `cav guide music`. Render with
`cav render --audio`.

### Finish the music in REAPER

`cav score storyboard.json --video renders/final.mp4 --audio music/take1.mp3` creates
`score/project.rpp` with tempo, one marker per shot, a video track and separate music
takes. The user opens it in REAPER to choose a take, trim, fade and adjust timing by ear.
The video track is silent. `cav score render score/project.rpp` makes a WAV for
`cav render --audio`.

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
- The page steps through exact frames. J/K/L shuttles playback, and I/O sets ranges.
  Reviewers can draw arrows, boxes, ellipses and freehand marks. The audio waveform
  appears below the timeline.
- Re-rendering to the same path loads the new video and keeps the earlier version.
  W cycles through A/B wipe, difference image and onion-skin comparison modes.
- Each note has a frame or range, text, drawings and a PNG with the drawings added to
  the reviewed frame. Inspect the PNG to see the reviewer's exact view.
- `wait` returns when the reviewer sends notes or approves a stage, including approval without notes.
  It exits with code 3 if neither action occurs before the timeout. Run it again.
- `cav review` needs to listen on 127.0.0.1. A sandbox that blocks local ports must run
  it outside.
- The cavalry plugin for Claude Code and Codex adds a hook (`cav review hook`). When the
  reviewer presses Send to agent and no `cav review wait` picks up the notes, the hook
  notifies the agent at the next prompt or turn end. Then run `cav review wait`. In the
  Antigravity CLI, the plugin adds the same hook (`cav review hook --host antigravity`);
  it notifies the agent before a model call or at the end of the turn.
- In Claude Code, the optional cav-review plugin displays open notes above the prompt,
  links to the review page and can start a turn when notes arrive.
- When `cav mcp` runs (the cavalry plugin starts it; see `cav help mcp`), the `show_review`
  tool shows the page inside the chat in hosts that draw MCP Apps. It includes a small
  preview of the render. In other hosts, give the user the browser page
  with `cav review`. The notes go to the same review file, so `cav review wait` reads them
  as usual. Without a shell, read them with the `review_notes` tool: it returns the sent
  notes with their snapshot images and marks the send received.

### Review stills

`cav review` also supports planning stills. The user can approve each stage before
slow or paid work starts.

| Target | Items | Verdicts | Review file |
|---|---|---|---|
| `brief.json` | its fields | approve, redo | `brief.review.json` |
| `moodboard.json` | references: images, clips, video embeds | keep, more (more like this), drop | `moodboard.review.json` |
| `storyboard.json` | shots | approve, redo | `storyboard.review.json` |

- The page shows items in a grid. The user sets a verdict for each item and can add
  drawings and notes. Each note stores the item, verdict, text, drawings and a PNG.
- The Approve button works only when no item is marked redo or drop and no note is open.
  An open "more" does not block. Approving writes `approved` into the review file.
- Approval counts as a send. `cav review wait storyboard.json` returns even when there
  are no notes and says "is approved by <name>". `cav review export storyboard.json --json`
  reports `approved`, item verdicts and `blockers`.
- The page shows a shot's `why` under its text. Adding or changing `why` changes the shot, so
  its verdict becomes outdated once and the user judges it again.
- To fix a note, change the item in its file (for a shot: its `what`, its sketch SVG, or its frame with
  `cav board frames --force`), then run `cav review resolve <id> --note "what changed"`.
  Do not mark anything approved: there is no command for that, and the agent must never edit
  `approved` by hand. The page holds a secret that the server needs for an approval, so the
  chat tools cannot approve. This provides a strong approval signal. It cannot enforce a
  security boundary because an agent with a shell could still read the page and send the
  approval itself. Never do that.
- If a target file cannot load, `wait` and `export` report "approval unknown". Fix the
  file before relying on its approval state.
- A storyboard also has a "board" item: its settings (tempo, size, offset, beat grid and
  style). Changing any of them removes the approval ("board changed").
- Each item has a hash. If an item changes, its old verdict becomes outdated and cav
  removes approval. `approvalCleared` records the reason. Ask the user to review and
  approve again. Other shots keep their verdicts.
- On a mood board, a clip and a YouTube or Vimeo video play on the item (the user presses the
  play button; nothing loads before). The page shows the agent's take-from-it line under each item.
  In the chat view (`show_review`), a host may block the player: the link under the item
  always works, and a clip over 24 MB says to run `cav review moodboard.json`.
- A mood board or brief review needs `moodboard.json` or `brief.json`; cav reads their items
  from the same file name. `cav brief show` checks a brief first: a brief with a problem does
  not open in review. The hook and `cav mcp` (`show_review`, `review_notes`) work with
  all three targets.
