"""Tests for yue2_server.py with a fake pipeline: python -m unittest discover -s tools/yue2-server

Set YUE2_ROOT to a YuE2 checkout to also run the instrumental helper's real conversion.
"""
import json
import os
from pathlib import Path
import tempfile
import threading
import time
import unittest
import urllib.error
import urllib.request

import yue2_server as srv


class FakeSong:
    def __init__(self, abc, truncated=False):
        self.abc, self.config, self._truncated = abc, {"device": "fake"}, truncated

    def save_artifacts(self, directory):
        directory = Path(directory)
        directory.mkdir(parents=True, exist_ok=True)
        (directory / "audio.flac").write_bytes(b"fLaCfake")
        if self.abc is not None:
            (directory / "score.abc").write_text(self.abc, encoding="utf-8")
        return {"truncated": {"abc": False, "semantic": self._truncated}, "audio_seconds": 30.5,
                "sample_rate": 48000, "timing": {"e2e_seconds": 1.0}}


class FakePlan:
    def __init__(self, abc, truncated=False):
        self.abc, self.truncated = abc, truncated


class FakeTokenizer:
    def encode(self, text):
        return text.split()


class FakePipe:
    tokenizer = FakeTokenizer()

    def __init__(self, gate=None, semantic_truncated=False):
        self.calls, self.gate, self.semantic_truncated = [], gate, semantic_truncated

    def plan(self, **kw):
        self.calls.append(("plan", kw))
        return FakePlan("PLANNED-WITH-VOCAL")

    def __call__(self, **kw):
        self.calls.append(("song", kw))
        if self.gate is not None:
            while not self.gate.wait(0.01):
                if kw["cancelled"]():
                    raise InterruptedError("cancelled")
        return FakeSong(kw.get("abc"), self.semantic_truncated)


class FakeScore:
    class V:
        chords = [(0, "C")]
    voices = {"Vocal": V()}


class FakeHelper:
    @staticmethod
    def convert_score(text, overlap, keep_chords):
        return "CONVERTED " + text, {"vocal_notes_before": 7, "affected_ins_notes": [{}]}

    @staticmethod
    def validate_score(text):
        return FakeScore()

    @staticmethod
    def lyric_tags(text):
        return "[Intro]\n\n[Outro]\n"

    @staticmethod
    def report(score):
        return {"nominal_duration_seconds": 32.0, "bpm": 96}


class FakeTranscriber:
    device = "fake-device"

    def __init__(self, fail=False):
        self.calls, self.fail = [], fail

    def __call__(self, audio, out, cancelled):
        self.calls.append(Path(audio).read_bytes())
        if self.fail:
            raise srv.TranscriptionFailed("SheetSage2 could not transcribe the recording: no melody")
        return "TRANSCRIBED-MELODY", ["short input"]


def form(fields, audio=b"ID3fake", filename="song.mp3"):
    """A multipart/form-data body, as cav and curl -F send it."""
    boundary = "cavtestboundary"
    out = b""
    for name, value in fields.items():
        out += (f"--{boundary}\r\nContent-Disposition: form-data; name=\"{name}\"\r\n\r\n{value}\r\n").encode()
    if audio is not None:
        out += (f"--{boundary}\r\nContent-Disposition: form-data; name=\"audio\"; filename=\"{filename}\"\r\n"
                "Content-Type: application/octet-stream\r\n\r\n").encode() + audio + b"\r\n"
    out += f"--{boundary}--\r\n".encode()
    return out, f"multipart/form-data; boundary={boundary}"


class ServerTest(unittest.TestCase):
    def start(self, pipe, key="", max_pending=16, helper=FakeHelper, transcriber=None):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        jobs = srv.Jobs(tmp.name, max_pending=max_pending)
        worker = srv.Worker(jobs, lambda: (srv.Engine(pipe, helper, transcriber), {"device": "fake", "model": "fake-model"}),
                            log=open(os.devnull, "w"))
        worker.start()
        httpd = srv.ThreadingHTTPServer(("127.0.0.1", 0), srv.make_handler(jobs, worker, key))
        threading.Thread(target=httpd.serve_forever, daemon=True).start()
        self.addCleanup(httpd.shutdown)
        self.base = f"http://127.0.0.1:{httpd.server_address[1]}"
        self.key = key
        for _ in range(100):
            if worker.ready():
                break
            time.sleep(0.01)
        return jobs

    def call(self, method, path, body=None, headers=None):
        h = dict(headers or {})
        if self.key:
            h.setdefault("Authorization", "Bearer " + self.key)
        data = None
        if isinstance(body, tuple):  # a form from form()
            data, h["Content-Type"] = body
        elif body is not None:
            data = json.dumps(body).encode()
            h["Content-Type"] = "application/json"
        req = urllib.request.Request(self.base + path, data=data, method=method, headers=h)
        try:
            with urllib.request.urlopen(req) as r:
                return r.status, r.read(), r.headers
        except urllib.error.HTTPError as e:
            return e.code, e.read(), e.headers

    def wait(self, job_id):
        for _ in range(500):
            code, body, _ = self.call("GET", "/v1/jobs/" + job_id)
            job = json.loads(body)
            if job["status"] in srv.TERMINAL:
                return job
            time.sleep(0.01)
        self.fail("job did not finish")

    def test_instrumental_plans_converts_and_generates(self):
        pipe = FakePipe()
        self.start(pipe)
        code, body, headers = self.call("POST", "/v1/jobs", {"style": "warm synthwave", "lyrics": "[Intro]\n\n[Outro]",
                                                             "seed": 7, "instrumental": True})
        self.assertEqual(code, 202, body)
        self.assertTrue(headers["Location"].startswith("/v1/jobs/"))
        job = self.wait(json.loads(body)["id"])
        self.assertEqual(job["status"], "succeeded", job)
        res = job["result"]
        self.assertEqual(res["truncated"], {"abc": False, "semantic": False})
        self.assertEqual(res["instrumental"]["nominal_seconds"], 32.0)
        self.assertGreaterEqual(res["timing"]["job_seconds"], 0)
        self.assertEqual(res["model"], "fake-model")
        self.assertEqual(res["instrumental"]["score_origin"], "YuE2")
        self.assertEqual(res["audio_url"], f"/v1/jobs/{job['id']}/audio")
        (kind, plan), (kind2, song) = pipe.calls
        self.assertEqual((kind, plan["style"], plan["cot"], plan["seed"]), ("plan", "warm synthwave", "full", 7))
        self.assertEqual(kind2, "song")
        self.assertEqual(song["abc"], "CONVERTED PLANNED-WITH-VOCAL")
        self.assertEqual(song["style"], "Instrumental, warm synthwave, no vocals, no singing, no choir, no spoken words.")
        self.assertEqual(song["lyrics"], "[Intro]\n\n[Outro]\n")
        self.assertEqual(song["cot"], "full")
        code, audio, headers = self.call("GET", res["audio_url"])
        self.assertEqual((code, audio, headers["Content-Type"]), (200, b"fLaCfake", "audio/flac"))
        code, score, _ = self.call("GET", res["score_url"])
        self.assertEqual((code, score), (200, b"CONVERTED PLANNED-WITH-VOCAL"))

    def test_turbo_fields_without_instrumental_go_straight_to_the_pipeline(self):
        pipe = FakePipe(semantic_truncated=True)
        self.start(pipe)
        code, body, _ = self.call("POST", "/v1/jobs", {"style": "pop", "lyrics": "[Verse]\nla la", "cot": "melody", "cfg_scale": 1.5})
        self.assertEqual(code, 202, body)
        job = self.wait(json.loads(body)["id"])
        self.assertEqual(job["status"], "truncated")
        self.assertNotIn("instrumental", job["result"])
        self.assertEqual([c[0] for c in pipe.calls], ["song"])
        self.assertEqual(pipe.calls[0][1]["cfg_scale"], 1.5)

    def test_validation(self):
        self.start(FakePipe())
        for body in ({"style": "x"}, {"style": "x", "lyrics": "y", "n": 2}, {"style": "x", "lyrics": "y", "bogus": 1},
                     {"style": "x", "lyrics": "y", "cot": "off", "instrumental": True},
                     {"style": "x", "lyrics": "y", "seed": -1}, {"style": " ", "lyrics": "y"},
                     {"style": "x", "lyrics": "y", "seconds": 30},
                     {"style": "x", "lyrics": "y", "instrumental": True, "seconds": 2}):
            code, resp, _ = self.call("POST", "/v1/jobs", body)
            self.assertEqual(code, 422, (body, resp))

    def test_bearer_key(self):
        self.start(FakePipe(), key="k" * 20)
        code, _, _ = self.call("GET", "/health/ready", headers={"Authorization": "none"})
        self.assertEqual(code, 200)
        code, _, headers = self.call("POST", "/v1/jobs", {"style": "x", "lyrics": "y"}, headers={"Authorization": "Bearer wrong"})
        self.assertEqual(code, 401)
        self.assertEqual(headers["WWW-Authenticate"], "Bearer")
        code, _, _ = self.call("POST", "/v1/jobs", {"style": "x", "lyrics": "y"})
        self.assertEqual(code, 202)

    def test_health_ready_lists_the_extension(self):
        self.start(FakePipe())
        code, body, _ = self.call("GET", "/health/ready")
        ready = json.loads(body)
        self.assertEqual((code, ready["status"], ready["extensions"]), (200, "ready", ["instrumental"]))
        self.assertEqual(json.loads(self.call("GET", "/health/live")[1]), {"status": "alive"})

    def test_idempotency_queue_and_cancel(self):
        gate = threading.Event()
        self.addCleanup(gate.set)
        self.start(FakePipe(gate=gate), max_pending=2)
        req = {"style": "x", "lyrics": "y"}
        code, first, _ = self.call("POST", "/v1/jobs", req, headers={"Idempotency-Key": "a"})
        self.assertEqual(code, 202)
        code, again, _ = self.call("POST", "/v1/jobs", req, headers={"Idempotency-Key": "a"})
        self.assertEqual((code, json.loads(again)["id"]), (200, json.loads(first)["id"]))
        code, _, _ = self.call("POST", "/v1/jobs", {"style": "x", "lyrics": "z"}, headers={"Idempotency-Key": "a"})
        self.assertEqual(code, 409)
        code, second, _ = self.call("POST", "/v1/jobs", {"style": "x", "lyrics": "z"})
        self.assertEqual(code, 202)
        code, _, headers = self.call("POST", "/v1/jobs", {"style": "x", "lyrics": "w"})
        self.assertEqual((code, headers["Retry-After"]), (429, "5"))
        second_id = json.loads(second)["id"]
        code, body, _ = self.call("POST", f"/v1/jobs/{second_id}/cancel")
        self.assertEqual((code, json.loads(body)["status"]), (200, "cancelled"))
        first_id = json.loads(first)["id"]
        self.call("POST", f"/v1/jobs/{first_id}/cancel")
        self.assertEqual(self.wait(first_id)["status"], "cancelled")
        code, _, _ = self.call("GET", f"/v1/jobs/{first_id}/audio")
        self.assertEqual(code, 409)

    def test_failure_is_reported(self):
        class Broken(FakePipe):
            def plan(self, **kw):
                return FakePlan(None, truncated=True)
        self.start(Broken())
        code, body, _ = self.call("POST", "/v1/jobs", {"style": "x", "lyrics": "[Intro]", "instrumental": True})
        job = self.wait(json.loads(body)["id"])
        self.assertEqual((job["status"], job["error"]["code"]), ("failed", "invalid_generation"))

    def test_instrumental_cover_transcribes_then_moves_the_melody(self):
        pipe, tr = FakePipe(), FakeTranscriber()
        jobs = self.start(pipe, transcriber=tr)
        ready = json.loads(self.call("GET", "/health/ready")[1])
        self.assertEqual(ready["extensions"], ["instrumental", "covers"])
        body = form({"style": "solo cello", "lyrics": "[Intro]", "seed": "11", "instrumental": "true"})
        code, resp, headers = self.call("POST", "/v1/covers", body, headers={"Idempotency-Key": "c1"})
        self.assertEqual(code, 202, resp)
        job = self.wait(json.loads(resp)["id"])
        self.assertEqual(job["status"], "succeeded", job)
        self.assertEqual(tr.calls, [b"ID3fake"])
        (kind, song), = pipe.calls  # no planning: the score comes from the recording
        self.assertEqual(kind, "song")
        self.assertEqual(song["abc"], "CONVERTED TRANSCRIBED-MELODY")
        self.assertEqual(song["seed"], 11)
        res = job["result"]
        self.assertEqual(res["instrumental"]["score_origin"], "SheetSage2")
        self.assertEqual((res["cover"]["transcriber"], res["cover"]["device"], res["cover"]["warnings"]),
                         ("SheetSage2", "fake-device", ["short input"]))
        self.assertEqual(res["cover"]["score_seconds"], 32.0)
        # The same key and the same recording find the same job.
        code, again, _ = self.call("POST", "/v1/covers", body, headers={"Idempotency-Key": "c1"})
        self.assertEqual((code, json.loads(again)["id"]), (200, job["id"]))
        other = form({"style": "solo cello", "lyrics": "[Intro]", "seed": "11", "instrumental": "true"},
                     audio=b"ID3other")
        code, _, _ = self.call("POST", "/v1/covers", other, headers={"Idempotency-Key": "c1"})
        self.assertEqual(code, 409)
        self.assertEqual(len(list((jobs.root / "covers").glob("*.mp3"))), 2)

    def test_sung_cover_uses_the_melody_as_turbo_does(self):
        pipe = FakePipe()
        self.start(pipe, transcriber=FakeTranscriber())
        code, resp, _ = self.call("POST", "/v1/covers", form({"style": "piano pop", "lyrics": "[Verse]\nla la"}))
        self.assertEqual(code, 202, resp)
        job = self.wait(json.loads(resp)["id"])
        self.assertEqual(job["status"], "succeeded", job)
        self.assertNotIn("instrumental", job["result"])
        (_, song), = pipe.calls
        self.assertEqual((song["cot"], song["abc"], song["seed"]), ("melody", "TRANSCRIBED-MELODY", 831001))

    def test_cover_validation(self):
        self.start(FakePipe(), transcriber=FakeTranscriber())
        for body in (form({"style": "x", "lyrics": "y"}, audio=None), form({"style": "x", "lyrics": "y"}, audio=b""),
                     form({"style": "x"}), form({"style": "x", "lyrics": "y", "bogus": "1"}),
                     form({"style": "x", "lyrics": "y", "seed": "seven"}),
                     form({"style": "x", "lyrics": "y", "instrumental": "yes"}),
                     form({"style": "x", "lyrics": "y", "seconds": "30"})):
            code, resp, _ = self.call("POST", "/v1/covers", body)
            self.assertEqual(code, 422, resp)
        code, _, _ = self.call("POST", "/v1/covers", {"style": "x", "lyrics": "y"})
        self.assertEqual(code, 422)
        code, resp, _ = self.call("POST", "/v1/covers", (b"x", "multipart/form-data; boundary=b"),
                                  headers={"Content-Length": str(srv.COVER_LIMIT + 1)})
        self.assertEqual(code, 413, resp)

    def test_covers_off_answers_503_without_retry(self):
        self.start(FakePipe())
        self.assertEqual(json.loads(self.call("GET", "/health/ready")[1])["extensions"], ["instrumental"])
        code, resp, headers = self.call("POST", "/v1/covers", form({"style": "x", "lyrics": "y"}))
        self.assertEqual(code, 503)
        self.assertIsNone(headers.get("Retry-After"))
        self.assertIn("covers are off", json.loads(resp)["detail"])

    def test_failed_transcription_has_its_own_code(self):
        self.start(FakePipe(), transcriber=FakeTranscriber(fail=True))
        code, resp, _ = self.call("POST", "/v1/covers", form({"style": "x", "lyrics": "y", "instrumental": "true"}))
        job = self.wait(json.loads(resp)["id"])
        self.assertEqual((job["status"], job["error"]["code"]), ("failed", "transcription_failed"))
        self.assertIn("no melody", job["error"]["message"])

    def test_transcriber_runs_the_helper_script(self):
        # A stand-in for the helper's transcribe_cover.py: it records its arguments and writes a score.
        tmp = Path(tempfile.mkdtemp())
        self.addCleanup(lambda: __import__("shutil").rmtree(tmp, ignore_errors=True))
        script = tmp / "transcribe_cover.py"
        script.write_text("import json, sys, pathlib\n"
                          "out = pathlib.Path(sys.argv[sys.argv.index('--output') + 1]); out.mkdir(parents=True)\n"
                          "(out / 'args.json').write_text(json.dumps(sys.argv[1:]))\n"
                          "if 'fail' in sys.argv[1]:\n"
                          "    (out / 'failure.json').write_text(json.dumps({'error': 'bad audio'})); sys.exit(2)\n"
                          "(out / 'score.abc').write_text('X:1')\n"
                          "(out / 'transcription_manifest.json').write_text(json.dumps({'warnings': ['w']}))\n")
        import sys as _sys
        tr = srv.Transcriber(_sys.executable, script, "models/SheetSage2", base_model="models/MERT", device="mps")
        abc, warnings = tr(tmp / "song.mp3", tmp / "t1", lambda: False)
        self.assertEqual((abc, warnings), ("X:1", ["w"]))
        args = json.loads((tmp / "t1/args.json").read_text())
        self.assertEqual(args[:5], [str(tmp / "song.mp3"), "--output", str(tmp / "t1"), "--task", "melody-full"])
        for flag, value in (("--device", "mps"), ("--dtype", "fp32"), ("--base-model", "models/MERT"), ("--model", "models/SheetSage2")):
            self.assertEqual(args[args.index(flag) + 1], value)
        with self.assertRaisesRegex(srv.TranscriptionFailed, "bad audio"):
            tr(tmp / "fail.mp3", tmp / "t2", lambda: False)

    def test_style_rewrite_matches_the_helper(self):
        self.assertEqual(srv.instrumental_style("Instrumental, piano, no vocals."),
                         "Instrumental, piano, no vocals, no singing, no choir, no spoken words.")


@unittest.skipUnless(os.environ.get("YUE2_ROOT"), "YUE2_ROOT is not set")
class RealHelperTest(unittest.TestCase):
    def test_converts_the_helper_example(self):
        helper = srv.helper_modules(os.environ["YUE2_ROOT"])
        source = Path(os.environ["YUE2_ROOT"], "examples/score.abc").read_text()
        converted, transfer = helper.convert_score(source, overlap="vocal", keep_chords=True)
        score = helper.validate_score(converted)
        self.assertFalse(score.voices["Vocal"].notes)
        self.assertGreater(transfer["vocal_notes_before"], 0)
        self.assertGreater(helper.report(score)["nominal_duration_seconds"], 0)

    def test_fit_score_cuts_at_a_phrase_and_keeps_the_outro(self):
        helper = srv.helper_modules(os.environ["YUE2_ROOT"])
        source = Path(os.environ["YUE2_ROOT"], "examples/score.abc").read_text()
        converted, _ = helper.convert_score(source, overlap="vocal", keep_chords=True)
        full = helper.report(helper.validate_score(converted))["nominal_duration_seconds"]
        fitted, got = srv.fit_score(helper, converted, full / 2)
        score = helper.validate_score(fitted)
        self.assertAlmostEqual(helper.report(score)["nominal_duration_seconds"], got)
        self.assertLess(got, full)
        self.assertLess(abs(got - full / 2), 12)
        labels = [l for l in fitted.splitlines() if l.startswith("% ")]
        original = [l for l in converted.splitlines() if l.startswith("% ")]
        if original[-1] == "% outro":
            self.assertEqual(labels[-1], "% outro")
        same, got_full = srv.fit_score(helper, converted, 600)
        self.assertEqual((same, got_full), (converted, full))


if __name__ == "__main__":
    unittest.main()
