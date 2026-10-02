# Syncing motion to music

## 1. Get the grid

With a track:

```bash
cav beats music.wav --fps 60 -o build/grid.json
```

It prints the BPM, the frame of every beat, and the downbeats (beat 1 of each 4/4 bar). Steady
tracks are snapped to an exact grid. If the BPM looks like half or double the tempo you hear
described, run it again with `--bpm <expected>`.

Without a track (you or the user choose the tempo):

```bash
cav beats --bpm 120 --seconds 16 --fps 60
```

Useful numbers at 60 fps: 120 BPM = 30 frames per beat, 128 BPM = 28.1, 100 BPM = 36,
90 BPM = 40. A 4/4 bar is 4 beats.

## 2. Use it in scripts

```js
var b = cav.beats(120, { offset: 0 })  // offset = frame of the first beat (from cav beats)
b.beat(0)    // frame of the first beat
b.bar(2)     // frame of the first beat of the third bar
b.frames     // frames per beat (can be fractional; helpers round)
```

Or read the file: `var g = JSON.parse(api.readFromFile('/abs/path/build/grid.json'))`, then
`g.beatFrames[n]` and `g.downbeatFrames[n]`.

## 3. Map the music to the picture

| Musical moment | Picture |
|---|---|
| Downbeat of a new section (every 4 or 8 bars) | Cut or transition, new scene, flash |
| Downbeat inside a section | Main element arrives or changes |
| Beats 2, 3, 4 | Small accents: `cav.punch`, a colour change, one word |
| Off-beats (half way) | Secondary elements, stagger steps |
| Break or pause before a drop | Hold, shrink everything to a point, dim |
| Drop / big hit | Explode out: `cav.flash`, `cav.ring`, `cav.burst`, `cav.shake` on the rig |

- A move that "hits" the beat must **end** on the beat frame. Start it earlier: a 12-frame
  `outBack` pop that should land on frame 120 starts at 108. Flashes and cuts start exactly on the beat.
- Build every section from the grid, never from hand-typed frame numbers.
- Keep a steady visual rhythm: if you punch on beats 2 and 4 in bar 1, keep that pattern.

## 4. Check

```bash
cav sheet 108,114,120,126,132 --bpm 120   # tiles show b4.6 ... b5.4: hits should land on whole beats
cav render -o renders/final.mp4 --audio music.wav
```

You cannot hear the result. Tell the user which frames the hits are on, so they can check.
If you generate music yourself, say that no one has listened to it yet.
