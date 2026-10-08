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

**Creative North Star: "The Exposure Sheet"**

The review page is an animator's exposure sheet. Frames run down ruled rows. Each note is written into the row of the frame it is about. The sheet scrolls with the playhead, so the current frame stays at the reading line. The page refuses the dark editing-suite panel with a rail of rounded comment cards.

The material is printed sheet stock. In light it is white paper; in dark it is ink (#16181f) stock. Graphite hairlines rule the paper, in three weights. A condensed grotesk letters the sheet's heads and numerals, and every frame number and timecode uses tabular figures. One colour, cav orange, marks what is under the eye. The stage stays neutral black in both themes, so the surround never changes a colour judgement.

The page is dense and quiet. Chrome is flat and thin, and it serves the frame. Light is the base. Dark applies when the system asks for it, unless the person chose light, or when the person chose dark.

**Key Characteristics:**
- Ruled rows, not cards. Depth comes from rule weight, not shadow.
- One reserved accent (cav orange) for the playhead and what sits on it.
- A neutral black stage (#0b0c0e) in both themes.
- An embedded condensed face for the sheet's lettering; the system sans for everything else.
- Tabular figures for every frame number and timecode.
- Drawn line icons in one 1.5px stroke.

## Colors

The palette is paper, ink and graphite, with one orange for the playhead and a neutral black stage.

### Primary
- **Cav Orange** (cav-orange): The fill for what is under the eye. It draws the playhead line and its caret on the timeline, the two rules and the flag of the playhead row in the sheet, the selected note's timeline mark, the edges of the I/O range, the wipe divider, the focus ring and the text caret. It also fills the Add button, which sits in the playhead row. Text on it is ink (on-orange).
- **Cav Orange Ink** (cav-orange-ink in light, cav-orange-ink-dark in dark): The orange used as text. It sets the frame numeral of the playhead row and of the live note (the note whose frame or range is on screen). The plain fill orange is never used as small text.
- **Cav Orange Wash** (cav-orange-soft, cav-orange-soft-dark): The tint behind the selected note, the I/O range on the timeline and text selection.

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
**The Under-the-Eye Rule.** Cav orange marks only what is under the eye: the playhead, the playhead row, the live note's frame number, the selection, and the controls that act on them. Nothing else is orange, at any size. Drawing strokes are not orange: the person draws in a swatch colour (yellow #ffd23f by default, then red, cyan, white).

**The Neutral Stage Rule.** The stage is neutral black (#0b0c0e) in both themes. Never tint it, and never let the theme change it.

**The Graphite Recedes Rule.** A resolved note turns graphite: its words, its frame numeral, and its snapshot (85% greyscale at 70% opacity). Its timeline mark drops to graphite at 45% opacity.

## Typography

**Sheet Font:** Barlow Semi Condensed SemiBold (600), embedded as a Latin subset in woff2 under the SIL Open Font License 1.1, by Jeremy Tribby. The page names it `Sheet` and falls back to ui-sans-serif.
**Body Font:** the system sans (ui-sans-serif, -apple-system, BlinkMacSystemFont, Segoe UI Variable Text, Segoe UI, Roboto, Helvetica Neue, Arial).

**Character:** The condensed face letters the sheet like a printed form: narrow, firm numerals and small spaced caps. The system sans carries every word a person reads or writes, so notes feel native to the device.

### Hierarchy
- **Readout** (Sheet 600, 24px, line-height 1, +0.01em, tabular): The current frame number in the transport. It drops to 21px below 760px.
- **Frame numeral** (Sheet 600, 16px, line-height 1.1, +0.01em, tabular): The frame number in each note row and in the playhead row. A range end sets at 13px in ink-2 under a short rule.
- **Column head and field key** (Sheet 600, 12px, line-height 1, uppercase, +0.1em for column heads, +0.09em for title block field keys): FRAME and NOTE over the sheet; the keys in the title block fields.
- **Run label** (Sheet 600, 12px, line-height 1.2, tabular): The frame range beside a run of frames with no note. Timeline second labels use the same face at 12px, line-height 1.
- **Title** (system sans 650, 16px, -0.005em): The video name in the title block and dialog titles.
- **Body** (system sans 400, 13px, line-height 1.45; note text at 1.42): Note text, the composer, and the page default.
- **Label** (system sans 650, 12px): The Add button. The Send button uses 650 at the body size.
- **Meta** (system sans 400, 11px to 12px, tabular): Timecodes under frame numerals, note meta lines, the hint and the send note.

### Named Rules
**The Lettering Rule.** The Sheet face sets only the sheet's lettering: column heads, field keys, frame numerals, run labels and timeline labels. Everything else, including buttons, notes, tooltips and dialogs, uses the system sans. Never set a sentence in the Sheet face.

**The Tabular Rule.** Every frame number and timecode uses tabular figures (`font-variant-numeric: tabular-nums`), so the numbers hold still while the playhead moves.

## Layout

The page is a two-column grid under a title block. The stage column takes the remaining width. The sheet column has a fixed width. The page fills the viewport height (100dvh), and the sheet scrolls inside its column.

The stage column stacks three bands: the stage, the timeline strip (38px), and the transport bar (46px minimum, wrapping when it must). The sheet column stacks the sheet head (46px), the scrolling sheet, and the sheet foot with Send to agent. Bars share one 46px height, so the title block, transport and sheet head line up.

Each sheet row is a grid of a frame column (76px) and the note column. Row padding is 9px; the note body is 9px 10px 10px with a 72px snapshot at 16:9 and a 10px gap.

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

The sheet and the timeline show time on two different scales. Each has one job.

**The Square-Root Paper Rule.** A run of frames with no note is a strip of ruled paper. Its height is 4 + 2.2·√frames px, capped at 160px. Hairlines rule the strip every 6px. The sheet draws no seconds rules.

**The Linear Timeline Rule.** The timeline under the stage carries time on a linear scale. It draws a tick every second and a heavier, taller tick every 5 seconds. It adds quarter-second ticks when a second gets more than 60px. It labels every 1, 5 or 10 seconds, depending on the room per second (more than 44px, more than 12px, or less).

**Adaptation from the direction contract.** The contract asked for heavier rules every second of frames on the sheet. The build does not do this. A linear sheet would make a 3600-frame render thousands of pixels tall, so each run of frames is sized on a square-root scale. On that scale, rules at whole seconds would misstate time, so the sheet does not draw the seconds rhythm. The timeline carries it on a linear scale instead.

## Elevation & Depth

The system is flat. Depth comes from the weight of the rules and from one tinted row, not from shadows. Inset box-shadows in the build are rules, not lift: the two orange rules of the playhead row, the range edges, the 1px ring around a snapshot, and the ring around the chosen swatch.

### Shadow Vocabulary
- **Float** (`box-shadow: 0 6px 18px rgba(0, 0, 0, 0.18)`): The toast only. It floats over the page.
- **Handle** (`box-shadow: 0 1px 3px rgba(0, 0, 0, 0.4)`): The round grip of the wipe divider, on the stage.

### Named Rules
**The Flat Sheet Rule.** Content never casts a shadow. Rows, notes, bars and the composer are flat on the paper. Only things that float over the page (the toast) or sit on the video (the wipe handle) may cast one.

## Shapes

Corners are small and plain. Controls, inputs, tooltips and key caps use 4px. The theme switch and the toast use 5px. The dialog uses 6px. A snapshot uses 2px. Swatches and the lock dot are round. The sheet itself has no corners at all: it is ruled paper that meets the edges.

Two triangle flags mark the playhead: a down-pointing caret on top of the timeline playhead line, and a right-pointing flag at the left edge of the playhead row. Both are cav orange.

Icons are drawn line icons on a 16px box with one 1.5px stroke, round caps and round joins, in currentColor. Note actions draw them at 15px and meta lines at 13px.

## Components

### Tool buttons
Quiet, flat and square-ish. They sit on the paper with no border.
- **Shape:** 4px corners, 30px high, at least 30px wide, 0 7px padding. A text button adds 10px side padding and weight 550.
- **Rest:** transparent, ink-2 icon.
- **Hover / Active:** a 6% ink wash and ink icon; rule-2 when pressed down. Colour transitions take 120ms ease-out.
- **Pressed or chosen** (aria-pressed or aria-checked): an ink fill with paper icon. This marks the drawing tool, the theme and the filter. A chosen swatch instead shows a 2px paper gap and a 1.5px ink ring.
- **Disabled:** graphite at 55% opacity.

### Add (the composer's action)
The one orange button. It sits in the playhead row, at the reading line. Cav orange fill, ink text, 650 weight, 12px, 28px high. Hover brightens it by 6%. Disabled turns to paper-3 with graphite text.

### Send to agent
The sheet's foot. An ink bar, 36px high, the full width of the foot, paper text at weight 650. Hover brightens it by 15%. Disabled turns to paper-3 with graphite text.

### Inputs and select
- **Reply input:** 28px high, a 1px rule-2 border, 4px corners, paper ground. Focus turns the border cav orange and drops the outline.
- **Composer:** a borderless textarea in the playhead row, 13px at line-height 1.42, growing with its content from 40px to 160px.
- **Select:** 30px high, a 1px rule-2 border, 4px corners, a drawn chevron. Hover darkens the border to rule-3.
- **Focus:** every other control shows a 2px cav orange outline with a 2px offset.

### Tooltips
Tooltips carry each control's label and its shortcut key, for example "Back one frame  ←" or "Add note  ⌘↵". They are ink pills with paper-2 text (the colours swap in dark), 4px corners, 4px 8px padding. They appear after 350ms on hover or keyboard focus and rise 3px over 120ms. They hide on devices without hover. Each one can sit above, below, or aligned to the start or end of its control.

### The sheet (signature component)
- **Head:** FRAME and NOTE column heads over a heavy rule, with the note count and the resolved-notes filter at the right.
- **Note row:** the frame cell (frame numeral, timecode under it, range end under a short rule) beside the note body (snapshot, words, meta line, actions). Hover adds a 4.5% ink wash. Actions fade in on hover, focus or selection, and always show without hover. A reply thread hangs off a rule-2 left line.
- **Live and selected:** the live note's frame numeral turns cav orange ink. The selected note takes the orange wash.
- **Playhead row:** the composer. Two orange rules above and below, an orange frame-column edge, an orange flag, and the frame numeral in orange ink. The sheet scrolls so this row stays at the reading line as frames step.
- **Run of empty frames:** a run label in graphite beside a strip of ruled paper (see Layout).

### Timeline strip
A ruled strip under the stage, 38px high, on paper. The audio waveform lies under the ticks at 20% graphite. Notes show as 9px ink marks along the top edge. A resolved mark is graphite at 45%, and the selected mark is cav orange. The playhead is a 2px orange line with a caret.

### Key caps
Inline key labels in the shortcuts dialog: paper-2 ground, a 1px rule-2 border with a 2px bottom edge, 4px corners, system sans 600 at 11px.

### Coarse pointers
Under `pointer: coarse`, every tool button, swatch, note action, the Add button, the reply input and the select grow to 40px targets. The theme switch buttons grow to 36px by 38px.

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
- **Don't** use decorative or tonal gradients. The hard-stop hairlines of the ruled paper are rules, not a gradient effect.
- **Don't** use orange for status, decoration, headings or the person's drawing strokes.
- **Don't** draw seconds rules into the sheet. Its runs of frames are on a square-root scale; time in seconds belongs to the timeline.
- **Don't** set sentences, buttons or tooltips in the Sheet face.
- **Don't** tint or theme the stage.
