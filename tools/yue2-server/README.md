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

When the environment variable `YUE2_API_KEY` is set, every route except the health routes
needs it as a Bearer key. cav sends the key from `cav config set-key yue2 <source>`, or from
`YUE2_API_KEY`.

## API

The routes and fields are a subset of the API of
[YuE2-Turbo](https://github.com/NoizAI/YuE2-Turbo), NOIZ's serving layer for YuE2
(`src/yue2/service.py`, read on 2026-10-11). One client can then work with either server.
Turbo needs Linux, CUDA and vLLM, so it does not run on an Apple silicon Mac or, probably,
on a 12 GB GPU. This server runs on both.

| Route | What it does |
|---|---|
| `GET /health/live` | `{"status": "alive"}` |
| `GET /health/ready` | 503 while the model loads (or failed to load), then 200 with `"extensions": ["instrumental"]` |
| `POST /v1/jobs` | Starts a job and answers 202 with it. Takes an `Idempotency-Key` header. 429 with `Retry-After` when the queue is full. |
| `GET /v1/jobs/{id}` | The job. `status` is `queued`, `running`, `succeeded`, `truncated`, `failed` or `cancelled`; `stage` says what it is doing. |
| `POST /v1/jobs/{id}/cancel` | Cancels the job. |
| `GET /v1/jobs/{id}/audio` | The audio, FLAC, 48 kHz stereo. |
| `GET /v1/jobs/{id}/score` | The ABC score that YuE2 generated the audio from. |

Job fields, as in Turbo: `style`, `lyrics` (with `[Verse]` and `[Chorus]` markers), `cot`
(`full`, `melody` or `off`), `seed`, `n` (only 1 here), `abc`, `cfg_scale`. A finished
job's `result` has `audio_seconds`, `sample_rate`, `truncated` (`abc` and `semantic`),
`timing` and `configuration`. `POST /v1/covers` answers 501: covers need Turbo.

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

## Licence

This server is part of cav. YuE2's code is Apache 2.0. Its weights are CC BY-NC 4.0, with a
separate permission: personal users, creators and musicians acting as individuals may
use and sell what they make. A company needs a commercial licence from the YuE2 authors.
Read `MODEL_LICENSE` in the YuE2 repository.

## Tests

```bash
python3 -m unittest discover -s tools/yue2-server
```

The tests use a fake pipeline. Set `YUE2_ROOT` to a YuE2 checkout to also run the
helper's real score conversion.
