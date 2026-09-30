"""Make eval fixtures: two music tracks with known tempo, a CSV and an app screenshot.
Run: uv run --no-project --with numpy --with scipy --with pillow python make_fixtures.py"""
import csv
import os
from pathlib import Path

import numpy as np
from scipy.io import wavfile
from scipy.signal import butter, lfilter
from PIL import Image, ImageDraw, ImageFont

from evalpaths import FIXTURES

OUT = Path(os.environ.get("FIXTURES_OUT", FIXTURES))
OUT.mkdir(parents=True, exist_ok=True)
SR = 44100
rng = np.random.default_rng(7)


def env(n, attack, decay):
    t = np.arange(n) / SR
    a = np.clip(t / max(attack, 1e-4), 0, 1)
    return a * np.exp(-t / decay)


def kick(n):
    t = np.arange(n) / SR
    f = 45 + 110 * np.exp(-t * 30)
    return np.sin(2 * np.pi * np.cumsum(f) / SR) * env(n, 0.001, 0.18)


def snare(n):
    noise = rng.standard_normal(n)
    b, a = butter(2, [1200 / (SR / 2), 8000 / (SR / 2)], "band")
    t = np.arange(n) / SR
    return 0.6 * lfilter(b, a, noise) * env(n, 0.001, 0.09) + 0.3 * np.sin(2 * np.pi * 190 * t) * env(n, 0.001, 0.05)


def hat(n):
    b, a = butter(2, 7000 / (SR / 2), "high")
    return 0.25 * lfilter(b, a, rng.standard_normal(n)) * env(n, 0.001, 0.025)


def saw(freq, n):
    t = np.arange(n) / SR
    return 2 * ((t * freq) % 1) - 1


def lowpass(x, cutoff):
    b, a = butter(2, cutoff / (SR / 2))
    return lfilter(b, a, x)


def note(m):
    return 440 * 2 ** ((m - 69) / 12)


def track(bpm, bars, chords, name, drop_bar=None, break_before_drop=True, tail=1.5):
    beat = 60 / bpm
    total = int((bars * 4 * beat + tail) * SR)
    mix = np.zeros(total)
    side = np.ones(total)
    for b in range(bars):
        in_break = break_before_drop and drop_bar is not None and b == drop_bar - 1
        for q in range(4):
            t0 = int((b * 4 + q) * beat * SR)
            n = int(0.5 * SR)
            if in_break and q >= 2:
                continue  # half a bar of silence before the drop
            if not in_break or q < 2:
                k = kick(min(n, total - t0))
                mix[t0 : t0 + len(k)] += 0.9 * k
                # sidechain duck
                d = np.clip(np.arange(int(beat * SR)) / (0.25 * beat * SR), 0.35, 1)
                side[t0 : t0 + len(d)] = np.minimum(side[t0 : t0 + len(d)], d[: len(side[t0 : t0 + len(d)])])
            if q in (1, 3) and not in_break:
                s = snare(min(n, total - t0))
                mix[t0 : t0 + len(s)] += 0.5 * s
            for e in (0, 1):
                th = int((b * 4 + q + e * 0.5) * beat * SR)
                h = hat(min(int(0.1 * SR), total - th))
                mix[th : th + len(h)] += 0.8 * h if e else 0.4 * h
        # bass and chord stab per bar
        root = chords[b % len(chords)]
        t0 = int(b * 4 * beat * SR)
        n = int(4 * beat * SR)
        if not in_break:
            bass = lowpass(saw(note(root - 24), n), 300) * 0.35
            mix[t0 : t0 + n] += bass * side[t0 : t0 + n]
            for q in (0, 1.5, 2.5):
                ts = int((b * 4 + q) * beat * SR)
                m = int(0.35 * SR)
                stab = sum(lowpass(saw(note(root + iv), m), 2500) for iv in (0, 3, 7, 12)) * 0.08 * env(m, 0.005, 0.12)
                mix[ts : ts + m] += stab
    if drop_bar is not None:
        # impact on the drop downbeat
        t0 = int(drop_bar * 4 * beat * SR)
        n = int(1.2 * SR)
        mix[t0 : t0 + n] += 0.5 * lowpass(rng.standard_normal(n), 900) * env(n, 0.001, 0.4)
    # final hit
    t0 = int(bars * 4 * beat * SR)
    k = kick(min(int(tail * SR), total - t0))
    mix[t0 : t0 + len(k)] += k
    mix /= np.max(np.abs(mix)) * 1.1
    wavfile.write(OUT / name, SR, (mix * 32767).astype(np.int16))
    print(name, f"{bpm} BPM, {bars} bars, {len(mix) / SR:.2f} s")


track(128, 8, [57, 53, 48, 55], "loop128.wav", drop_bar=4)   # A min, F, C, G; drop at bar 5 (index 4)
track(100, 6, [50, 58, 53, 57], "promo100.wav", drop_bar=2)  # D min progression; drop at bar 3

with open(OUT / "revenue.csv", "w", newline="") as f:
    w = csv.writer(f)
    w.writerow(["quarter", "revenue_musd"])
    for q, v in [("Q1 2025", 4.2), ("Q2 2025", 5.1), ("Q3 2025", 4.8), ("Q4 2025", 6.9), ("Q1 2026", 8.3), ("Q2 2026", 9.6)]:
        w.writerow([q, v])
print("revenue.csv")

# A simple project-manager app screenshot, 1440 x 900.
W, H = 1440, 900
img = Image.new("RGB", (W, H), "#f4f5f7")
d = ImageDraw.Draw(img)
def font(size, bold=False):
    for p in ["/System/Library/Fonts/Supplemental/Arial Bold.ttf" if bold else "/System/Library/Fonts/Supplemental/Arial.ttf", "/Library/Fonts/Arial.ttf"]:
        try:
            return ImageFont.truetype(p, size)
        except OSError:
            pass
    return ImageFont.load_default()
d.rectangle([0, 0, 260, H], fill="#1f2433")
d.text((32, 34), "Lumen", font=font(30, True), fill="#ffffff")
for i, item in enumerate(["Home", "Projects", "Calendar", "Team", "Settings"]):
    y = 120 + i * 56
    if item == "Projects":
        d.rounded_rectangle([16, y - 12, 244, y + 32], 8, fill="#343b52")
    d.text((40, y), item, font=font(20), fill="#e8eaf0" if item == "Projects" else "#9aa3b8")
d.rectangle([260, 0, W, 84], fill="#ffffff")
d.text((300, 26), "Projects", font=font(30, True), fill="#1f2433")
d.rounded_rectangle([W - 250, 20, W - 40, 64], 10, fill="#5b5bf0")
d.text((W - 222, 31), "+ New project", font=font(20, True), fill="#ffffff")
cards = [("Website redesign", "12 tasks  ·  due Oct 14", "#ff8a65", 0.72), ("Mobile app v2", "31 tasks  ·  due Nov 2", "#4fc3f7", 0.45), ("Brand refresh", "8 tasks  ·  due Oct 30", "#81c784", 0.9), ("Q4 campaign", "19 tasks  ·  due Dec 5", "#ba68c8", 0.2)]
for i, (title, meta, col, prog) in enumerate(cards):
    x = 300 + (i % 2) * 540
    y = 130 + (i // 2) * 250
    d.rounded_rectangle([x, y, x + 500, y + 210], 16, fill="#ffffff", outline="#e3e6ec")
    d.rounded_rectangle([x + 28, y + 28, x + 76, y + 76], 12, fill=col)
    d.text((x + 96, y + 30), title, font=font(24, True), fill="#1f2433")
    d.text((x + 96, y + 64), meta, font=font(18), fill="#7a8399")
    d.rounded_rectangle([x + 28, y + 150, x + 472, y + 162], 6, fill="#eceff4")
    d.rounded_rectangle([x + 28, y + 150, x + 28 + int(444 * prog), y + 162], 6, fill=col)
    d.text((x + 28, y + 116), f"{int(prog * 100)}% complete", font=font(16), fill="#7a8399")
img.save(OUT / "app.png")
print("app.png")

# The held-out countdown track was added later and generated on its own, starting from a
# fresh generator. Re-seeding here keeps the file identical and leaves the tracks above as
# they were.
rng = np.random.default_rng(7)
track(90, 4, [52, 48, 55, 50], "count90.wav", drop_bar=3)    # E min, C, G, D; the GO lands on bar 4
