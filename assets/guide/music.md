# Syncing motion to music

## 1. Get the grid

With a track:

```bash
cav beats music.wav --fps 60 -o build/grid.json
```

`cav beats` prints beats per minute (BPM), each beat's frame and each downbeat. A downbeat
is beat 1 of a 4/4 bar. The command fits steady tracks to an exact grid. If the result
is half or twice the expected tempo, run it again with `--bpm <expected>`.

Without a track, choose a tempo with the user:

```bash
cav beats --bpm 120 --seconds 16 --fps 60
```

At 60 fps, 120 BPM gives 30 frames per beat; 128 gives 28.1; 100 gives 36; and 90 gives
40. A 4/4 bar has four beats.

For an audio track, `cav beats` also reports these features:

- **sections** mark changes such as an intro, drop or break. Each entry gives the bar,
  loudness and change from the prior section. Plan one scene per section.
- **events** include `rise`, a beat at least 6 dB louder than the prior two beats;
  `fall`, a sudden decrease; and `silence`, a gap of at least 0.15 s.
- **accents** identify strong hits, their timing and their frequency band. `low` covers
  kick and bass, `mid` covers snare, stabs and voice, and `high` covers hats. Give strong
  low or mid accents between grid beats their own visual hit when appropriate.
- **barEnergy** gives each bar's loudness from 0 to 1 in the JSON.

## 2. Look at the track

Use `cav spectrogram` to inspect frequency and timing:

```bash
cav spectrogram music.wav --fps 60                  # renders/spectrogram.png
cav spectrogram music.wav --fps 60 --from 40 --to 54  # zoom into 14 seconds
```

Open the image. Its lanes share a time axis in seconds and frames:

| Lane | What it shows | How to read it |
|---|---|---|
| top | Spectrogram: low notes at the bottom, brighter = louder. White bar lines, numbered; yellow section lines. | A bright band starting at a bar line is a new instrument. A wide bright column is a hit. A dark gap is a break. A streak rising over a few bars is a riser: it lands where it ends. A streak bending down is a tape-stop or a pitch drop. |
| db | Loudness. Red shading = silence, green triangle = rise, red triangle = fall. | Plan big moves where the curve jumps, and holds where it dips. |
| low, mid, hi | Onsets per band. White dots = the strongest accents. | The low lane is the kick pattern, mid the snares and stabs, hi the hats. |

Describe visible events in musical terms, such as "a riser from bar 15 to bar 19" and
"a one-beat pause before bar 27". Tell the user that the description comes from the image.

## 3. Use it in scripts

```js
var b = cav.beats(120, { offset: 0 })  // offset = frame of the first beat (from cav beats)
b.beat(0)    // frame of the first beat
b.bar(2)     // frame of the first beat of the third bar
b.frames     // frames per beat (can be fractional; helpers round)
```

Scripts can also read the file with
`var g = JSON.parse(api.readFromFile('/abs/path/build/grid.json'))`. Then use
`g.beatFrames[n]` and `g.downbeatFrames[n]`.

## 4. Map the music to the picture

| Musical moment | Picture |
|---|---|
| Downbeat of a new section (every 4 or 8 bars) | Cut or transition, new scene, flash |
| Downbeat inside a section | Main element arrives or changes |
| Beats 2, 3, 4 | Small accents: `cav.punch`, a colour change, one word |
| Off-beats (half way) | Secondary elements, stagger steps |
| Break or pause before a drop | Hold, shrink everything to a point, dim |
| Drop / big hit | Explode out: `cav.flash`, `cav.ring`, `cav.burst`, `cav.shake` on the rig |
| `silence` event (a pause) | Freeze or cut to black for exactly its length; hit hard on the frame after |
| Riser in the spectrogram | Build with it (zoom, scale up, tighten), and release where it lands |
| Strong low/mid accent off the grid | A hit of its own: a punch or a cut on that frame |

- A move that matches a beat must **end** on that beat frame. Start a 12-frame `outBack`
  pop at frame 108 to land on frame 120. Start flashes and cuts on the beat.
- Use grid positions to time sections. The grid provides the frame numbers.
- Keep repeated accents consistent. For example, repeat punches on beats 2 and 4 in each bar.

## 5. Continuous movement with the music

For continuous movement that follows the music, connect Cavalry's Sound behaviour.
This can drive a bass-linked glow or moving bars without individual keys:

```js
// scale.y between 1 (quiet) and 1.4 (loudest), from the kick and bass only
cav.sound('/abs/path/music.wav', logo, 'scale.y', { min: 1, max: 1.4, low: 20, high: 150, smooth: 2 })
```

Place major hits on the grid. Use `cav.sound` for continuous movement between them.

## 6. Check

```bash
cav sheet 108,114,120,126,132 --bpm 120   # tiles show b4.6 ... b5.4: hits should land on whole beats
cav render -o renders/final.mp4 --audio music.wav
cav sync renders/final.mp4                # renders/sync.png and a report
```

`cav sync` finds sharp visual changes in the render, including cuts and the fastest frame
of a move. It measures each change against the beat grid in frames. It also lists strong
musical events without a visual hit. The picture adds a "pic" lane below the spectrogram.
Green marks are on the grid; red marks are off it. Check unintended red marks and decide
whether each unmatched musical event needs a visual hit. Long tracks can have a grid error
of one or two frames, so do not adjust every ±1-frame difference.

The CLI does not play audio for the agent. Give the user the hit frames for a listening
check. If the agent generated the music, state whether a person has listened to it.
