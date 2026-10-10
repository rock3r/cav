---
name: cavalry
description: Build professional motion graphics in Cavalry (the 2D motion design app) from the shell with the `cav` CLI - logo stings, kinetic type, lower thirds, charts, UI walkthroughs, transitions, and animations synced to music beats. Use when the user asks to create, animate, edit, inspect or render anything in Cavalry, or mentions .cv scenes.
license: MIT
compatibility: Needs Cavalry 2.4+ (tested on 2.7.2 and 2.8.0) on macOS or Windows, the `cav` CLI, and ffmpeg. Cavalry must be running with the cav-bridge script open.
metadata:
  version: "1.3.0"
---

# Cav: motion graphics in Cavalry

The `cav` CLI does the work and carries its own guides. This skill only gets you started.

1. Run `cav doctor`.
   - `cav: command not found`: ask the user, then install it as `references/install.md` says.
   - `FAIL bridge`: ask the user to open Cavalry and click **Scripts > cav-bridge**, and to
     keep its window open. Wait, then run `cav doctor` again. Do not reach Cavalry any other
     way.
   - `blocked: a sandbox`: your sandbox stops `cav`. Show the user the fix that `cav doctor`
     prints (allow `cav` to reach 127.0.0.1:8723, or run `cav relay` outside the sandbox).
2. Run `cav guide` and follow it. It covers the workflow, script rules and a done checklist.
   Read `cav guide design` before a creative task and `cav guide music` when there is music
   (it covers `cav spectrogram` and `cav sync`, which let you look at music you cannot hear).
   `cav helpers` lists the script library; `cav help <command>` explains each command.
3. Work only in new scenes (`cav scene new`). Never use `--force` on the user's own work.
4. You cannot see Cavalry: look at `cav sheet` images after every change, use
   `cav onion <start-end>` to see how one move travels, and run `cav check` before you render.
   For storyboards, generated assets, music, sound effects, licences and a review page where
   the user leaves notes for you, read `cav guide production`.

5. Start diagnostics with `cav check --quick --json`. Treat performance findings as
   structural risks; add `--profile` for small measured samples. Inspect failed/skipped
   coverage before calling the scene clean. A native call can exceed the CLI wait budget.
6. After `run`, `check` or `render` times out, inspect the printed operation ID with
   `cav operation status <id>` and continue with `cav operation resume <id> --timeout 30m`.
   Do not retry the original command. Raw `cav job wait` completes only one bridge job,
   including a render's preparatory metadata. Keep the original scene/session and inputs
   unchanged during recovery; unknown outcomes require manual inspection.

7. Use `cav render` for final video output; do not wait only for an MP4 to grow. On exit 4
   (bridge disconnect or session change), stop automatic waiting, inspect the operation,
   and preserve partial artifacts. File size/age is not completion or proof of a crash.
   Reconcile an unknown outcome before abandoning and starting a new render; never
   silently relaunch the app, retry or switch to chunk rendering.
