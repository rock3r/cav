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

With a track, `cav beats` also prints the shape of the music:

- **sections**: where the music changes (an intro, a drop, a break), with the bar, the
  loudness and the change from the section before. Plan one scene per section.
- **events**: `rise` (a beat at least 6 dB louder than the two before it: a drop or a hit),
  `fall` (a sudden drop-out) and `silence` (a gap of 0.15 s or more).
- **accents**: the strongest hits, each with its band (`low` kick and bass, `mid` snare, stabs
  and voice, `high` hats) and its place on the grid. Strong low or mid accents off the grid
  are syncopations that deserve a hit of their own.
- **barEnergy**: loudness per bar from 0 (quietest bar) to 1 (loudest), in the JSON.

## 2. Look at the track

You cannot hear the music, but you can look at it:

```bash
cav spectrogram music.wav --fps 60                  # renders/spectrogram.png
cav spectrogram music.wav --fps 60 --from 40 --to 54  # zoom into 14 seconds
```

Open the image. Everything shares one time axis in seconds and frames:

| Lane | What it shows | How to read it |
|---|---|---|
| top | Spectrogram: low notes at the bottom, brighter = louder. White bar lines, numbered; yellow section lines. | A bright band starting at a bar line is a new instrument. A wide bright column is a hit. A dark gap is a break. A streak rising over a few bars is a riser: it lands where it ends. A streak bending down is a tape-stop or a pitch drop. |
| db | Loudness. Red shading = silence, green triangle = rise, red triangle = fall. | Plan big moves where the curve jumps, and holds where it dips. |
| low, mid, hi | Onsets per band. White dots = the strongest accents. | The low lane is the kick pattern, mid the snares and stabs, hi the hats. |

Describe what you see to the user in musical words ("a riser from bar 15 that lands on bar
19, a one-beat pause before bar 27"), and say that it comes from the picture, not from
listening.

## 3. Use it in scripts

```js
var b = cav.beats(120, { offset: 0 })  // offset = frame of the first beat (from cav beats)
b.beat(0)    // frame of the first beat
b.bar(2)     // frame of the first beat of the third bar
b.frames     // frames per beat (can be fractional; helpers round)
```

Or read the file: `var g = JSON.parse(api.readFromFile('/abs/path/build/grid.json'))`, then
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

- A move that "hits" the beat must **end** on the beat frame. Start it earlier: a 12-frame
  `outBack` pop that should land on frame 120 starts at 108. Flashes and cuts start exactly on the beat.
- Build every section from the grid, never from hand-typed frame numbers.
- Keep a steady visual rhythm: if you punch on beats 2 and 4 in bar 1, keep that pattern.

## 5. Continuous movement with the music

For a pulse that follows the music all the time (a glow that breathes with the bass, bars
that bounce), let Cavalry read the sound instead of placing keys:

```js
// scale.y between 1 (quiet) and 1.4 (loudest), from the kick and bass only
cav.sound('/abs/path/music.wav', logo, 'scale.y', { min: 1, max: 1.4, low: 20, high: 150, smooth: 2 })
```

Key the big hits to the grid as usual; use `cav.sound` for texture under them, not instead of them.

## 6. Check

```bash
cav sheet 108,114,120,126,132 --bpm 120   # tiles show b4.6 ... b5.4: hits should land on whole beats
cav render -o renders/final.mp4 --audio music.wav
cav sync renders/final.mp4                # renders/sync.png and a report
```

`cav sync` finds the moments the rendered picture changes sharply (cuts, the fastest frame of
a pop or slam, moves that land) and measures each one against the beat grid in frames. It also
lists strong musical moments (sections, rises, silences, big accents) with no visual hit. Its
picture adds a "pic" lane under the spectrogram view: green marks are hits on the grid, red
marks are off it. Fix red marks you did not mean, and decide for each unanswered moment
whether it needs a hit. The grid can be a frame or two off on long tracks, so do not chase
±1-frame offsets.

You still cannot hear the result. Tell the user which frames the hits are on, so they can
check. If you generate music yourself, say that no one has listened to it yet.
