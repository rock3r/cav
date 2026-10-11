#!/usr/bin/env python3
"""A small HTTP server that keeps one YuE2 pipeline loaded and runs song jobs.

cav's `yue2` music service talks to it. Run it with the Python of a YuE2 checkout:

    uv run --project ~/src/YuE --no-sync tools/yue2-server/yue2_server.py --device mps

The routes and fields are a subset of the YuE2-Turbo server's API
(https://github.com/NoizAI/YuE2-Turbo, src/yue2/service.py, read on 2026-10-11), so one
client works with either server:

    GET  /health/live            200 {"status": "alive"}
    GET  /health/ready           503 while the model loads, then 200
    POST /v1/jobs                202 with the job; takes an Idempotency-Key header
    GET  /v1/jobs/{id}           status: queued, running, succeeded, truncated, failed, cancelled
    POST /v1/jobs/{id}/cancel
    GET  /v1/jobs/{id}/audio     FLAC
    GET  /v1/jobs/{id}/score     ABC

Job fields: style, lyrics, cot (full, melody or off), seed, n (this server takes only 1),
abc, cfg_scale. When YUE2_API_KEY is set, every route but the health routes needs it as a
Bearer key. A full queue answers 429 with Retry-After.

Extension, not in YuE2-Turbo: "instrumental": true makes a track with no vocals, the way
the MIT-licensed instrumental helper in the YuE2 repository does it
(skills/yue2-music/instrumental): YuE2 plans a score, the helper moves the notes of the
Vocal voice to the Ins voice, then YuE2 generates audio from the changed score. "lyrics" are
then only section tags for the planner ("[Intro]\\n\\n[Verse]..."); nobody sings them. With
"abc", the server converts that score instead of planning one. The result has an extra
"instrumental" object with the score's nominal length. /health/ready lists "instrumental"
in "extensions", so a client can tell this server from YuE2-Turbo.

Only the Python standard library is used here; YuE2 and the helper come from the checkout.
"""
from __future__ import annotations

import argparse
import hmac
import json
import os
from pathlib import Path
import queue
import re
import shutil
import sys
import tempfile
import threading
import time
import traceback
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import uuid

SERVER = "cav-yue2-server"
TERMINAL = {"succeeded", "truncated", "failed", "cancelled"}
FIELDS = {"n", "style", "lyrics", "cot", "seed", "abc", "cfg_scale", "instrumental", "seconds"}
BODY_LIMIT = 256 * 1024
NO_VOCALS = ("no vocals", "no singing", "no choir", "no spoken words")


class Invalid(ValueError):
    """A request the server will not run (HTTP 422)."""


def validate(body):
    """Check a job request against YuE2-Turbo's GenerateRequest rules, plus instrumental."""
    if not isinstance(body, dict):
        raise Invalid("the body must be a JSON object")
    unknown = sorted(set(body) - FIELDS)
    if unknown:
        raise Invalid(f"unknown fields: {unknown}")
    req = {"n": body.get("n", 1), "style": body.get("style"), "lyrics": body.get("lyrics"),
           "cot": body.get("cot", "full"), "seed": body.get("seed", 831001), "abc": body.get("abc"),
           "cfg_scale": body.get("cfg_scale"), "instrumental": body.get("instrumental", False),
           "seconds": body.get("seconds")}
    if type(req["n"]) is not int or req["n"] != 1:
        raise Invalid("this server makes one take per job: n must be 1")
    for name, limit in (("style", 2000), ("lyrics", 16000)):
        value = req[name]
        if not isinstance(value, str) or not value.strip() or len(value) > limit:
            raise Invalid(f"{name} must be text of 1 to {limit} characters")
    if req["cot"] not in ("full", "melody", "off"):
        raise Invalid("cot must be full, melody or off")
    if type(req["seed"]) is not int or not 0 <= req["seed"] < 2**63:
        raise Invalid("seed must be an integer from 0 to 2**63-1")
    if req["abc"] is not None and (not isinstance(req["abc"], str) or not req["abc"].strip() or len(req["abc"]) > 64000):
        raise Invalid("abc must be text of 1 to 64000 characters")
    if req["abc"] is not None and req["cot"] == "off":
        raise Invalid("abc needs cot full or melody")
    cfg = req["cfg_scale"]
    if cfg is not None and (isinstance(cfg, bool) or not isinstance(cfg, (int, float)) or not 0 <= cfg <= 20):
        raise Invalid("cfg_scale must be a number from 0 to 20")
    if not isinstance(req["instrumental"], bool):
        raise Invalid("instrumental must be true or false")
    if req["instrumental"] and req["cot"] == "off":
        raise Invalid("instrumental needs cot full or melody: it works on the planned score")
    sec = req["seconds"]
    if sec is not None and (isinstance(sec, bool) or not isinstance(sec, (int, float)) or not 5 <= sec <= 600):
        raise Invalid("seconds must be a number from 5 to 600")
    if sec is not None and not req["instrumental"]:
        raise Invalid("seconds needs instrumental: the server fits the planned score")
    return req


def instrumental_style(style):
    """The style the helper's prepare step writes: starts with Instrumental, says no vocals."""
    style = style.strip().rstrip(".,")
    if not re.match(r"^instrumental\b", style, re.I):
        style = "Instrumental, " + style
    for condition in NO_VOCALS:
        if condition not in style.lower():
            style += ", " + condition
    return style + "."


def fit_score(helper, text, seconds):
    """Cuts a converted score to about `seconds` at its section boundaries.

    YuE2 has no length setting, but its audio follows the planned score closely: in 5 takes
    on 2026-10-11 the audio was within 3 s of the score's nominal length. The planner writes
    as much as it likes, so the server keeps the score from the start up to a cut point (a
    section start, or every fourth bar inside a section) and, when the score ends with an
    outro, keeps the whole outro after it. It picks the cut whose length is closest to the
    request. Returns the new score and its nominal length in seconds; a score that is
    already closest to the request comes back unchanged.
    """
    from fractions import Fraction
    score = helper.parse_abc(text)
    vocal, ins = score.voices["Vocal"], score.voices["Ins"]
    sec_per_quarter = Fraction(60) / score.bpm
    total = vocal.time
    starts = helper.section_starts(text, score)
    bar_starts = [b[0] for b in vocal.bars]
    sections = sorted(set(starts) | {Fraction(0)})
    ends = sections[1:] + [total]
    # Cut points: section starts, and every fourth bar inside a section (a phrase). An outro
    # stays whole, so a cut score can still end on it.
    phrases, outro = [], None
    for a, b in zip(sections, ends):
        if starts.get(a) == "outro" and b == total:
            outro = (a, b)
            continue
        inside = [t for t in bar_starts if a <= t < b]
        cuts = inside[::4] + [b]
        phrases += list(zip(cuts, cuts[1:]))
    target = Fraction(seconds).limit_denominator(1000) / sec_per_quarter
    options = []
    for k in range(1, len(phrases) + 1):
        options.append(phrases[:k])
        if outro is not None:
            options.append(phrases[:k] + [outro])
    if outro is not None:
        options.append([outro])
    segments = phrases + ([outro] if outro else [])
    best = min(options, key=lambda o: (abs(sum(b - a for a, b in o) - target), len(o)))
    if best == segments:
        return text, float(total * sec_per_quarter)
    # Rebuild the score from the kept segments, shifted to follow one another.
    bars, notes, chords = [], [], []
    at = Fraction(0)
    keys = vocal.keys
    for a, b in best:
        shift = at - a
        for start, _, (n, d) in vocal.bars:
            if a <= start < b:
                bar = {"meter": f"{n}/{d}", "key": next(k for t, k in reversed(keys) if t <= start)}
                if start in starts:
                    bar["section"] = starts[start]
                bars.append(bar)
        for t, pitch, dur in ins.notes:
            if a <= t < b:
                notes.append([str(t + shift), str(min(dur, b - t)), pitch])
        active = next((s for t, s in reversed(vocal.chords) if t <= a), None)
        if active is not None:
            chords.append([str(at), active])
        for t, s in vocal.chords:
            if a < t < b:
                chords.append([str(t + shift), s])
        at += b - a
    dedup = []
    for when, symbol in chords:
        if dedup and dedup[-1][0] == when:
            dedup[-1] = [when, symbol]
        elif not dedup or dedup[-1][1] != symbol:
            dedup.append([when, symbol])
    fitted, _ = helper.compile_events({"bpm": score.bpm, "key": bars[0]["key"], "bars": bars,
                                       "notes": notes, "chords": dedup})
    return fitted, float(at * sec_per_quarter)


class Jobs:
    """Jobs in memory, with their artifacts in one folder. One worker runs them in order."""

    def __init__(self, root, max_pending=16, keep=200):
        self.root = Path(root)
        self.max_pending, self.keep = max_pending, keep
        self.lock = threading.Lock()
        self.jobs, self.requests, self.cancels, self.idem = {}, {}, {}, {}
        self.queue = queue.Queue()

    def submit(self, req, key=None):
        """Returns (job, created). Raises Full or Conflict."""
        fingerprint = json.dumps(req, sort_keys=True)
        with self.lock:
            if key is not None and key in self.idem:
                old_print, job_id = self.idem[key]
                if old_print != fingerprint:
                    raise Conflict()
                return dict(self.jobs[job_id]), False
            pending = sum(j["status"] in ("queued", "running") for j in self.jobs.values())
            if pending >= self.max_pending:
                raise Full()
            now = time.time()
            job_id = uuid.uuid4().hex
            job = {"id": job_id, "status": "queued", "stage": "queued", "created_at": now, "updated_at": now,
                   "seed": req["seed"], "cancel_requested": False, "result": None, "error": None}
            self.jobs[job_id], self.requests[job_id] = job, req
            self.cancels[job_id] = threading.Event()
            if key is not None:
                self.idem[key] = (fingerprint, job_id)
            self._forget_old()
        self.queue.put(job_id)
        return dict(job), True

    def _forget_old(self):
        done = sorted((j for j in self.jobs.values() if j["status"] in TERMINAL), key=lambda j: j["created_at"])
        for job in done[:max(0, len(self.jobs) - self.keep)]:
            job_id = job["id"]
            for table in (self.jobs, self.requests, self.cancels):
                table.pop(job_id, None)
            self.idem = {k: v for k, v in self.idem.items() if v[1] != job_id}
            shutil.rmtree(self.root / job_id, ignore_errors=True)

    def get(self, job_id):
        with self.lock:
            job = self.jobs.get(job_id)
            return dict(job) if job else None

    def update(self, job_id, **fields):
        with self.lock:
            job = self.jobs[job_id]
            job.update(fields, updated_at=time.time())
            if fields.get("status") in TERMINAL:
                job.update(stage="finished", finished_at=job["updated_at"])

    def cancel(self, job_id):
        with self.lock:
            job = self.jobs.get(job_id)
            if job is None:
                return None
            if job["status"] not in TERMINAL:
                job["cancel_requested"] = True
                self.cancels[job_id].set()
                if job["status"] == "queued":
                    now = time.time()
                    job.update(status="cancelled", stage="finished", updated_at=now, finished_at=now)
            return dict(job)

    def pending(self):
        with self.lock:
            return sum(j["status"] in ("queued", "running") for j in self.jobs.values())


class Full(Exception):
    pass


class Conflict(Exception):
    pass


class Engine:
    """Runs one request on a YuE2 pipeline. helper is the instrumental helper's scripts module set."""

    def __init__(self, pipe, helper=None):
        self.pipe, self.helper = pipe, helper

    def run(self, req, out, cancelled, stage):
        """Writes the artifacts into out and returns the job result (without the URLs)."""
        common = {"seed": req["seed"], "cfg_scale": req["cfg_scale"], "cancelled": cancelled}
        extra = None
        plan_truncated = False
        if req["instrumental"]:
            if self.helper is None:
                raise RuntimeError("the instrumental helper is not loaded (see --yue2)")
            abc, origin = req["abc"], "request"
            if abc is None:
                stage("planning")
                plan = self.pipe.plan(style=req["style"], lyrics=req["lyrics"], cot=req["cot"],
                                      seed=req["seed"], cancelled=cancelled)
                plan_truncated = bool(plan.truncated)
                if plan_truncated or not plan.abc:
                    raise Invalid("YuE2 planned an empty or truncated score; try another seed")
                abc, origin = plan.abc, "YuE2"
            out.mkdir(parents=True, exist_ok=True)
            (out / "original.abc").write_text(abc, encoding="utf-8")
            stage("converting")
            converted, transfer = self.helper.convert_score(abc, overlap="vocal", keep_chords=True)
            planned = self.helper.report(self.helper.validate_score(converted))["nominal_duration_seconds"]
            if req["seconds"] is not None:
                stage("fitting")
                converted, _ = fit_score(self.helper, converted, req["seconds"])
            score = self.helper.validate_score(converted)
            if len(self.pipe.tokenizer.encode(converted)) > 4096:
                raise Invalid("the score is longer than the 4096-token planning budget")
            report = self.helper.report(score)
            final = {"style": instrumental_style(req["style"]), "lyrics": self.helper.lyric_tags(converted),
                     "abc": converted, "cot": "full" if score.voices["Vocal"].chords else "melody"}
            extra = {"score_origin": origin, "nominal_seconds": report["nominal_duration_seconds"],
                     "planned_seconds": planned,
                     "bpm": report["bpm"], "vocal_notes_moved": transfer["vocal_notes_before"],
                     "ins_notes_shortened": len(transfer["affected_ins_notes"]),
                     "style": final["style"], "lyrics": final["lyrics"]}
            stage("generating")
            song = self.pipe(**final, **common)
        else:
            stage("generating")
            song = self.pipe(style=req["style"], lyrics=req["lyrics"], cot=req["cot"], abc=req["abc"], **common)
        stage("saving")
        saved = song.save_artifacts(out)
        truncated = dict(saved["truncated"])
        if req["instrumental"]:
            # The final pass took a fixed score; the planner's flag is the one that counts.
            truncated["abc"] = plan_truncated
        result = {"audio_seconds": saved["audio_seconds"], "sample_rate": saved["sample_rate"],
                  "truncated": truncated, "timing": saved["timing"], "configuration": song.config}
        if extra is not None:
            result["instrumental"] = extra
        return result


class Worker:
    def __init__(self, jobs, load, log=sys.stderr):
        self.jobs, self.load, self.log = jobs, load, log
        self.engine, self.startup_error, self.info = None, None, {}
        self.thread = threading.Thread(target=self._main, daemon=True)

    def start(self):
        self.thread.start()

    def ready(self):
        return self.engine is not None

    def _main(self):
        try:
            self.engine, self.info = self.load()
        except BaseException as exc:  # report it on /health/ready instead of dying silently
            self.startup_error = f"{type(exc).__name__}: {exc}"
            traceback.print_exc(file=self.log)
            return
        while True:
            job_id = self.jobs.queue.get()
            if job_id is None:
                return
            self.run(job_id)

    def run(self, job_id):
        job = self.jobs.get(job_id)
        if job is None or job["status"] != "queued":
            return
        req, cancel = self.jobs.requests[job_id], self.jobs.cancels[job_id]
        out = self.jobs.root / job_id
        start = time.time()
        self.jobs.update(job_id, status="running", stage="claimed", started_at=start)

        def cancelled():
            return cancel.is_set()

        def stage(name):
            if cancel.is_set():
                raise InterruptedError("cancelled")
            self.jobs.update(job_id, stage=name)

        try:
            result = self.engine.run(req, out, cancelled, stage)
            if cancel.is_set():
                raise InterruptedError("cancelled")
            # e2e_seconds covers only the final pass; job_seconds also covers planning.
            result["timing"] = dict(result["timing"], job_seconds=time.time() - start)
            result["audio_url"] = f"/v1/jobs/{job_id}/audio"
            result["score_url"] = f"/v1/jobs/{job_id}/score" if (out / "score.abc").is_file() else None
            status = "truncated" if any(result["truncated"].values()) else "succeeded"
            self.jobs.update(job_id, status=status, result=result)
            print(f"job {job_id}: {status}, {result['audio_seconds']:.1f} s of audio in {time.time() - start:.1f} s",
                  file=self.log, flush=True)
        except InterruptedError:
            self.jobs.update(job_id, status="cancelled")
        except Exception as exc:
            traceback.print_exc(file=self.log)
            code = "invalid_generation" if isinstance(exc, (ValueError, TypeError)) else "inference_failed"
            self.jobs.update(job_id, status="failed", error={"code": code, "message": f"{type(exc).__name__}: {exc}"})


def make_handler(jobs, worker, api_key):
    class Handler(BaseHTTPRequestHandler):
        server_version = SERVER

        def log_message(self, fmt, *args):
            if not self.path.startswith("/v1/jobs/") or self.command != "GET":
                super().log_message(fmt, *args)

        def send_json(self, code, value, headers=None):
            data = json.dumps(value).encode()
            self.send_response(code)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            for k, v in (headers or {}).items():
                self.send_header(k, v)
            self.end_headers()
            self.wfile.write(data)

        def detail(self, code, message, headers=None):
            self.send_json(code, {"detail": message}, headers)

        def authorized(self):
            if not api_key:
                return True
            given = self.headers.get("Authorization", "")
            if given.startswith("Bearer ") and hmac.compare_digest(given[7:].encode(), api_key.encode()):
                return True
            self.detail(401, "Invalid bearer token", {"WWW-Authenticate": "Bearer"})
            return False

        def route(self):
            parts = [p for p in self.path.split("?")[0].split("/") if p]
            return parts

        def do_GET(self):
            parts = self.route()
            if parts == ["health", "live"]:
                return self.send_json(200, {"status": "alive"})
            if parts == ["health", "ready"]:
                if worker.startup_error:
                    return self.send_json(503, {"status": "failed", "error": worker.startup_error})
                if not worker.ready():
                    return self.send_json(503, {"status": "loading"})
                return self.send_json(200, {"status": "ready", "server": SERVER, "extensions": ["instrumental"],
                                            **worker.info})
            if len(parts) < 3 or parts[:2] != ["v1", "jobs"] or len(parts) > 4:
                return self.detail(404, "Not found")
            if not self.authorized():
                return
            job = jobs.get(parts[2])
            if job is None:
                return self.detail(404, "Job not found")
            if len(parts) == 3:
                return self.send_json(200, job)
            names = {"audio": ("audio.flac", "audio/flac"), "score": ("score.abc", "text/plain; charset=utf-8")}
            if parts[3] not in names:
                return self.detail(404, "Not found")
            if job["status"] not in ("succeeded", "truncated"):
                return self.detail(409, "Artifact is not available for this job state")
            name, media = names[parts[3]]
            path = jobs.root / job["id"] / name
            if not path.is_file():
                return self.detail(404, "Artifact not found")
            data = path.read_bytes()
            self.send_response(200)
            self.send_header("Content-Type", media)
            self.send_header("Content-Length", str(len(data)))
            self.send_header("Content-Disposition", f'attachment; filename="{job["id"]}-{name}"')
            self.end_headers()
            self.wfile.write(data)

        def do_POST(self):
            parts = self.route()
            if len(parts) == 4 and parts[:2] == ["v1", "jobs"] and parts[3] == "cancel":
                if not self.authorized():
                    return
                job = jobs.cancel(parts[2])
                return self.send_json(200, job) if job else self.detail(404, "Job not found")
            if parts == ["v1", "covers"]:
                return self.detail(501, "covers need YuE2-Turbo; this server does not transcribe audio")
            if parts != ["v1", "jobs"]:
                return self.detail(404, "Not found")
            if not self.authorized():
                return
            length = int(self.headers.get("Content-Length") or 0)
            if length > BODY_LIMIT:
                return self.detail(413, "Request body is too large")
            try:
                req = validate(json.loads(self.rfile.read(length) or b"null"))
            except (ValueError, Invalid) as exc:
                return self.detail(422, str(exc))
            if worker.startup_error or not worker.ready():
                return self.detail(503, "Inference worker is not ready", {"Retry-After": "5"})
            key = self.headers.get("Idempotency-Key")
            if key is not None and not 1 <= len(key) <= 128:
                return self.detail(422, "Idempotency-Key must be 1 to 128 characters")
            try:
                job, created = jobs.submit(req, key)
            except Full:
                return self.detail(429, "Queue is full", {"Retry-After": "5"})
            except Conflict:
                return self.detail(409, "Idempotency-Key was already used with different input")
            self.send_json(202 if created else 200, job, {"Location": f"/v1/jobs/{job['id']}"})

    return Handler


def helper_modules(yue2_root):
    """Imports the instrumental helper's scripts (MIT) from a YuE2 checkout."""
    scripts = Path(yue2_root) / "skills/yue2-music/instrumental/scripts"
    if not (scripts / "instrumentalize.py").is_file():
        raise FileNotFoundError(f"no instrumental helper at {scripts}; pass --yue2 <YuE2 checkout>")
    sys.path.insert(0, str(scripts))
    import abc_tools
    import compile_score
    import instrumental
    import instrumentalize

    class Helper:
        convert_score = staticmethod(instrumentalize.convert_score)
        section_starts = staticmethod(instrumentalize.section_starts)
        compile_events = staticmethod(compile_score.compile_events)
        parse_abc = staticmethod(abc_tools.parse_abc)
        validate_score = staticmethod(instrumental.validate_score)
        lyric_tags = staticmethod(instrumental.lyric_tags)
        report = staticmethod(abc_tools.report)

    return Helper


def find_checkout(package):
    """The YuE2 checkout: $YUE2_KIT, the source tree of an editable install, or the folder
    that holds this interpreter's virtual environment (uv run --project <checkout>)."""
    candidates = [os.environ.get("YUE2_KIT"), package.resolve().parents[2], Path(sys.prefix).parent]
    for c in candidates:
        if c and (Path(c) / "skills/yue2-music/instrumental").is_dir():
            return Path(c)
    raise FileNotFoundError("cannot find the YuE2 checkout for the instrumental helper; pass --yue2 <checkout>")


def loader(args):
    def load():
        import yue2
        from yue2 import YuE2Pipeline
        root = Path(args.yue2) if args.yue2 else find_checkout(Path(yue2.__file__))
        helper = helper_modules(root)
        model, vae = args.model, args.vae
        if model is None:
            local = root / "models/YuE2-3B"
            model = str(local) if local.is_dir() else "m-a-p/YuE2-3B"
        if vae is None:
            local = root / "models/YuE2-Vae"
            vae = str(local) if local.is_dir() else "m-a-p/YuE2-Vae"
        pipe = YuE2Pipeline.from_pretrained(model, vae=vae, device=args.device, memory_budget_gib=args.budget,
                                            local_files_only=args.offline)
        info = {"device": str(pipe.device), "budget_gib": args.budget, "model": model, "vae": vae}
        print(f"ready: {model} on {pipe.device}, budget {args.budget:g} GiB", file=sys.stderr, flush=True)
        return Engine(pipe, helper), info
    return load


def main(argv=None):
    p = argparse.ArgumentParser(description="Serve YuE2 song jobs over HTTP for cav (a subset of the YuE2-Turbo API).")
    p.add_argument("--host", default="127.0.0.1", help="address to listen on (0.0.0.0 for other machines)")
    p.add_argument("--port", type=int, default=8002)
    p.add_argument("--device", default="auto", help="auto, cuda, mps or cpu")
    p.add_argument("--budget", type=float, default=24, help="memory budget in GiB; 12 decodes audio in smaller pieces")
    p.add_argument("--yue2", help="YuE2 checkout, for the instrumental helper and local models (default: where yue2 is installed)")
    p.add_argument("--model", help="model path or Hugging Face id (default: m-a-p/YuE2-3B)")
    p.add_argument("--vae", help="audio decoder path or Hugging Face id (default: m-a-p/YuE2-Vae)")
    p.add_argument("--offline", action="store_true", help="use only models already downloaded")
    p.add_argument("--max-pending", type=int, default=16, help="queued and running jobs before 429")
    p.add_argument("--artifacts", help="folder for job files (default: a temporary folder, removed on exit)")
    args = p.parse_args(argv)
    api_key = os.environ.get("YUE2_API_KEY", "")
    if args.host not in ("127.0.0.1", "localhost", "::1") and not api_key:
        print("warning: listening beyond this machine with no YUE2_API_KEY: anyone on the network can use the GPU",
              file=sys.stderr)
    temp = None
    root = args.artifacts
    if root is None:
        temp = tempfile.mkdtemp(prefix="yue2-server-")
        root = temp
    jobs = Jobs(root, max_pending=args.max_pending)
    worker = Worker(jobs, loader(args))
    worker.start()
    httpd = ThreadingHTTPServer((args.host, args.port), make_handler(jobs, worker, api_key))
    print(f"listening on http://{args.host}:{args.port} (loading the model…)", file=sys.stderr, flush=True)
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        httpd.server_close()
        if temp:
            shutil.rmtree(temp, ignore_errors=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
