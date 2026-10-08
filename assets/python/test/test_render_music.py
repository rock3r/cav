"""Tests for render_music.py, the renderer behind `cav music render`.

Run from the repository root:

  uv run --no-project --python 3.12 --with numpy --with pedalboard --with pyloudnorm \
    --with mido --with matchering python -m unittest discover -s assets/python/test

The plugin tests run only when CAV_TEST_VST3 names a VST3 instrument (for example Dexed.vst3).
"""
import json
import math
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

import numpy as np
import pedalboard as pb
import pyloudnorm

HERE = Path(__file__).resolve().parent
SCRIPT = HERE.parent / "render_music.py"
sys.path.insert(0, str(SCRIPT.parent))
import render_music as rm  # noqa: E402

SR = rm.SR


def write_wav(path, x, sr):
    with pb.io.AudioFile(str(path), "w", sr, x.shape[0]) as f:
        f.write(x.astype(np.float32))


def read_wav(path):
    with pb.io.AudioFile(str(path)) as f:
        return f.read(f.frames), f.samplerate


def render(score, td, stems=False):
    score_path, out = Path(td) / "score.json", Path(td) / "out.wav"
    score_path.write_text(json.dumps(score))
    stem_dir = ""
    if stems:
        stem_dir = str(Path(td) / "stems")
        os.makedirs(stem_dir)
    res = subprocess.run([sys.executable, str(SCRIPT), str(score_path), str(out), stem_dir],
                         capture_output=True, text=True, check=False)
    if res.returncode:
        raise AssertionError(res.stderr)
    return json.loads(res.stdout), out


def onsets(x, sr, threshold=0.1):
    # Sample indexes where the level rises over threshold * max after 50 ms or more under it.
    a = np.abs(x).max(axis=0) if x.ndim == 2 else np.abs(x)
    over = np.flatnonzero(a > threshold * a.max())
    return [int(i) for k, i in enumerate(over) if k == 0 or i - over[k - 1] > 0.05 * sr]


def one_shot(sr):
    # A short stereo click-and-thump, like a kick sample.
    t = np.arange(int(0.25 * sr)) / sr
    x = np.sin(2 * math.pi * 60 * t) * np.exp(-t * 20)
    x[: int(0.002 * sr)] += 0.5
    return np.stack([x, 0.8 * x])


class TruePeak(unittest.TestCase):
    def test_finds_the_peak_between_samples(self):
        # A sine at a quarter of the sample rate, sampled 45 degrees off its crests: every
        # sample is at 0.707, and the true peak is 1.0 (+3 dB).
        n = np.arange(4800)
        x = np.sin(2 * math.pi * n / 4 + math.pi / 4)[None, :]
        self.assertAlmostEqual(float(np.abs(x).max()), math.sqrt(0.5), places=3)
        self.assertAlmostEqual(rm.true_peak_db(x[:, 100:-100]), 0.0, delta=0.1)

    def test_limiter_keeps_the_true_peak_under_the_ceiling(self):
        # White noise peaks between samples, like the synthesised hat.
        x = np.random.default_rng(1).normal(0, 0.5, (2, SR)).astype(np.float32)
        y = rm.limit(x, ceiling_db=-1.2)
        self.assertLessEqual(rm.true_peak_db(y), -1.1)


class Render(unittest.TestCase):
    def test_built_in_mix_meets_the_loudness_and_true_peak_target(self):
        with tempfile.TemporaryDirectory() as td:
            score = {"bpm": 120, "beats": 16, "tracks": [
                {"name": "kick", "instrument": "kick", "hits": list(range(16))},
                {"name": "hat", "instrument": "hat", "hits": [b + 0.5 for b in range(16)]},
                {"name": "bass", "instrument": "bass", "notes": [[0, 2, 40, 0.8], [8, 2, 43, 0.8]], "duck": True}],
                "master": {"lufs": -14}}
            res, out = render(score, td)
            x, sr = read_wav(out)
            self.assertEqual(sr, SR)
            self.assertAlmostEqual(res["lufs"], -14, delta=0.3)
            self.assertLessEqual(res["peak"], -1.0)
            self.assertLessEqual(rm.true_peak_db(x), -1.0)

    def test_sample_track_plays_each_hit_on_its_beat(self):
        with tempfile.TemporaryDirectory() as td:
            # A 44.1 kHz stereo file, so the renderer has to resample and fold it to mono.
            write_wav(Path(td) / "kick.wav", one_shot(44100), 44100)
            hits = [0, 1, 2.5, 4, 6]
            score = {"bpm": 120, "beats": 8, "humanize": {"timing": 0, "velocity": 0},
                     "tracks": [{"name": "kick", "instrument": f"sample:{td}/kick.wav", "hits": hits}]}
            res, _ = render(score, td, stems=True)
            stem, sr = read_wav(res["stems"][0])
            self.assertEqual(sr, SR)
            got = [i / SR for i in onsets(stem, SR)]
            self.assertEqual(len(got), len(hits))
            for want, at in zip(hits, got):
                self.assertAlmostEqual(at, want * 0.5, delta=0.002)
            # A lone kick is too peaky for -14 LUFS under -1 dBTP: the renderer says so.
            self.assertLessEqual(res["peak"], -1.0)
            self.assertLess(res["lufs"], -14.3)
            self.assertIn("too sparse", res["warning"])

    def test_sample_track_in_a_full_mix_meets_the_target(self):
        with tempfile.TemporaryDirectory() as td:
            write_wav(Path(td) / "kick.wav", one_shot(44100), 44100)
            score = {"bpm": 120, "beats": 16, "tracks": [
                {"name": "kick", "instrument": f"sample:{td}/kick.wav", "hits": list(range(16))},
                {"name": "pad", "instrument": "pad", "notes": [[0, 16, 52, 0.6]], "gain": -4}]}
            res, _ = render(score, td)
            self.assertAlmostEqual(res["lufs"], -14, delta=0.3)
            self.assertLessEqual(res["peak"], -1.0)
            self.assertEqual(res["warning"], "")

    def test_reference_master_still_meets_the_target(self):
        with tempfile.TemporaryDirectory() as td:
            # A loud, bright 44.1 kHz reference: Matchering alone would leave the mix near
            # its loudness and sample rate.
            sr = 44100
            t = np.arange(20 * sr) / sr
            rng = np.random.default_rng(3)
            ref = 0.5 * np.sin(2 * math.pi * 110 * t) * (0.6 + 0.4 * np.sin(2 * math.pi * 2 * t))
            ref = ref + 0.2 * rng.normal(0, 1, len(t))
            ref = np.clip(ref * 2.5, -0.99, 0.99)
            write_wav(Path(td) / "ref.wav", np.stack([ref, ref]), sr)
            self.assertGreater(pyloudnorm.Meter(sr).integrated_loudness(np.stack([ref, ref]).T), -9)
            score = {"bpm": 120, "beats": 32, "tracks": [
                {"name": "kick", "instrument": "kick", "hits": list(range(32))},
                {"name": "pad", "instrument": "pad", "notes": [[0, 16, 52, 0.5], [16, 16, 55, 0.5]], "gain": -6}],
                "master": {"lufs": -14, "reference": f"{td}/ref.wav"}}
            res, out = render(score, td)
            x, sr_out = read_wav(out)
            self.assertEqual(sr_out, SR)
            self.assertEqual(x.shape[1], 16 * SR)
            self.assertAlmostEqual(res["lufs"], -14, delta=0.3)
            self.assertLessEqual(res["peak"], -1.0)
            self.assertLessEqual(rm.true_peak_db(x), -1.0)


@unittest.skipUnless(os.environ.get("CAV_TEST_VST3"), "set CAV_TEST_VST3 to a VST3 instrument")
class PluginInstrument(unittest.TestCase):
    notes = [[0.5, 1, 60, 0.8], [2, 0.5, 64, 0.8], [3, 2, 67, 0.8], [6, 0.25, 72, 0.8]]

    def score(self, **track):
        return {"bpm": 120, "beats": 8, "humanize": {"timing": 0, "velocity": 0},
                "tracks": [dict({"name": "synth", "instrument": "vst3:" + os.environ["CAV_TEST_VST3"],
                                 "notes": self.notes}, **track)]}

    def stem(self, td, **track):
        res, _ = render(self.score(**track), td, stems=True)
        return read_wav(res["stems"][0])[0]

    def test_notes_start_on_time_and_stop_at_their_length(self):
        with tempfile.TemporaryDirectory() as td:
            x = np.abs(self.stem(td)).max(axis=0)
            env = np.sqrt(np.convolve(x ** 2, np.ones(96) / 96, "same"))  # 2 ms RMS
            floor = env.max() * 10 ** (-60 / 20)
            for beat, length, _, _ in self.notes:
                start, end = int(beat * 0.5 * SR), int((beat + length) * 0.5 * SR)
                rise = start + int(np.argmax(env[start - 2400 :] > env.max() * 0.02)) - 2400
                self.assertLess(abs(rise - start) / SR, 0.010, f"note at beat {beat} starts at {rise / SR:.4f}s")
                self.assertGreater(env[end - int(0.01 * SR)], floor, f"note at beat {beat} stops early")
                # The release ends within 250 ms of the note-off (the synth's own tail).
                self.assertLess(env[end + int(0.25 * SR)], floor, f"note at beat {beat} rings on")

    def test_params_and_presets_change_the_sound(self):
        plugin = pb.load_plugin(os.environ["CAV_TEST_VST3"])
        name = next(k for k, p in plugin.parameters.items() if p.max_value - p.min_value > 0 and not p.is_boolean)
        param = plugin.parameters[name]
        value = param.min_value + 0.3 * (param.max_value - param.min_value)
        with tempfile.TemporaryDirectory() as td:
            plain = self.stem(td)
        with tempfile.TemporaryDirectory() as td:
            changed = self.stem(td, params={name: value})
        self.assertFalse(np.allclose(plain, changed, atol=1e-4), f"setting {name} changed nothing")
        setattr(plugin, name, value)
        with tempfile.TemporaryDirectory() as td:
            Path(td, "p.vstpreset").write_bytes(plugin.preset_data)
            Path(td, "p.state").write_bytes(plugin.raw_state)
            for preset in ("p.vstpreset", "p.state"):
                with tempfile.TemporaryDirectory() as out:
                    got = self.stem(out, preset=f"{td}/{preset}")
                np.testing.assert_allclose(got, changed, atol=1e-4, err_msg=preset)

    def test_unknown_param_names_the_known_ones(self):
        with tempfile.TemporaryDirectory() as td:
            with self.assertRaises(AssertionError) as e:
                render(self.score(params={"no_such_knob": 1}), td)
            self.assertIn("no parameter 'no_such_knob'", str(e.exception))


if __name__ == "__main__":
    unittest.main()
