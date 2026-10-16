# Software showcase videos

Use these rules for launch videos, product demos, feature videos and UI walkthroughs.
Read recipe 12 in `cav guide design` for UI construction. `cav guide production` covers
storyboards and user review.

## Contents
1. Before you build
2. Story and message
3. A faithful UI
4. Framing and attention
5. Motion and continuity
6. Sound
7. Timing and formats

## 1. Before you build

- **Follow the default flow.** Make a brief, mood board, storyboard, animatic and final
  video (`cav guide flow`). Get the user's approval at each required review stage before
  spending time on detailed work (`cav guide production`). Clarify unclear instructions
  before starting a costly build.
- **Agree on the editing style.** Use one continuous shot by default, as section 5 describes.
  If the user asks for cuts, request example videos for the desired style.
- **Use real product evidence.** Read the app's code and inspect screenshots or recordings
  before drawing. Do not invent features, screens or controls.

## 2. Story and message

- **Choose a structure for the format.** A launch trailer can show a problem, the product,
  its features, the result and one action. A feature tour, release update or tutorial can
  use a different order that fits its content.
- **Give each section one message.** End with one action, such as install, download or visit.
- **Explain every feature you show.** Give commands and numbers enough context for a new
  viewer. Show the action and its result. Use little on-screen text. Record each shot's
  purpose in the storyboard's `why`; `cav check` warns when it is missing.
- **Introduce features in story order.** Focus on a feature in its own section. An early
  overview can include later features if it does not emphasize them.
- **Change the main visual between sections.** Reuse an element when the story needs to
  show it in different contexts, such as a widget on home and lock screens.
- **Write short outcome labels.** State what the viewer gains. Revise repeated wording
  and remove stock phrases. Button names alone do not explain the result.

## 3. A faithful UI

- **Build the UI as layers.** Use screenshots only to compare the result
  (`cav guide design`, recipe 12).
- **Match the real components.** Copy the app's colours, fonts, sizes, corner radii,
  stroke widths and spacing. Keep the values in one file, such as
  `themes.json`, that every script reads.
- **Use the real icons.** Use the app's icon set and brand marks. If a shot focuses on
  an icon, animate its parts when that helps explain the action.
- **Match platform controls.** Status bars, lock screens, menu bars, window controls and
  notifications must match the real platform.
- **Use the product's visual forms.** Build graphics from its bars, rings, cards and
  indicators.
- **Use realistic sample data.** Show what each feature does with real provider and product
  names and believable numbers, unless the user directs otherwise. Avoid placeholder text
  and private data.

## 4. Framing and attention

- **Use device frames only when the story needs them.** Show mobile UI elements directly
  when the device adds no information. For desktop apps, leave out unrelated desktop areas
  and monitor borders. Show the device for stories across devices.
- **Show relevant UI.** Include other parts only when they support a transition or show
  the app in context, such as a widget on a home screen.
- **Use camera movement to focus attention.** Show the full screen briefly, move toward
  the relevant control, then return to the full view. Use rig tweens with `inOutCubic`,
  `cav.zoomThrough` and a blur filter when depth helps.
- **Limit large zooms.** Use fast zooms for 2-3 key moments. Zoom and pan to show needed
  details or to move between scenes.
- **Use camera holds on purpose.** Hold on one feature long enough to show its result.
  Keep a small visible change, such as slow movement or a ticking clock. `cav check`
  reports holds over about 2 s with no movement. Use 2.5D moves when depth clarifies the scene.
- **Show each interaction's result.** Complete the cursor movement or tap before changing
  scenes. Hold the result long enough to read. Use `inOutCubic` for the cursor, `punch`
  for the button and a visible change after it.

## 5. Motion and continuity

- **Use one continuous shot by default.** Bring elements in and out as the camera moves
  between sections. Keep earlier content visible under a transition when it helps.
  Move shared elements into the next section.
- **Use cuts when the user asks for them.** Apply the chosen cutting style consistently.
  Mix cuts and continuous transitions when the user requests that combination.
- **Check both sides of every boundary.** Look at the last frame before each hand-off and
  the first frame after it (`cav seams`, `cav frame`). Look for elements that pop in, jump
  or vanish: a one-frame flash, a font weight jump, a dot off centre, the wrong layer
  order. Check nearby frames for early or late appearances. A circular reveal, for example,
  can appear before its animation starts. `cav check` flags sudden changes at boundaries.
  Mark an intended hard cut with `"cut": true` on the shot.
- **Emphasize one element at a time.** Stagger other elements by 3-4 frames for small,
  quick items. Use longer delays for large or slow moves.
- **Springs on containers, never on values.** Cards and panels can overshoot when they
  enter. Numbers, bars and rings must settle without overshoot because intermediate values
  can be wrong. `cav check` warns about spring, back and bounce easing on data. If its
  classification is wrong, set `cav.role(layer, 'container')` or `'data'`.
- **Draw highlights along their outline.** Use `cav.drawOn` for a callout box sized to
  its target. Give it enough contrast. Fade its fill near the end if needed.

## 6. Sound

- **Use one timing file.** Find beats with `cav beats`, record section frames in
  `timing.json`, and have every script read it (`cav guide music`).
- **Fit the music to the story.** Plan quiet sections, changes in texture, drops and
  silence around the visual events they support.
- **Place sound effects where they remain clear.** Use a few effects during quieter music.
  Mix their levels and use stereo position when it helps locate an action.

## 7. Timing and formats

- **Allow reading time.** Hold text for about one second per three words. At 1080p, keep
  text at least 28-30 px. Remove a line after the viewer has time to read it.
- **Choose the length with the user.** Start planning a launch video at 60-80 seconds.
- **Use 16:9 by default for YouTube.** Make other formats when the user requests them.
  After production, offer 9:16 and 1:1 versions or 15 s and 6 s cuts if useful. Give
  each short cut one message. Rework the layout for new formats and keep key UI in the
  middle half.
  `cav scene reframe --to 9:16` (or `--to 1:1`) makes a copy of the comp at the new size.
  It fits the `rig` group (or the layers in `--key`) into the middle half and scales the
  background to cover the frame. It also lists what to fix. Use `--dry-run`
  first to read the plan. The original comp remains unchanged. Refine the copy by moving
  titles and captions into the bands around the middle half; `cav.safeArea()` gives their
  bounds. Enlarge small text and adjust timing where needed. Check the result with
  `cav sheet --comp "<new name>"` and `cav check --comp "<new name>"`. Ask whether short cuts
  should loop. Make the last frame usable as a thumbnail. Check that the video still
  communicates its message without sound.
