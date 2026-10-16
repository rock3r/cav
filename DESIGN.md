---
name: cav review
description: An animator's exposure sheet for reviewing an agent's renders, frame by frame.
colors:
  cav-orange: "#ff5a36"
  cav-orange-ink: "#c2360e"
  cav-orange-ink-dark: "#ff8a66"
  cav-orange-soft: "rgba(255, 90, 54, 0.11)"
  cav-orange-soft-dark: "rgba(255, 90, 54, 0.15)"
  on-orange: "#16181f"
  paper: "#ffffff"
  paper-2: "#f4f5f7"
  paper-3: "#e8eaee"
  ink: "#16181f"
  ink-2: "#353a45"
  graphite: "#5c6370"
  rule: "rgba(22, 24, 31, 0.11)"
  rule-2: "rgba(22, 24, 31, 0.22)"
  rule-3: "rgba(22, 24, 31, 0.62)"
  paper-dark: "#16181f"
  paper-2-dark: "#1c1f27"
  paper-3-dark: "#282c35"
  ink-dark: "#eceef2"
  ink-2-dark: "#c8ccd4"
  graphite-dark: "#9aa1ad"
  rule-dark: "rgba(236, 238, 242, 0.09)"
  rule-2-dark: "rgba(236, 238, 242, 0.19)"
  rule-3-dark: "rgba(236, 238, 242, 0.5)"
  stage: "#0b0c0e"
  stage-text: "#b9bec8"
  ok: "#1d7a47"
  warn: "#9a5800"
  bad: "#bf2d2d"
  ok-dark: "#62d293"
  warn-dark: "#f2b94d"
  bad-dark: "#ff8079"
typography:
  readout:
    fontFamily: "Sheet (Barlow Semi Condensed SemiBold), ui-sans-serif, -apple-system, sans-serif"
    fontSize: "24px"
    fontWeight: 600
    lineHeight: 1
    letterSpacing: "0.01em"
    fontFeature: "tnum"
  frame-numeral:
    fontFamily: "Sheet (Barlow Semi Condensed SemiBold), ui-sans-serif, -apple-system, sans-serif"
    fontSize: "16px"
    fontWeight: 600
    lineHeight: 1.1
    letterSpacing: "0.01em"
    fontFeature: "tnum"
  column-head:
    fontFamily: "Sheet (Barlow Semi Condensed SemiBold), ui-sans-serif, -apple-system, sans-serif"
    fontSize: "12px"
    fontWeight: 600
    lineHeight: 1
    letterSpacing: "0.1em"
  run-label:
    fontFamily: "Sheet (Barlow Semi Condensed SemiBold), ui-sans-serif, -apple-system, sans-serif"
    fontSize: "12px"
    fontWeight: 600
    lineHeight: 1.2
    fontFeature: "tnum"
  title:
    fontFamily: "ui-sans-serif, -apple-system, BlinkMacSystemFont, Segoe UI Variable Text, Segoe UI, Roboto, Helvetica Neue, Arial, sans-serif"
    fontSize: "16px"
    fontWeight: 650
    lineHeight: 1.45
    letterSpacing: "-0.005em"
  body:
    fontFamily: "ui-sans-serif, -apple-system, BlinkMacSystemFont, Segoe UI Variable Text, Segoe UI, Roboto, Helvetica Neue, Arial, sans-serif"
    fontSize: "13px"
    fontWeight: 400
    lineHeight: 1.45
  label:
    fontFamily: "ui-sans-serif, -apple-system, BlinkMacSystemFont, Segoe UI Variable Text, Segoe UI, Roboto, Helvetica Neue, Arial, sans-serif"
    fontSize: "12px"
    fontWeight: 650
    lineHeight: 1
  meta:
    fontFamily: "ui-sans-serif, -apple-system, BlinkMacSystemFont, Segoe UI Variable Text, Segoe UI, Roboto, Helvetica Neue, Arial, sans-serif"
    fontSize: "11px"
    fontWeight: 400
    lineHeight: 1.45
    fontFeature: "tnum"
  tooltip:
    fontFamily: "ui-sans-serif, -apple-system, BlinkMacSystemFont, Segoe UI Variable Text, Segoe UI, Roboto, Helvetica Neue, Arial, sans-serif"
    fontSize: "11.5px"
    fontWeight: 500
    lineHeight: 1.3
rounded:
  snap: "2px"
  sm: "4px"
  md: "5px"
  lg: "6px"
  round: "50%"
spacing:
  xs: "2px"
  sm: "4px"
  md: "6px"
  row: "9px"
  lg: "10px"
  xl: "12px"
  frame-column: "76px"
  frame-column-narrow: "72px"
  sheet: "392px"
  sheet-mid: "312px"
  sheet-chat: "264px"
  bar: "46px"
  timeline: "38px"
components:
  button-tool:
    backgroundColor: "transparent"
    textColor: "{colors.ink-2}"
    rounded: "{rounded.sm}"
    height: "30px"
    padding: "0 7px"
  button-tool-hover:
    backgroundColor: "rgba(22, 24, 31, 0.06)"
    textColor: "{colors.ink}"
  button-tool-pressed:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.paper}"
  button-add:
    backgroundColor: "{colors.cav-orange}"
    textColor: "{colors.on-orange}"
    typography: "{typography.label}"
    rounded: "{rounded.sm}"
    height: "28px"
    padding: "0 10px 0 8px"
  button-add-disabled:
    backgroundColor: "{colors.paper-3}"
    textColor: "{colors.graphite}"
  button-send:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.paper}"
    rounded: "{rounded.sm}"
    height: "36px"
  button-send-disabled:
    backgroundColor: "{colors.paper-3}"
    textColor: "{colors.graphite}"
  input-reply:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.ink}"
    rounded: "{rounded.sm}"
    height: "28px"
    padding: "0 8px"
  select:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.ink}"
    rounded: "{rounded.sm}"
    height: "30px"
    padding: "0 24px 0 8px"
  tooltip:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.paper-2}"
    typography: "{typography.tooltip}"
    rounded: "{rounded.sm}"
    padding: "4px 8px"
  frame-cell:
    textColor: "{colors.ink}"
    typography: "{typography.frame-numeral}"
    width: "{spacing.frame-column}"
    padding: "9px 8px 9px 12px"
  frame-cell-playhead:
    textColor: "{colors.cav-orange-ink}"
  note-selected:
    backgroundColor: "{colors.cav-orange-soft}"
  kbd:
    backgroundColor: "{colors.paper-2}"
    textColor: "{colors.ink}"
    rounded: "{rounded.sm}"
    padding: "0 5px"
  toast:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.paper}"
    rounded: "{rounded.md}"
    padding: "9px 14px"
---

# Design System: cav review

## Overview

**Design reference: an animator's exposure sheet**

The review page uses rows like an animator's exposure sheet. Each note appears beside
its frame. The sheet scrolls with the playhead and keeps the current frame at a fixed
reading line. Notes appear in rows without separate rounded cards.

Use white in light mode and `#16181f` in dark mode. Separate rows with three graphite
line weights. Use a condensed sans-serif for headings and numerals. Give frame numbers
and timecodes tabular figures. Mark the playhead row and active note or drawing in cav
orange. Keep the video stage black in both themes.

Use thin, flat controls around the frame. Follow the system theme unless the user has
chosen light or dark manually. The default theme is light.

**Main rules:**
- Separate rows with line weight. Do not use cards or content shadows.
- Reserve cav orange for the playhead and active content tied to it.
- Keep the stage `#0b0c0e` in both themes.
- Use the embedded condensed font for sheet labels and the system sans elsewhere.
- Use tabular figures for frame numbers and timecodes.
- Draw line icons with one 1.5px stroke.

## Colors

The palette uses paper, ink, graphite, one playhead orange and a black stage.

### Primary
- **Cav Orange** (cav-orange): Use it for the timeline playhead and caret, including the
  playhead row's two rules and flag. Use it for the selected note's timeline mark, I/O range
  edges, wipe divider, focus and text carets, and Add button. Use ink (on-orange) for text on it.
- **Cav Orange Ink** (cav-orange-ink in light, cav-orange-ink-dark in dark): Use this
  for the frame number of the playhead row and live note. A live note covers the frame
  or range on screen. Keep plain fill orange out of small text.
- **Cav Orange Wash** (cav-orange-soft, cav-orange-soft-dark): Use this tint behind the
  selected note, timeline I/O range and selected text.

### Neutral
- **Sheet Paper** (paper, paper-dark): The page and sheet ground. In dark it is the cav ink colour itself.
- **Paper Shade** (paper-2) and **Paper Deep** (paper-3): Key caps and the disabled fill of the Add and Send buttons.
- **Ink** (ink, ink-dark): Open note text, frame numerals, the Send button fill and the pressed-tool fill.
- **Ink Soft** (ink-2, ink-2-dark): Resting tool icons, timecodes in the readout, reply threads.
- **Graphite** (graphite, graphite-dark): Column heads, field keys, run labels, timeline labels, meta lines, hints and resolved notes.
- **Rules** (rule, rule-2, rule-3 and their dark values): Three hairline weights. The light rule separates rows. The middle rule draws the frame column's edge, control borders and separators. The heavy rule draws the frame of the page: the title block foot, the sheet head and foot, the sheet's left edge and the timeline top.
- **Stage Black** (stage): The stage and the empty snapshot ground, in both themes. Text on the stage uses stage-text.

### Status
- **OK / Warn / Bad** (ok, warn, bad and their dark values): Resolved state and the agent's fix line (ok), the seeking indicator (warn), the armed delete button and error notes (bad). Status colours never decorate.

### Named Rules
**Orange use.** Cav orange marks the playhead, playhead row, live note frame number,
selection and controls that act on them. Do not use it elsewhere. Drawing strokes use
swatch colours: yellow `#ffd23f` by default, then red, cyan and white.

**Stage colour.** Keep the stage neutral black (`#0b0c0e`) in both themes.

**Resolved notes.** Show resolved note text and frame numbers in graphite. Show their
snapshots at 85% greyscale and 70% opacity. Set timeline marks to graphite at 45% opacity.

## Typography

**Sheet Font:** Embed the Latin subset of Barlow Semi Condensed SemiBold (600) as woff2.
Jeremy Tribby released it under SIL Open Font License 1.1. The page calls it `Sheet`
and falls back to ui-sans-serif.
**Body Font:** Use the system sans stack (ui-sans-serif, -apple-system,
BlinkMacSystemFont, Segoe UI Variable Text, Segoe UI, Roboto, Helvetica Neue, Arial).

Use the condensed face for narrow numerals and small spaced capitals in the sheet.
Use the system sans for sentences that people read or write, including notes.

### Hierarchy
- **Readout** (Sheet 600, 24px, line-height 1, +0.01em, tabular): The current frame number in the transport. It drops to 21px below 760px.
- **Frame numeral** (Sheet 600, 16px, line-height 1.1, +0.01em, tabular): The frame number in each note row and in the playhead row. A range end sets at 13px in ink-2 under a short rule.
- **Column head and field key** (Sheet 600, 12px, line-height 1, uppercase, +0.1em for column heads, +0.09em for title block field keys): Use FRAME and NOTE over the sheet. Use the same style for keys in the title block fields.
- **Run label** (Sheet 600, 12px, line-height 1.2, tabular): The frame range beside a run of frames with no note. Timeline second labels use the same face at 12px, line-height 1.
- **Title** (system sans 650, 16px, -0.005em): The video name in the title block and dialog titles.
- **Body** (system sans 400, 13px, line-height 1.45; note text at 1.42): Note text, the composer, and the page default.
- **Label** (system sans 650, 12px): The Add button. The Send button uses 650 at the body size.
- **Meta** (system sans 400, 11px to 12px, tabular): Timecodes under frame numerals, note meta lines, the hint and the send note.

### Named Rules
**Sheet font use.** Apply `Sheet` only to column heads, field keys, frame numbers, run
labels and timeline labels. Use the system sans for buttons, notes, tooltips and dialogs.
Do not set sentences in `Sheet`.

**Tabular numbers.** Apply `font-variant-numeric: tabular-nums` to every frame number
and timecode. This keeps numeral widths fixed during playback.

## Layout

Place a two-column grid below the title block. The stage takes the available width.
The sheet has a fixed width. Fill the viewport height (`100dvh`). Scroll notes
within the sheet column.

The stage column contains the stage, a 38px timeline and a transport bar at least 46px
high. The transport can wrap. The sheet column contains a 46px head, scrolling notes
and a foot with Send to agent. Shared 46px bars align across columns.

Each sheet row has a 76px frame column and a note column. Use 9px row padding and
9px 10px 10px note padding. Set snapshots to 72px at 16:9 with a 10px gap.

### Breakpoints
| Width | Behaviour |
|---|---|
| 1180px and up | The sheet is 392px. |
| 1024px to 1179px | The sheet is 312px. |
| below 1024px | Optional title block fields hide. |
| below 760px | The sheet drops below the stage. It takes min(70vh, 560px). The title block fields hide, and the readout drops its label. |
| Chat frame, 600px to 759px | The chat keeps two columns with a 264px sheet, and the page takes the frame height (600px by default). This CSS lives in `internal/reviewmcp/reviewmcp.go` (AppPage). |
| Sheet container below 340px | A container query keeps the columns: a 72px frame column and a 52px snapshot with an 8px gap. |
| Stage column container below 640px | The transport's separators hide, so a wrapped row does not start with a divider. |

### Time on the sheet and on the timeline

The sheet groups unmarked frames. The timeline shows elapsed time at a linear scale.

**Empty frame runs.** Give an unmarked run a height of 4 + 2.2·√frames px, capped at
160px. Draw a thin line every 6px. Do not draw second markers in the sheet.

**Timeline scale.** Draw timeline ticks every second and larger ticks every 5 seconds.
Add quarter-second ticks when each second has more than 60px. Label every 1, 5 or 10
seconds when space per second is above 44px, above 12px or below 12px, respectively.

**Change from the direction contract.** The contract requested heavier sheet rules every
second. A linear sheet for 3600 frames would be thousands of pixels tall. The built sheet
uses the square-root run height above. Second rules would imply a linear scale, so only
the timeline shows second markers.

## Elevation & Depth

Use line weights and one tinted row for hierarchy. Inset box-shadows draw the playhead
row rules, range edges, a 1px snapshot border and the selected swatch border. They do
not create a raised appearance.

### Shadow Vocabulary
- **Float** (`box-shadow: 0 6px 18px rgba(0, 0, 0, 0.18)`): The toast only. It floats over the page.
- **Handle** (`box-shadow: 0 1px 3px rgba(0, 0, 0, 0.4)`): The round grip of the wipe divider, on the stage.

### Named Rules
**Content shadows.** Rows, notes, bars and the composer cast no shadow. Only the toast
and wipe handle can cast shadows with the values above.

## Shapes

Use 4px corners on controls, inputs, tooltips and key caps. Use 5px on the theme switch
and toast, 6px on dialogs and 2px on snapshots. Swatches and the lock dot are round.
The sheet has square corners and reaches its container edges.

Mark the playhead with two orange triangles. Place a downward caret above its timeline
line and a right-pointing flag at the left of its sheet row.

Draw icons in a 16px box with a 1.5px `currentColor` stroke, round caps and round joins.
Use 15px icons for note actions and 13px icons in meta lines.

## Components

### Tool buttons
Use flat tool buttons without borders.
- **Shape:** 4px corners, 30px high, at least 30px wide, 0 7px padding. A text button adds 10px side padding and weight 550.
- **Rest:** transparent, ink-2 icon.
- **Hover / Active:** a 6% ink wash and ink icon; rule-2 when pressed down. Colour transitions take 120ms ease-out.
- **Pressed or chosen** (aria-pressed or aria-checked): an ink fill with paper icon. This marks the drawing tool, the theme and the filter. A chosen swatch instead shows a 2px paper gap and a 1.5px ink ring.
- **Disabled:** graphite at 55% opacity.

### Add (the composer's action)
Place the Add button in the playhead row at the reading line. Give it cav orange fill,
ink text, 650 weight, 12px text and 28px height. Brighten it by 6% on hover. Disabled
buttons use paper-3 fill and graphite text.

### Send to agent
Place Send to agent across the full sheet foot. Use a 36px ink bar with paper text at
weight 650. Brighten it by 15% on hover. Use paper-3 and graphite when disabled.

### Inputs and select
- **Reply input:** 28px high, a 1px rule-2 border, 4px corners, paper ground. Focus turns the border cav orange and drops the outline.
- **Composer:** a borderless textarea in the playhead row, 13px at line-height 1.42, growing with its content from 40px to 160px.
- **Select:** 30px high, a 1px rule-2 border, 4px corners, a drawn chevron. Hover darkens the border to rule-3.
- **Focus:** every other control shows a 2px cav orange outline with a 2px offset.

### Tooltips
Tooltips show a control label and shortcut, such as "Back one frame  ←" or
"Add note  ⌘↵". Use ink fill, paper-2 text, 4px corners and 4px 8px padding; swap colours
in dark mode. Show a tooltip after 350ms of hover or keyboard focus. Move it up 3px
over 120ms. Hide it on devices without hover. Position it near the relevant control.

### The sheet (signature component)
- **Head:** FRAME and NOTE column heads over a heavy rule, with the note count and the resolved-notes filter at the right.
- **Note row:** the frame cell (frame numeral, timecode under it, range end under a short rule) beside the note body (snapshot, words, meta line, actions). Hover adds a 4.5% ink wash. Actions fade in on hover, focus or selection, and always show without hover. A reply thread hangs off a rule-2 left line.
- **Live and selected:** the live note's frame numeral turns cav orange ink. The selected note takes the orange wash.
- **Playhead row:** the composer. Two orange rules above and below, an orange frame-column edge, an orange flag, and the frame numeral in orange ink. The sheet scrolls so this row stays at the reading line as frames step.
- **Run of empty frames:** a run label in graphite beside a strip of ruled paper (see Layout).

### Timeline strip
Place a 38px paper timeline below the stage. Draw the waveform behind ticks at 20%
graphite. Show notes as 9px ink marks at the top. Resolved marks use graphite at 45%;
the selected mark uses cav orange. Draw the playhead as a 2px orange line with a caret.

### Key caps
Inline key labels in the shortcuts dialog: paper-2 ground, a 1px rule-2 border with a 2px bottom edge, 4px corners, system sans 600 at 11px.

### Coarse pointers
Under `pointer: coarse`, give tool buttons, swatches, note actions, Add, reply input
and select at least 40px targets. Set theme buttons to 36px by 38px.

## Do's and Don'ts

### Do:
- **Do** write each note into the row of its frame, and keep the playhead row at the reading line.
- **Do** keep cav orange for the playhead, the playhead row, the live note's frame number, the selection and the controls that act on them.
- **Do** keep the stage neutral black (#0b0c0e) in both themes.
- **Do** set column heads, field keys, frame numerals, run labels and timeline labels in the Sheet face, and every other word in the system sans.
- **Do** use tabular figures for every frame number and timecode.
- **Do** separate things with the three rule weights: light between rows, middle for the frame column and control borders, heavy for the page's frame.
- **Do** put the label and the shortcut key in each control's tooltip.
- **Do** give every control a 40px target under a coarse pointer.
- **Do** draw icons as 16px line icons in one 1.5px stroke with round caps.

### Don't:
- **Don't** wrap notes in cards, rounded panels or a comment rail.
- **Don't** cast shadows on content. Only the toast and the wipe handle may cast one.
- **Don't** use decorative or tonal gradients. Hard-stop gradients may draw the ruled-paper hairlines.
- **Don't** use orange for status, decoration, headings or the person's drawing strokes.
- **Don't** draw seconds rules into the sheet. Its runs of frames are on a square-root scale; time in seconds belongs to the timeline.
- **Don't** set sentences, buttons or tooltips in the Sheet face.
- **Don't** tint or theme the stage.
