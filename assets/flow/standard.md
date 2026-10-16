# Standard flow

When a project has no `flow.md` and `~/.cav/flow.md` is absent, cav uses these stages.
Copy this file to `flow.md` to change the stages for a project.

## brief

Ask about the goal, audience and format in small groups of questions. Run `cav brief init`
and fill in the fields. Run `cav brief show` to find missing details. The user approves
the brief through `cav review brief.json`. Skip this stage when the user provides a complete brief.

outputs: brief.json
review: yes
use: cav brief init, then cav brief show

## mood board

Find references and record each licence and credit. Add the user's files and web videos.
Run `cav board mood` to create `moodboard.json` and `renders/moodboard.png`. For each item,
write one line about a colour, timing detail or movement to use. Do not copy its layout.
The user marks each item Keep, More like this or Drop. The user then approves the board
through `cav review moodboard.json`. Skip this stage when the user provides a style guide or frames.

outputs: moodboard.json, moodboard/, renders/moodboard.png
review: yes
use: cav ref search, cav ref get, cav ref add, then cav board mood, then cav board mood take for each item

## storyboard

Plan each shot, create its frame and inspect the sheet. The user reviews every shot and
approves the storyboard through `cav review storyboard.json`. Wait for approval before
starting slow or paid work. Skip this stage for a single shot or a piece under about
8 seconds. If no image service is available, draw simple shapes in `board/<id>.svg` for
each shot. Cavalry renders these files as sketch frames (`cav guide production`, "Sketch frames").

outputs: storyboard.json, board/
review: yes
use: cav board init, cav board frames, cav board sheet

## music and animatic

Use the user's track or make one for the approved shots. Find its beats and build the
animatic. The user checks its timing through `cav review renders/animatic.mp4`. Video reviews
collect feedback without an approval flag. Skip the animatic if there is no music or storyboard.

outputs: renders/animatic.mp4, build/grid.json
review: no
use: cav music gen, cav beats -o build/grid.json, cav board animatic

## build

Build the scene in Cavalry. Inspect a sheet after every change. Run `cav check` before
rendering. The user reviews the render through `cav review renders/final.mp4`. Video reviews
collect feedback without an approval flag. Always complete this stage.

outputs: renders/final.mp4, scenes/*.cv
review: no
use: cav guide scene, then cav run, cav sheet, cav check, cav render -o renders/final.mp4
