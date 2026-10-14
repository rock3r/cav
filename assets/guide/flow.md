# Production flow: from brief to final render

A longer piece goes through five stages. Stages 1 to 4 plan it. Stage 5 builds it. Each stage
starts from the files of the stage before it, or from files the user brings. Command details
are in `cav guide production`. How to build the scene is in `cav guide scene`.

Other guides: `cav guide scene` (the build loop), `cav guide production` (commands, services,
licences, review), `cav guide music` (beat sync).

## 1. The stages

The stages are a menu with a suggested order. The user can reorder, drop, add or change a
stage. When the user has no preference, follow the standard flow below.

| Stage | Commands | Files written | Review | Skip when |
|---|---|---|---|---|
| 1. Brief | `cav brief init`, `cav brief show`, `cav review brief.json`; you work as a creative director | `brief.json` | The user presses Approve in `cav review brief.json`; `cav review export brief.json --json` shows approved | The user gives a complete brief |
| 2. Mood board | `cav ref search`, `get`, `arena`, `add`; `cav board mood` | `moodboard/`, `renders/moodboard.png` | Show `renders/moodboard.png` to the user | The user brings a style guide or frames |
| 3. Storyboard | `cav board init`, `frames`, `sheet`, `cav review` | `storyboard.json`, `board/`, `renders/board.png` | The user presses Approve storyboard in `cav review storyboard.json`; `cav review export storyboard.json --json` shows approved | The piece is under about 8 seconds, or has one shot |
| 4. Music and animatic | `cav music gen`, `cav beats -o build/grid.json`, `cav board motion`, `cav board animatic`, `cav review` | `music/`, `build/grid.json`, `renders/animatic.mp4` | The user reviews the animatic (`cav review renders/animatic.mp4`) | Skip music generation when the user brings a track. Without music, or without a storyboard, skip the animatic; use the track directly in the build. |
| 5. Build loop | `cav board place` (with a storyboard), `cav run`, `cav sheet`, `cav onion`, `cav check`, `cav render`, `cav review` | `build/*.js`, `scenes/*.cv`, `renders/` | The user approves the render | Never |

## 2. Only the build loop is mandatory

You may skip stages 1 to 4. When you skip one, tell the user which stage and why. A short
sting with a clear request needs no storyboard. A vague request needs a brief.

The rules inside stage 5 do not change: look at a sheet after every change, run `cav check`
before a render, and render with `cav render`.

## 3. Stage 1: the brief

Work like an agency creative director. Do not start with the tools.

- Ask a few questions at a time, not all at once: goal, audience, tone, length, format,
  deliverables, music.
- Collect every reference the user mentions: images, links, other videos. They become the
  first mood board items.
- Propose two or three short directions, one or two sentences each. Let the user pick or mix.
- Write the brief down in `brief.json`. Use `cav brief init --title "..." --goal "..."
  --audience "..." --tone confident --tone playful --seconds 15 --bpm 120 --width 1080
  --height 1920 --fps 30 --ref <link or file>`, or run `cav brief init` with no flags and fill
  the template by hand. Repeat `--tone`, `--must-have`, `--avoid`, `--deliverable` and `--ref`
  for more than one value. Then run `cav brief show`: fix each problem it names, and fill
  every field under "Still to fill".
- Start the review page in the background (`cav review brief.json &`; on Windows, in a
  second terminal), give the user its URL, then run `cav review wait brief.json`. The user
  gives a verdict on each field and presses Approve. Do not start stage 2 until
  `cav review export brief.json --json` shows `approved` as `true`.
- The brief's references become the first mood board items. A link stays a link. For a
  local file, copy it into `moodboard/` and record it with `cav ref add`, as stage 2 says.

## 4. Stage 2: the mood board

Find references with `cav ref search`, save them with `cav ref get` (it records the licence
and credit), read a user's Are.na channel with `cav ref arena`, and add the user's own files: copy each one
into `moodboard/`, then record it with `cav ref add moodboard/<file> --licence <id>`. Then run `cav board mood` and look at `renders/moodboard.png`. Show it to
the user and ask what to keep and what to drop.

**A reference teaches. It is not a template.** For each reference, write down what you take
from it: the essence. This can be a colour relationship, a rhythm, a type mood or a kind of
motion. Never copy a layout, a frame, a character or a sequence. The one exception is a user
who asks for a copy directly.

A reference is for looking at. It goes into the render only when its licence allows it and
the user asks for it (`cav guide production`, "Licences come first").

## 5. Stage 3: the storyboard

Run `cav board init --shots 6`. It takes the size, fps, length, tempo and title from
`brief.json` and says what it took. Without a brief, give them as flags, for example
`cav board init --width 1080 --height 1920 --fps 30 --seconds 15 --bpm 120 --shots 6`.
Then carry the approved direction into `storyboard.json`: set `style.prompt` to the look in a
few words, `style.palette` to the chosen colours, and `style.refs` to the mood board images that
show it. `cav board frames` sends these with every frame. Write what happens in each shot,
then run `cav board frames` and `cav board sheet`. Look at `renders/board.png` and fix the weak shots.

Then let the user review the storyboard. Start the review page in the background
(`cav review storyboard.json &`; on Windows, in a second terminal), give the user its URL,
then run `cav review wait storyboard.json`. The user gives a verdict on each shot, draws and
writes notes, and presses Approve storyboard. Fix each note and each shot marked redo in
`storyboard.json` (and its frames), resolve the note with `cav review resolve`, and wait again.

The gate is that the user presses Approve storyboard: `cav review export storyboard.json --json`
shows `approved` as `true`. You cannot approve for the user. Do not start anything slow or
costly before that. That means music generation, Veo clips (`cav board motion`), the
animatic and the build. After the approval, every change costs a generation, a music take
or a render. If you change a shot or the storyboard settings later, the approval is removed
and the user approves again.

## 6. Stage 4: music and animatic

1. Music: use the user's track, or make one that follows the approved shots:
   `cav music gen "<style, mood, tempo>" --board storyboard.json --takes 2`. It writes the
   takes into `music/` and prints their paths. Listen with `cav listen`, pick one, and
   call it `<track>` below.
2. Beat grid: `cav beats <track> --fps <storyboard fps> -o build/grid.json`. Set `"grid"` in `storyboard.json`.
3. Optional moving shots: `cav board motion --only s2,s5`.
4. Animatic: `cav board animatic --audio <track>`. It uses the approved frames as they are,
   so the user does not approve the frames a second time. The review is only about timing
   against the music. Start the review page in the background
   (`cav review renders/animatic.mp4 &`; on Windows, in a second terminal), give the user
   its URL, then run `cav review wait`.

## 7. Stage 5: the build loop

Follow `cav guide scene`. `cav scene new` takes the size, fps and length from `brief.json`
and says what it took. If the storyboard changed any of them, pass the storyboard's values
(`cav scene new --width W --height H --fps F --seconds S`). Then
`cav board place` puts its frames into the scene as placeholders; without one, start from the plan table in `cav guide scene`. Repeat change, render and review until the user approves the render.
