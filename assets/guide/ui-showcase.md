# Software showcase videos

Rules for videos that show software: launch videos, product demos, feature videos and UI
walkthroughs. Read this with `cav guide design` (recipe 12 shows how to rebuild a UI) and
`cav guide production` (storyboards and review).

## Contents
1. Before you build
2. Story and message
3. A faithful UI
4. Framing and attention
5. Motion and continuity
6. Sound
7. Timing and formats

## 1. Before you build

- **Follow the default flow**: brief, mood board, storyboard, moving previz (animatic), then
  the final (`cav guide flow`). Get the user's approval at each step before you polish
  (`cav guide production`). When the instructions are vague, ask the user before you spend
  time and money on a build.
- **Agree on the editing style.** The default is one long, continuous shot (section 5). If
  the user wants cuts, ask for reference videos of the style they want.
- **Ground everything in the real product.** Read the app's code, screenshots or screen
  recordings before you draw anything. Never make up a feature, a screen or a control.

## 2. Story and message

- **Pick the structure for the format.** A launch trailer can open on the problem the
  product removes, then show the product, the features, the payoff and one call to action.
  This works, but it is formulaic. Other formats (a feature tour, a release update, a
  tutorial) do not need to open on a problem.
- **One message per section**, and end with a single next step (install, download, visit).
- **Explain every feature you show.** The viewer has never seen the product: a command or a
  number with no context means nothing. Explain by showing, not by telling. Keep text on
  screen to a minimum. Write the reason for each shot in the storyboard's `why`; `cav check`
  warns about a shot that has none.
- **Show things in story order.** Do not call out or focus on a feature before its own
  section. An early overview shot of the whole app may still include later features, as
  long as it does not draw attention to them.
- **Each section gets a new picture.** Do not reuse the same hero element for several
  sections in a row. The exception is a clear story reason, such as a montage of one widget
  in different places (home screen, lock screen).
- **Labels state outcomes, in few words.** Write what the viewer gets, not the names of
  buttons. Revise the copy so that lines do not all sound the same. Do not settle for the
  first phrasing, and avoid stock patterns such as "X, not Y".

## 3. A faithful UI

- **Rebuild the UI as layers.** Never animate a screenshot; use it only as a reference
  (`cav guide design`, recipe 12).
- **Match the real components exactly**: the app's own colour tokens, fonts, sizes, corner
  radii, stroke widths and spacing. Keep the tokens in one file (for example
  `themes.json`) that every script reads.
- **Use the real icons**, never placeholders. Use the app's own icon set and the real brand
  marks. When an icon is the focus of a shot, you can animate it in and out, including its
  parts, not only with a fade, scale or move.
- **Copy system chrome exactly.** Status bars, lock screens, menu bars, window controls and
  notifications must look as they do on the real platform.
- **Use the product's own shapes as the motif.** Build the video's graphics from the
  product's visual language (its bars, rings, cards, indicators), not from generic shapes.
- **Use realistic fake data that sells the feature**, unless the user says otherwise. Use
  real provider and product names and believable numbers. Never use lorem ipsum, and never
  use private data.

## 4. Framing and attention

- **No device frame unless the story needs it.** For a mobile app, do not put the UI inside
  a phone. Bring the UI elements into the video on their own and move between them. For a
  desktop app, do not show the rest of the desktop or the monitor bezels. Show the device
  only when the story needs it, for example a story across several devices.
- **Show only what matters.** Leave out parts of the UI that do not serve the shot. Keep
  them only when they help a transition, or when the shot shows the app in its real
  context (a widget on a home screen).
- **Direct attention with the camera.** Zoom, pan and blur what is not important (depth of
  field). Show the whole screen briefly, zoom to the part that matters, then pull back so
  the viewer stays oriented. Use rig tweens with `inOutCubic`, `cav.zoomThrough`, and a
  blur filter for depth of field.
- **Few big zooms.** Zoom to the element that matters, not to where the pointer is. Keep
  fast, dramatic zooms for 2-3 key moments. Zooms and pans also work as transitions between
  scenes.
- **Use camera holds on purpose.** When the camera stops, use the time to show one element
  or feature, then move the camera again. Keep something alive during a hold (a slow drift,
  a clock that ticks over). `cav check` reports holds over about 2 s where nothing moves.
  Use 2.5D camera and subject moves for drama, but rarely: not everything needs to fly at
  the viewer.
- **Let every interaction finish.** A cursor move or a tap must complete its action, and the
  result must stay on screen long enough to register. If you cut away too soon, it reads as
  a glitch. Tween the cursor with `inOutCubic`, `punch` the button, then show the change.

## 5. Motion and continuity

- **Default to one continuous shot.** Treat the video as an elaborate sequence shot, like
  a one-take music video: elements enter and leave the action all the time, and each scene
  flows into the next. Keep the previous shot playing underneath, and let shared elements
  travel from one shot to the next.
- **Cuts only when asked.** Fast, action-heavy videos can use hard cuts, but only when the
  user asks for that style. Then use cuts consistently. Do not mix the two styles unless
  the user says so.
- **Check both sides of every boundary.** Look at the last frame before each hand-off and
  the first frame after it (`cav seams`, `cav frame`). Look for elements that pop in, jump
  or vanish: a one-frame flash, a font weight jump, a dot off centre, the wrong layer
  order. Also look for elements that appear or vanish a few frames too early or too late,
  such as a circular reveal that shows as a small circle a few frames before its animation
  starts. `cav check` does the first look for you: it flags a boundary where the picture
  jumps in one frame. Mark a hard cut on purpose with `"cut": true` on the shot.
- **One hero at a time, and stagger the rest.** Staggering is almost always good: it gives
  the motion a direction and points at what matters. Choose the step for the move: 3-4
  frames for small, quick items, longer for big or slow moves.
- **Springs on containers, never on values.** Cards and panels may spring and overshoot as
  they enter. Numbers, bars and rings must settle with no overshoot, so the data never
  shows a wrong value.
- **Draw highlights on.** A callout box draws on along its outline (`cav.drawOn`); it does
  not scale or fade in. It fits its target exactly and contrasts with the background. Its
  fill may fade in as the draw nears its end, so that nothing appears all at once.

## 6. Sound

- **Cut on the beat, from one timing file.** Get the beats with `cav beats`, keep every
  section frame in one `timing.json`, and let every script read it (`cav guide music`).
- **Shape the music to the story**: a quiet build, a drop on the reveal, a change of texture
  for each section, and silence just before the big hit. The hits and crescendos belong in
  the music track, not in added sound effects.
- **Use few sound effects, in quiet moments.** Place each one where the music is quiet, so
  the effects and the music do not fight. Mix them so that the result sounds like one
  piece; pan them in stereo when it helps.

## 7. Timing and formats

- **Give text time to read, then move on.** Hold about 1 second per 3 words, with no text
  under 28-30 px at 1080p. Do not let a line stay after it has been read.
- **About 60-80 seconds for a launch video.** One study of 100 product videos found a median
  of about 75 s, with about 8 scene changes a minute.
- **16:9 for YouTube is the default.** Make other formats only when the user asks. At the
  end of production, offer social formats as further work if the user has not asked for
  them: 9:16 and 1:1 (re-lay out the scene, do not just crop it, and keep the key UI in the
  middle half), and 15 s or 6 s cuts with one message each.
  `cav scene reframe --to 9:16` (or `--to 1:1`) does the mechanical part. It makes a copy of
  the comp at the new size, fits the `rig` group (or the layers in `--key`) into the middle
  half, scales the background to cover the frame, and lists what to fix. Use `--dry-run`
  first to read the plan. The original comp is never changed. Then refine the copy: move
  titles and captions into the free bands above and below the middle half (`cav.safeArea()`
  gives the numbers), raise text that became small, retime if the shorter frame needs it,
  and check it with `cav sheet --comp "<new name>"` and `cav check --comp "<new name>"`. Ask whether short cuts should
  loop. The last frame should work as a thumbnail, and the video should make sense with the
  sound off.
