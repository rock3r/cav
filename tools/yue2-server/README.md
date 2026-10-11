# YuE2 server for cav

`yue2_server.py` is a small HTTP server for the `yue2` music service in `cav music gen`. It
loads YuE2 once, keeps it in memory, and runs one job at a time.

YuE2 ([code](https://github.com/multimodal-art-projection/YuE),
[demo](https://map-yue2.github.io/)) has only a Python API, a command line and ComfyUI
nodes. It has no HTTP server of its own, so cav needs this one.

## Run it

1. Install YuE2 from its repository, as its README says, and let it download the
   weights (about 7 GB) once, for example with `yue2 generate`.
2. Start the server from this repository with the Python of that YuE2 checkout:

   ```bash
   uv run --project ~/src/YuE --no-sync tools/yue2-server/yue2_server.py --device mps
   ```

   Or run the checkout's Python directly:
   `~/src/YuE/.venv/bin/python tools/yue2-server/yue2_server.py --device cuda --budget 12`.
3. Point cav at it and check it:

   ```bash
   cav config endpoint yue2 http://127.0.0.1:8002
   cav config check
   cav music gen "warm synthwave, confident, builds to a drop" --seconds 60 --service yue2
   ```

| Flag | Default | What it does |
|---|---|---|
| `--device` | `auto` | `cuda`, `mps` (Apple silicon) or `cpu`. `auto` picks them in that order. |
| `--budget` | `24` | Memory budget in GiB. `12` decodes the audio in smaller pieces, for a 12 GB GPU. |
| `--host`, `--port` | `127.0.0.1`, `8002` | Where to listen. Use `--host 0.0.0.0` to serve other machines, and then set `YUE2_API_KEY`. |
| `--yue2` | found | The YuE2 checkout. The server looks at `$YUE2_KIT`, the installed package, and the folder that holds the virtual environment. |
| `--model`, `--vae` | `m-a-p/YuE2-3B`, `m-a-p/YuE2-Vae` | Model paths or Hugging Face ids. A checkout's `models/` folder wins when it exists. |
| `--offline` | off | Use only models that are already downloaded. |
| `--max-pending` | `16` | Jobs queued or running before the server answers 429. |
| `--artifacts` | a temporary folder | Where job files go. The temporary folder is removed when the server stops. |
| `--transcriber-python` | `<checkout>/.venv-transcribe/bin/python` | The Python of the SheetSage2 environment, for covers. Covers are off when it does not exist. |
| `--sheetsage`, `--mert` | `<checkout>/models/SheetSage2`, `<checkout>/models/MERT-v2-FullSong` | SheetSage2 and the MERT model it builds on. Without the folders, SheetSage2 comes from Hugging Face at the helper's pinned revision. |
| `--sheetsage-device` | `auto` | Where SheetSage2 runs: `cuda`, `mps`, `cpu`, or `off` for no covers. `auto` picks `mps` on a Mac, and `cuda` only when `--sheetsage-min-free-gib` (default 8) is free with YuE2 loaded, else `cpu`, as YuE2-Turbo does. |

When the environment variable `YUE2_API_KEY` is set, every route except the health routes
needs it as a Bearer key. cav sends the key from `cav config set-key yue2 <source>`, or from
`YUE2_API_KEY`.

## API

The routes and fields are a subset of the API of
[YuE2-Turbo](https://github.com/NoizAI/YuE2-Turbo), NOIZ's serving layer for YuE2
(`src/yue2/service.py`, read on 2026-10-11). One client can then work with either server.
Turbo needs Linux, CUDA and vLLM, so it does not run on an Apple silicon Mac or, probably,
on a 12 GB GPU. This server runs on both.

cav works with Turbo too. Turbo refuses this server's extensions, so with Turbo cav plans
the instrumental and cuts the score itself, and sends the result as `abc` (see "YuE2-Turbo"
in `cav guide production`). cav tells the servers apart by `"extensions"` in
`/health/ready`. It still uses the extensions here: they save a pass of audio generation,
which is slow on a Mac.

| Route | What it does |
|---|---|
| `GET /health/live` | `{"status": "alive"}` |
| `GET /health/ready` | 503 while the model loads (or failed to load), then 200 with `"extensions": ["instrumental"]` |
| `POST /v1/jobs` | Starts a job and answers 202 with it. Takes an `Idempotency-Key` header. 429 with `Retry-After` when the queue is full. |
| `GET /v1/jobs/{id}` | The job. `status` is `queued`, `running`, `succeeded`, `truncated`, `failed` or `cancelled`; `stage` says what it is doing. |
| `POST /v1/jobs/{id}/cancel` | Cancels the job. |
| `GET /v1/jobs/{id}/audio` | The audio, FLAC, 48 kHz stereo. |
| `GET /v1/jobs/{id}/score` | The ABC score that YuE2 generated the audio from. |
| `POST /v1/covers` | Starts a cover of an uploaded recording, as in Turbo. 503 without `Retry-After` when covers are off. |

Job fields, as in Turbo: `style`, `lyrics` (with `[Verse]` and `[Chorus]` markers), `cot`
(`full`, `melody` or `off`), `seed`, `n` (only 1 here), `abc`, `cfg_scale`. A finished
job's `result` has `audio_seconds`, `sample_rate`, `truncated` (`abc` and `semantic`),
`timing`, `configuration` and, as an extension, `model` (the model the server loaded).

### The instrumental extension

YuE2 makes songs with vocals. `"instrumental": true` is an extension of this server; Turbo
does not have it and refuses unknown fields. With it, the server does what the
instrumental helper in the YuE2 repository (`skills/yue2-music/instrumental`, MIT licence)
does:

1. YuE2 plans a score from `style` and `lyrics`. The lyrics are only section tags, such as
   `[Intro]\n\n[Verse]\n\n[Chorus]\n\n[Outro]`. Nobody sings them.
2. The helper moves every note of the score's Vocal voice to its Ins voice.
3. YuE2 generates the audio from the changed score, with "Instrumental" and "no vocals, no
   singing, no choir, no spoken words" added to the style.

With `abc`, the server converts that score instead of planning one. The result gets an
`instrumental` object: where the score came from, its nominal length and tempo, and how
many notes moved. The server imports the helper from the YuE2 checkout; it does not copy
it.

### Covers

`POST /v1/covers` takes a `multipart/form-data` upload of at most 40 MiB, with the same
fields as YuE2-Turbo: the recording in `audio`, and `style`, `lyrics`, `seed` and
`cfg_scale`. `instrumental` and `seconds` work as in a job. The server does what the
instrumental helper's cover command does (`references/cover.md` in the helper):

1. SheetSage2 transcribes the recording's melody to an ABC score, without chords. It runs
   in its own Python, through the helper's `scripts/transcribe_cover.py`, so its pinned
   packages stay out of YuE2's environment. The job's `stage` is `transcribing`.
2. Without `instrumental`, YuE2 sings the lyrics on that melody with `cot` `melody`, as
   Turbo does. With `"instrumental": "true"`, the server first moves the melody to the Ins
   voice, and YuE2 arranges a new accompaniment.

It does not copy the original singer. The result has a `cover` object: the device,
the transcription time, the score's length and tempo, and SheetSage2's warnings. A failed
transcription ends the job with the error code `transcription_failed`.
`/health/ready` lists `covers` in `extensions` when covers are on.

To set up SheetSage2 in the YuE2 checkout, with the revisions that the helper pins in
`assets/cover-release.json`:

```bash
cd ~/src/YuE
uv venv --python 3.11 .venv-transcribe
curl -L https://huggingface.co/m-a-p/SheetSage2/resolve/eab522a8168e8b8b8c4856bf8609cd86198f01fe/requirements.txt -o /tmp/sheetsage2.txt
VIRTUAL_ENV=.venv-transcribe uv pip install -r /tmp/sheetsage2.txt
.venv/bin/python -c "from huggingface_hub import snapshot_download as d
d('m-a-p/SheetSage2', revision='eab522a8168e8b8b8c4856bf8609cd86198f01fe', local_dir='models/SheetSage2')
d('m-a-p/MERT-v2-FullSong', revision='d8ba1c745e733b3908ce6ad16ebeb17ac7600a42', local_dir='models/MERT-v2-FullSong')"
```

SheetSage2 needs `ffmpeg` on `PATH`. On an M1 Max 64 GB it runs on MPS: in a test on 2026-10-11 it
transcribed a 36 s track in 42 s on its first run, at a peak of about 16 GB of memory,
and a 38 s track in 18 s in a later server job. Both times include loading the model. On CUDA it runs in BF16; on MPS and on the
CPU it runs in FP32.

## Licence

This server is part of cav. YuE2's code is Apache 2.0. Its weights are CC BY-NC 4.0, with a
separate permission: personal users, creators and musicians acting as individuals may
use and sell what they make. A company needs a commercial licence from the YuE2 authors.
Read `MODEL_LICENSE` in the YuE2 repository.

SheetSage2 and MERT-v2-FullSong have weights under CC BY-NC 4.0 too (their `LICENSE`
files). A cover also plays someone else's song: whoever uses it needs the songwriter's
permission or a cover licence.

## Tests

```bash
python3 -m unittest discover -s tools/yue2-server
```

The tests use a fake pipeline. Set `YUE2_ROOT` to a YuE2 checkout to also run the
helper's real score conversion.
