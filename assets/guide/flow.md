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
| 1. Brief | none; you work as a creative director | `brief.md` and `brief.json`, written by hand | The user confirms the brief in chat | The user gives a complete brief |
| 2. Mood board | `cav ref search`, `get`, `arena`, `add`; `cav board mood` | `moodboard/`, `renders/moodboard.png` | Show `renders/moodboard.png` to the user | The user brings a style guide or frames |
| 3. Storyboard | `cav board init`, `frames`, `sheet` | `storyboard.json`, `board/`, `renders/board.png` | The user approves the storyboard | The piece is under about 8 seconds, or has one shot |
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
- Write the brief down in `brief.md` (for the user to read) and `brief.json` (size, fps,
  length, tempo, `mustHave`, `avoid`, references). Then wait for the user to confirm it in
  chat. Do not start stage 2 before that.

## 4. Stage 2: the mood board

Find references with `cav ref search`, save them with `cav ref get` (it records the licence
and credit), read a user's Are.na channel with `cav ref arena`, and add the user's own files
with `cav ref add`. Then run `cav board mood` and look at `renders/moodboard.png`. Show it to
the user and ask what to keep and what to drop.

**A reference teaches. It is not a template.** For each reference, write down what you take
from it: the essence. This can be a colour relationship, a rhythm, a type mood or a kind of
motion. Never copy a layout, a frame, a character or a sequence. The one exception is a user
who asks for a copy directly.

A reference is for looking at. It goes into the render only when its licence allows it and
the user asks for it (`cav guide production`, "Licences come first").

## 5. Stage 3: the storyboard

Run `cav board init`, write what happens in each shot, then `cav board frames` and
`cav board sheet`. Look at `renders/board.png`, fix the weak shots, and show it to the user.

Do not start anything slow or costly before the user approves the storyboard. That means
music generation, Veo clips (`cav board motion`), the animatic and the build. After the
approval, every change costs a generation, a music take or a render.

## 6. Stage 4: music and animatic

1. Music: use the user's track, or make one that follows the approved shots:
   `cav music gen "<style, mood, tempo>" --board storyboard.json --takes 2`. It writes the
   takes into `music/` and prints their paths. Listen with `cav listen`, pick one, and
   call it `<track>` below.
2. Beat grid: `cav beats <track> -o build/grid.json`. Set `"grid"` in `storyboard.json`.
3. Optional moving shots: `cav board motion --only s2,s5`.
4. Animatic: `cav board animatic --audio <track>`. It uses the approved frames as they are,
   so the user does not approve the frames a second time. The review is only about timing
   against the music. Start the review page in the background
   (`cav review renders/animatic.mp4 &`; on Windows, in a second terminal), give the user
   its URL, then run `cav review wait`.

## 7. Stage 5: the build loop

Follow `cav guide scene`. With a storyboard, `cav board place` first puts its frames into the
scene as placeholders; without one, start from the plan table in `cav guide scene`. Repeat change, render and review until the user approves the render.
