---
version: 1
slug: "assets-review-index-html"
primary_target: "assets/review/index.html"
related_targets: []
---

# Surface brief: cav review page

Scope: `assets/review/index.html`, served by `cav review` in a browser and by `cav mcp` inside chat frames (with `assets/review/mcp-app.js`). Mode: Operate.

Audience and job: one motion designer, reviewing their own agent's renders in many quick passes. Most used: scrubbing and stepping frames, drawing on the frame, writing and managing notes. Compare is secondary. Wrong if: chrome competes with the video, or it looks like a generic SaaS app. Keyboard-first, fully mouse/trackpad usable, touch-friendly, WCAG AA in both themes. Theme follows the system by default, with a manual override.

## Direction contract

THESIS: The review is an animator's exposure sheet. Frames run down ruled rows; each note is written into the row of the frame it is about, and the sheet scrolls with the playhead. Refuses the dark NLE panel with a comment rail of rounded cards.

OWN-WORLD: Printed sheet stock (white in light, ink #16181f in dark) ruled in graphite hairlines, with heavier rules every second of frames; column heads set small in a condensed grotesk caps; tabular figures for every frame and timecode; one reserved cav orange for the playhead row, the live note and live drawing; neutral black stage. Icons are drawn line icons in one 1.5px stroke. No cards, no shadows on content, no gradients.

STORY: The designer sees the frame big, sees where every note sits in time on the sheet, steps to a frame, draws, types into that frame's row, and sends. Resolved notes recede to graphite.

FIRST VIEWPORT: Stage owns the left, black, at the video's aspect, with a thin transport bar and ruled timeline under it. To its right, the sheet: header row (FRAME, TC, NOTE), then a window of frame rows centred on the playhead, notes written in their rows with snapshot thumbs, open rows in ink, resolved in graphite. Composer sits as the playhead row of the sheet itself. Send to agent anchors the sheet's foot. Under 760px wide the sheet drops below the stage.

FORM: Exposure Sheet, position 1 on my ordered list (chosen as the pick over the roll), seed key 43a13dfc.

SIGNATURE INTERACTION: the sheet rides the playhead: as frames step, the ruled rows scroll so the current frame row stays at the reading line, flagged in orange.

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance
