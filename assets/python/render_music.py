# Renders a written score to a mixed WAV for `cav music render`. Run by cav through uv.
#   args: <score.json> <out.wav> <stems-dir or "">
# The score (JSON):
#   {"bpm": 120, "beats": 32, "swing": 0.1, "humanize": {"timing": 0.008, "velocity": 0.1},
#    "tracks": [
#      {"name": "bass", "instrument": "bass", "notes": [[beat, length, midi, velocity], ...],
#       "gain": -4, "pan": 0, "duck": true, "reverb": 0.1, "delay": 0},
#      {"name": "kick", "instrument": "kick", "hits": [0, 1, 2, 3], "velocity": 0.9},
#      {"name": "fx", "instrument": "sample:/abs/whoosh.wav", "hits": [15.5]},
#      {"name": "lead", "instrument": "vst3:/Library/Audio/Plug-Ins/VST3/Surge XT.vst3", "notes": [...],
#       "preset": "/abs/lead.vstpreset", "params": {"cutoff": 0.4}}],
#    "master": {"lufs": -14, "reference": "/abs/reference.wav"}}
# Instruments: bass, sub, pad, pluck, lead, keys, bell (synthesised here); kick, snare, hat,
# openhat, clap (synthesised drums); sample:<path>; vst3:<path> or au:<path> (pedalboard).
# A plugin track can load a "preset": a .vstpreset (VST3), or a state file holding the bytes of
# pedalboard's plugin.raw_state (VST3 or AU). "params" then sets parameters by their pedalboard
# names (see plugin.parameters).
# Prints JSON: {"out", "seconds", "lufs", "peak" (true peak, dBTP), "stems": [...], "warning"}.
import json
import math
import sys

import numpy as np

SR = 48000
rng = np.random.default_rng(7)


def midi_hz(m):
    return 440.0 * 2 ** ((m - 69) / 12)


def adsr(n, a, d, s, r, sustain_len):
    a, d, r = int(a * SR), int(d * SR), int(r * SR)
    env = np.zeros(n, dtype=np.float32)
    held = max(1, int(sustain_len * SR))
    pts = []
    i = 0
    if a:
        seg = min(a, n)
        env[:seg] = np.linspace(0, 1, seg, endpoint=False)
        i = seg
    if i < n and d:
        seg = min(d, n - i)
        env[i : i + seg] = np.linspace(1, s, seg, endpoint=False)
        i += seg
    end = min(held, n)
    if i < end:
        env[i:end] = s
        i = end
    if i < n:
        start = env[i - 1] if i else s
        seg = min(r, n - i)
        env[i : i + seg] = np.linspace(start, 0, seg)
    return env


def saw(f, n, phase=0.0):
    t = (np.arange(n) / SR * f + phase) % 1.0
    return (2 * t - 1).astype(np.float32)


def onepole_lp(x, cutoff):
    # Simple per-sample low-pass with a cutoff curve (array or float) in Hz.
    cutoff = np.broadcast_to(np.asarray(cutoff, dtype=np.float32), x.shape)
    a = np.exp(-2 * math.pi * cutoff / SR)
    y = np.empty_like(x)
    acc = 0.0
    for i in range(len(x)):
        acc = (1 - a[i]) * x[i] + a[i] * acc
        y[i] = acc
    return y


def synth_note(inst, midi, dur, vel):
    f = midi_hz(midi)
    tail = {"pad": 1.2, "keys": 0.6, "bell": 1.5, "pluck": 0.4}.get(inst, 0.15)
    n = int((dur + tail) * SR)
    if inst in ("bass", "sub"):
        x = 0.6 * saw(f, n) + 0.8 * np.sin(2 * math.pi * f / 2 * np.arange(n) / SR)
        if inst == "sub":
            x = np.sin(2 * math.pi * f * np.arange(n) / SR)
        env = adsr(n, 0.005, 0.15, 0.7, 0.08, dur)
        cut = 180 + 900 * adsr(n, 0.001, 0.12, 0.25, 0.05, dur)
        x = onepole_lp(x, cut) * env
    elif inst == "pad":
        x = sum(saw(f * d, n, p) for d, p in ((1, 0), (1.006, 0.3), (0.994, 0.7), (2.003, 0.5))) / 3
        x = onepole_lp(x.astype(np.float32), 1800) * adsr(n, 0.4, 0.5, 0.8, 1.0, dur)
    elif inst == "pluck":
        x = saw(f, n) + 0.5 * saw(f * 2.001, n)
        x = onepole_lp(x, 300 + 5000 * adsr(n, 0.001, 0.18, 0.0, 0.05, dur)) * adsr(n, 0.002, 0.3, 0.0, 0.2, dur)
    elif inst == "lead":
        sq = np.sign(np.sin(2 * math.pi * f * np.arange(n) / SR)).astype(np.float32)
        vib = 1 + 0.004 * np.sin(2 * math.pi * 5.5 * np.arange(n) / SR)
        x = 0.5 * sq + 0.5 * saw(f, n) * vib
        x = onepole_lp(x, 2600) * adsr(n, 0.01, 0.2, 0.75, 0.12, dur)
    elif inst in ("keys", "bell"):
        t = np.arange(n) / SR
        ratio, index, decay = (1, 1.2, 1.8) if inst == "keys" else (3.5, 2.5, 0.9)
        mod_env = np.exp(-t * (4 if inst == "keys" else 2))
        x = np.sin(2 * math.pi * f * t + index * mod_env * np.sin(2 * math.pi * f * ratio * t))
        x = (x * np.exp(-t * decay) * adsr(n, 0.003, 0.1, 1.0, 0.3, dur)).astype(np.float32)
    else:
        raise ValueError(f"unknown instrument {inst}")
    return (x * vel).astype(np.float32)


def drum(kind, vel):
    t = np.arange(int(0.6 * SR)) / SR
    if kind == "kick":
        f = 45 + 110 * np.exp(-t * 28)
        x = np.sin(2 * math.pi * np.cumsum(f) / SR) * np.exp(-t * 7)
        x[: int(0.004 * SR)] += rng.normal(0, 0.3, int(0.004 * SR))
    elif kind == "snare":
        noise = rng.normal(0, 1, len(t))
        x = 0.7 * onepole_lp(noise.astype(np.float32), 6000) * np.exp(-t * 18) + 0.5 * np.sin(2 * math.pi * 185 * t) * np.exp(-t * 25)
    elif kind in ("hat", "openhat"):
        noise = rng.normal(0, 1, len(t)).astype(np.float32)
        x = noise - onepole_lp(noise, 7000)
        x = x * np.exp(-t * (60 if kind == "hat" else 9))
    elif kind == "clap":
        noise = rng.normal(0, 1, len(t)).astype(np.float32)
        env = sum(np.exp(-np.clip(t - d, 0, None) * 120) * (t >= d) for d in (0, 0.011, 0.022)) + 0.6 * np.exp(-np.clip(t - 0.03, 0, None) * 14) * (t >= 0.03)
        x = (noise - onepole_lp(noise, 900)) * env
    else:
        raise ValueError(f"unknown drum {kind}")
    return (x * vel / max(1e-6, np.max(np.abs(x)))).astype(np.float32)


def load_sample(path):
    import pedalboard.io

    with pedalboard.io.AudioFile(path).resampled_to(SR) as f:
        a = f.read(f.frames)
    return a.mean(axis=0).astype(np.float32)


def place(buf, sig, start):
    if start >= len(buf):
        return
    end = min(len(buf), start + len(sig))
    buf[start:end] += sig[: end - start]


def true_peak_env(x, over=4, half=16):
    # Per-sample true-peak level of a (channels, n) signal: the largest absolute value of the
    # signal and of its windowed-sinc interpolation at over-1 points between each pair of
    # samples (4x oversampling, as in ITU-R BS.1770). White-noise drums such as the hat can
    # peak 2 dB or more between samples. That content sits near the Nyquist frequency, where
    # the 12-tap filter of BS.1770 misses peaks, so this one has 32 taps per phase: it reads
    # the same or higher than ffmpeg's ebur128 meter, which cav listen uses.
    x = np.asarray(x, dtype=np.float32)
    n = x.shape[1]
    env = np.max(np.abs(x), axis=0)
    pad = np.pad(x, ((0, 0), (half, half)))
    taps = np.arange(-half + 1, half + 1)
    for p in range(1, over):
        d = p / over - taps  # distance from the interpolated point to each tap
        h = np.sinc(d) * (0.5 + 0.5 * np.cos(math.pi * d / (half + 1)))
        y = np.zeros_like(x)
        for k, c in zip(taps, h):
            y += c * pad[:, half + k : half + k + n]
        env = np.maximum(env, np.max(np.abs(y), axis=0))
    return env


def true_peak_db(x):
    return float(20 * np.log10(np.max(true_peak_env(x)) + 1e-9))


def limit(x, ceiling_db=-1.2, lookahead_ms=5, release_ms=80, env=None):
    # Brick-wall limiter on a (channels, n) mix: the gain falls ahead of each peak, over the
    # lookahead window, and recovers over the release. No makeup gain. It detects true
    # (inter-sample) peaks, and the ceiling sits a little under -1 dBTP for the measurement
    # error of other meters. (The JUCE Limiter in pedalboard adds makeup gain, so it
    # overshoots a loudness target.) env is true_peak_env(x), when the caller has it.
    ceil = 10 ** (ceiling_db / 20)
    need = np.minimum(1.0, ceil / np.maximum(true_peak_env(x) if env is None else env, 1e-9))
    la = max(1, int(lookahead_ms * SR / 1000))
    padded = np.pad(need, (0, la), constant_values=1.0)
    ahead = np.lib.stride_tricks.sliding_window_view(padded, la + 1).min(axis=1)
    # Averaging the last la+1 values of `ahead` stays at or under `need` and ramps smoothly.
    # The values before the start repeat the first one, so a hit at 0 s is limited too.
    c = np.cumsum(np.pad(ahead, (la + 1, 0), mode="edge"))
    ramp = (c[la + 1 :] - c[: -la - 1]) / (la + 1)
    rel = math.exp(-1 / (release_ms * SR / 1000))
    g = np.empty_like(ramp)
    cur = 1.0
    for i, want in enumerate(ramp):
        cur = want if want < cur else want - (want - cur) * rel
        g[i] = cur
    return (x * g).astype(np.float32)


def master_to(pre, target, meter):
    # Gain to the target and limit the peaks. Limiting lowers the loudness, more so the harder
    # it works (a sparse kick), so the gain is searched: secant steps on (gain, loudness).
    # Returns the mix and a warning when the target is out of reach under the ceiling.
    env = true_peak_env(pre)  # scales with the gain, so it is measured once
    start = meter.integrated_loudness(pre.T)
    if not np.isfinite(start):
        return limit(pre, env=env), ""
    gain_db, last = target - start, None
    for _ in range(8):
        g = 10 ** (gain_db / 20)
        mix = limit(pre * g, env=env * g)
        lufs = meter.integrated_loudness(mix.T)
        if abs(target - lufs) < 0.1:
            break
        slope = 1.0
        if last and last[0] != gain_db:
            slope = (lufs - last[1]) / (gain_db - last[0])
            if lufs < target and slope < 0.15:
                # More gain only crushes the peaks: the mix is too sparse for the target.
                return mix, (f"the mix reaches {lufs:.1f} LUFS, not {target:g}, under a -1 dBTP peak: "
                             "it is too sparse or peaky; fill it out or set master.lufs lower")
            slope = float(np.clip(slope, 0.15, 1.0))
        last = (gain_db, lufs)
        gain_db += min(12.0, (target - lufs) / slope)
    return mix, ""


def match_reference(mix, reference, work_dir):
    # Matchering matches the mix's frequency balance and loudness to the reference. It works
    # at 44.1 kHz and sets the reference's loudness, so the result comes back unlimited, in
    # float, and is resampled; master_to then sets the target loudness and peak again.
    import matchering as mg
    import pedalboard as pb

    src, dst = f"{work_dir}/premaster.wav", f"{work_dir}/matched.wav"
    with pb.io.AudioFile(src, "w", SR, 2, bit_depth=32) as f:
        f.write(mix)
    mg.process(target=src, reference=reference, results=[mg.Result(dst, subtype="FLOAT", use_limiter=False, normalize=False)])
    with pb.io.AudioFile(dst).resampled_to(SR) as f:
        out = f.read(f.frames)
    n = mix.shape[1]
    return np.pad(out, ((0, 0), (0, max(0, n - out.shape[1]))))[:, :n].astype(np.float32)


def load_instrument(pb, path, tr):
    plugin = pb.load_plugin(path)
    preset = tr.get("preset")
    if preset:
        if preset.lower().endswith(".vstpreset"):
            plugin.load_preset(preset)
        else:
            plugin.raw_state = open(preset, "rb").read()
    for name, value in (tr.get("params") or {}).items():
        if name not in plugin.parameters:
            raise ValueError(f"track {tr['name']}: the plugin has no parameter {name!r}; it has: {', '.join(plugin.parameters)}")
        setattr(plugin, name, value)
    return plugin


def main():
    score_path, out_path, stems_dir = sys.argv[1], sys.argv[2], sys.argv[3]
    score = json.load(open(score_path))
    import pedalboard as pb

    bpm = float(score["bpm"])
    spb = 60.0 / bpm
    beats = float(score.get("beats") or 0)
    seconds = float(score.get("seconds") or beats * spb)
    total = int((seconds + 2.0) * SR)
    swing = float(score.get("swing", 0))
    hum = score.get("humanize", {})
    jitter, vjit = float(hum.get("timing", 0.006)), float(hum.get("velocity", 0.08))

    def at(beat):
        b = float(beat)
        if swing and (b * 2) % 2 == 1:  # off-beat eighths move later
            b += swing * 0.5
        return max(0, int((b * spb + rng.normal(0, jitter)) * SR))

    def v(x):
        return float(np.clip(x * (1 + rng.normal(0, vjit)), 0.05, 1.0))

    stems, kick_env = {}, None
    for tr in score["tracks"]:
        inst = tr["instrument"]
        buf = np.zeros(total, dtype=np.float32)
        if inst.startswith("vst3:") or inst.startswith("au:"):
            plugin = load_instrument(pb, inst.split(":", 1)[1], tr)
            import mido

            msgs = []
            for beat, length, midi, vel in tr["notes"]:
                s = at(beat) / SR
                msgs.append(mido.Message("note_on", note=int(midi), velocity=int(127 * v(vel)), time=s))
                msgs.append(mido.Message("note_off", note=int(midi), velocity=0, time=s + length * spb))
            rendered = plugin(msgs, duration=total / SR, sample_rate=SR, num_channels=2)
            buf += rendered.mean(axis=0).astype(np.float32)[:total]
        elif inst.startswith("sample:"):
            smp = load_sample(inst.split(":", 1)[1])
            for h in tr.get("hits", []):
                place(buf, smp * v(tr.get("velocity", 0.9)), at(h))
        elif inst in ("kick", "snare", "hat", "openhat", "clap"):
            for h in tr.get("hits", []):
                place(buf, drum(inst, v(tr.get("velocity", 0.9))), at(h))
        else:
            for beat, length, midi, vel in tr.get("notes", []):
                place(buf, synth_note(inst, midi, length * spb, v(vel)), at(beat))
        if inst == "kick":
            k = np.abs(buf)
            kick_env = onepole_lp(k, 12)
            kick_env = kick_env / max(1e-6, kick_env.max())
        stems[tr["name"]] = (tr, buf)

    mix = np.zeros((2, total), dtype=np.float32)
    stem_paths = []
    for name, (tr, buf) in stems.items():
        if tr.get("duck") and kick_env is not None:
            buf = buf * (1 - 0.6 * kick_env)  # sidechain-style ducking under the kick
        chain = []
        if tr.get("instrument") in ("pad", "keys", "bell", "lead", "pluck"):
            chain.append(pb.HighpassFilter(cutoff_frequency_hz=120))
        if tr.get("delay"):
            chain.append(pb.Delay(delay_seconds=spb * 0.75, feedback=0.3, mix=float(tr["delay"])))
        if tr.get("reverb"):
            chain.append(pb.Reverb(room_size=0.7, wet_level=float(tr["reverb"]), dry_level=1 - 0.5 * float(tr["reverb"])))
        chain.append(pb.Gain(gain_db=float(tr.get("gain", 0))))
        stereo = np.stack([buf, buf])
        stereo = pb.Pedalboard(chain)(stereo, SR)
        pan = float(tr.get("pan", 0))
        stereo[0] *= math.cos((pan + 1) * math.pi / 4) * math.sqrt(2)
        stereo[1] *= math.sin((pan + 1) * math.pi / 4) * math.sqrt(2)
        mix += stereo
        if stems_dir:
            p = f"{stems_dir}/{name}.wav"
            with pb.io.AudioFile(p, "w", SR, 2) as f:
                f.write(stereo[:, : int(seconds * SR)])
            stem_paths.append(p)

    master = score.get("master", {})
    mix = pb.Pedalboard([pb.Compressor(threshold_db=-14, ratio=2, attack_ms=20, release_ms=150)])(mix, SR)
    mix = mix[:, : int(seconds * SR)]
    import pyloudnorm

    meter = pyloudnorm.Meter(SR)
    target = float(master.get("lufs", -14))
    mix, warning = master_to(mix, target, meter)
    if master.get("reference"):
        import tempfile

        with tempfile.TemporaryDirectory() as td:
            mix, warning = master_to(match_reference(mix, master["reference"], td), target, meter)
    with pb.io.AudioFile(out_path, "w", SR, 2, bit_depth=24) as f:
        f.write(mix)
    json.dump({"out": out_path, "seconds": seconds, "lufs": round(float(meter.integrated_loudness(mix.T)), 2),
               "peak": round(true_peak_db(mix), 2), "stems": stem_paths, "warning": warning}, sys.stdout)


if __name__ == "__main__":
    main()
