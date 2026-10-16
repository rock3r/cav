# Production flow: from brief to final render

The standard flow has five stages. The first four plan a piece, and the fifth builds it.
Sections 3 to 7 describe these stages. Follow a project flow file when it defines different
stages. Each stage uses earlier outputs or files supplied by the user. Read
`cav guide production` for command details and `cav guide scene` for scene construction.

`cav guide scene` explains the build loop. `cav guide production` covers services, licences
and review. `cav guide music` covers beat timing.

## 1. The flow in effect

The end of this guide prints the active stages under "The flow in effect". cav first checks
the project's `flow.md`, then `~/.cav/flow.md`, then its built-in standard flow. The user can
change the order, content or number of stages through instructions or a flow file. A flow
file replaces the entire standard flow.

`cav plan` reports each stage's state and the next step. It does not block commands.

A flow file is Markdown. cav reads four things from it:

- `## name` starts a stage.
- `outputs:` lists output paths separated by commas. Paths are relative to the project.
  `*` matches within one folder. A path ending in `/` names a folder.
- `review: yes` or `review: no` controls approval through `cav review`. The first output
  holds the approval and must be one `.json` file when review is enabled.
- `use:` gives the agent instructions. cav prints this text without running it.

cav ignores lines inside code blocks and `<!-- -->` comments. Other text gives the agent
instructions. If the file contains an error, cav prints a warning with its line number.
The agent must fix that file. cav does not switch to another flow.

## 2. Only the build loop is mandatory

The agent can skip planning stages when the work does not need them. Tell the user which
stage was skipped and why. A short sting with a clear request needs no storyboard. An
unclear request needs a brief. The build loop is required.

In stage 5, inspect a sheet after every change. Run `cav check` before rendering, and use
`cav render` for the final video.

## 3. Stage 1: the brief

Agree with the user on the goal, audience and format before choosing tools.

- Ask a few questions at a time about the goal, audience, tone, length, format, deliverables
  and music.
- Collect the images, links and videos the user mentions. Add them as the first mood board items.
- Offer two or three directions in one or two sentences each. Let the user choose or combine them.
- Write the brief in `brief.json`. Run
  `cav brief init --title "..." --goal "..." --audience "..." --tone confident --tone playful --seconds 15 --bpm 120 --width 1080 --height 1920 --fps 30 --ref <link or file>`.
  Alternatively, run `cav brief init` without flags and fill the template by hand.
  Repeat `--tone`, `--must-have`, `--avoid`, `--deliverable` and `--ref` for multiple values.
  Run `cav brief show`. Fix its reported problems and fill every field under "Still to fill".
- Start the review page in the background (`cav review brief.json &`; on Windows, in a
  second terminal), give the user its URL, then run `cav review wait brief.json`. The user
  gives a verdict on each field and presses Approve. Do not start stage 2 until
  `cav review export brief.json --json` shows `approved` as `true`.
- Add brief references to the mood board. For YouTube or Vimeo, run `cav ref add <link> --by brief`;
  cav records an embed without downloading the video. Copy a local file into
  `moodboard/` and record it with `cav ref add moodboard/<file> --licence <id> --by brief`.
  Keep links to other sites in the brief.

## 4. Stage 2: the mood board

Find references with `cav ref search` and save them with `cav ref get`, which records
their licence and credit. Read the user's Are.na channel with `cav ref arena`. Copy the
user's files into `moodboard/`, then run `cav ref add moodboard/<file> --licence <id>`.
Handle `.mp4`, `.mov`, `.webm` and `.m4v` clips the same way.

Run `cav ref add <youtube or vimeo link>` for a web video. cav gets its title and thumbnail
and records it only as a reference. cav does not download the video. The user can play it
on the review page. For other sites, save a picture with `cav ref get`.

Run `cav board mood` to create `moodboard.json` and `renders/moodboard.png`. The JSON has
one item per image, clip or video. Inspect the picture. It shows six frames for each clip
and a thumbnail for each web video. cav keeps an item's ID and take line when files change.

For each reference, record one useful colour relationship, rhythm, type style or movement.
Do not copy its layout, frame, character or sequence unless the user asks for a copy.

Run `cav board mood take <id> "<one thing to take>"` for every item. The review page shows
the line under "Take from it". `cav board mood` lists items that still need a line.

Let the user review the board. Start the review page in the background
(`cav review moodboard.json &`; on Windows, in a second terminal), give the user its URL,
then run `cav review wait moodboard.json`. The user gives each item Keep, More like this or
Drop, and can leave notes. Act on each verdict:

- Drop: run `cav board mood drop <id>`. The file stays, and cav never brings the item back.
- More like this: search for more references like it and add them.
- A note that corrects a take line: change it with `cav board mood take <id> "..."`.

Resolve the notes with `cav review resolve`. Do not start stage 3 until the user approves
(`cav review export moodboard.json --json` shows `approved` as `true`).

Use references in the render only when their licences allow it and the user asks for it.
See `cav guide production`, "Licences come first".

## 5. Stage 3: the storyboard

Run `cav board init --shots 6`. It reads size, fps, length, tempo and title from
`brief.json` and reports the values it used. Without a brief, supply flags, for example
`cav board init --width 1080 --height 1920 --fps 30 --seconds 15 --bpm 120 --shots 6`.
Record the approved direction in `storyboard.json`. Put a short visual description in
`style.prompt`, chosen colours in `style.palette`, and kept mood board images in `style.refs`.
Use image items only. Do not send clip contact sheets or video thumbnails as references.
`cav board frames` sends these images with every frame. For each shot, write its action in
`what` and what the viewer learns or feels in `why`. The agent writes `why`. Run
`cav board frames` and `cav board sheet`, then inspect `renders/board.png` and fix weak shots.

cav first tries an available image service: keyed services, then mflux and comfyui. It then
tries sketches and finally greybox cards. Without an image service, write one SVG per shot,
such as `board/s1.svg`, before `cav board frames`. Cavalry renders each SVG as a sketch.
See `cav guide production`, "Sketch frames". Greybox text cards show no composition, so
make sketches before storyboard review. Cavalry must be open with a saved or empty scene.

Then let the user review the storyboard. Start the review page in the background
(`cav review storyboard.json &`; on Windows, in a second terminal), give the user its URL,
then run `cav review wait storyboard.json`. The user gives a verdict on each shot, draws and
writes notes, and presses Approve storyboard. Fix each note and each shot marked redo in
`storyboard.json` (and its frames), resolve the note with `cav review resolve`, and wait again.

Wait until the user presses Approve storyboard. Confirm that
`cav review export storyboard.json --json` reports `approved` as `true`. The agent cannot
approve for the user. This approval must precede music generation, Veo clips
(`cav board motion`), the animatic and the build. Later changes can require another
generation, music take or render. Changing a shot or storyboard setting removes approval,
so the user must approve again.

## 6. Stage 4: music and animatic

1. Use the user's track or make one for the approved shots:
   `cav music gen "<style, mood, tempo>" --board storyboard.json --takes 2`. It writes the
   takes into `music/` and prints their paths. Listen with `cav listen`, pick one, and
   call it `<track>` below.
2. Create the beat grid with `cav beats <track> --fps <storyboard fps> -o build/grid.json`.
   Set `"grid"` in `storyboard.json`.
3. Add moving shots if needed with `cav board motion --only s2,s5`.
4. Build the animatic with `cav board animatic --audio <track>`. It uses the approved frames.
   The user reviews only timing
   against the music. Start the review page in the background
   (`cav review renders/animatic.mp4 &`; on Windows, in a second terminal), give the user
   its URL, then run `cav review wait`.

## 7. Stage 5: the build loop

Follow `cav guide scene`. `cav scene new` reads size, fps and length from `brief.json` and
reports the values it used. If the storyboard changed them, pass its values with
`cav scene new --width W --height H --fps F --seconds S`. `cav board place` adds the
storyboard frames as scene placeholders. Without a storyboard, start with the plan table
in `cav guide scene`. Change, render and review the scene until the user approves the render.
