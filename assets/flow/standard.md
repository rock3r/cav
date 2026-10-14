# Standard flow

This is the flow cav uses when the project has no `flow.md` and `~/.cav/flow.md` does not
exist. Copy it to `flow.md` and change it to make your own.

## brief

Work like a creative director. Ask a few questions at a time, then write the brief down. Run
`cav brief init`, fill in the fields, and run `cav brief show` to see what is still missing.
The user approves it in `cav review brief.json`. Skip this stage when the user gives a
complete brief.

outputs: brief.json
review: yes
use: cav brief init, then cav brief show

## mood board

Find references, record their licence and credit, and add the user's own files and web
videos. Then build the board with `cav board mood`: it writes `moodboard.json` and
`renders/moodboard.png`. Write one "take from it" line for every item. A reference teaches; it
is not a template. The user gives each item Keep, More like this or Drop, and approves the
board in `cav review moodboard.json`. Skip this stage when the user brings a style guide or
frames.

outputs: moodboard.json, moodboard/, renders/moodboard.png
review: yes
use: cav ref search, cav ref get, cav ref add, then cav board mood, then cav board mood take for each item

## storyboard

Plan the shots, make a frame for each one, and look at the sheet. The user gives a verdict on
each shot and approves the storyboard in `cav review storyboard.json`. Do not start anything
slow or costly before that. Skip this stage when the piece is under about 8 seconds, or has
one shot. With no image service, write a `board/<id>.svg` of simple shapes for each shot first:
cav draws them through Cavalry as sketch frames (`cav guide production`, "Sketch frames").

outputs: storyboard.json, board/
review: yes
use: cav board init, cav board frames, cav board sheet

## music and animatic

Use the user's track, or make one that follows the approved shots. Find the beats, then build
the animatic to check the timing against the music. The user reviews the animatic in
`cav review renders/animatic.mp4`. A video review holds notes, not an approval, so this stage
has no approval flag. Skip the animatic when there is no music or no storyboard.

outputs: renders/animatic.mp4, build/grid.json
review: no
use: cav music gen, cav beats -o build/grid.json, cav board animatic

## build

Build the scene in Cavalry. Look at a sheet after every change, run the check before a render,
and render. The user approves the render in `cav review renders/final.mp4`. A video review
holds notes, not an approval, so this stage has no approval flag. Never skip this stage.

outputs: renders/final.mp4, scenes/*.cv
review: no
use: cav guide scene, then cav run, cav sheet, cav check, cav render -o renders/final.mp4
