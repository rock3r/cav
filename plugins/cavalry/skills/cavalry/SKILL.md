---
name: cavalry
description: Build motion graphics in Cavalry (the 2D motion design app) from the shell with the `cav` CLI - logo stings, kinetic type, lower thirds, charts, UI walkthroughs, transitions, and animations synced to music beats. Use when the user asks to create, animate, edit, inspect or render anything in Cavalry, or mentions .cv scenes.
license: MIT
compatibility: Needs Cavalry 2.4+ (tested on 2.7.2 and 2.8.0) on macOS or Windows, the `cav` CLI, and ffmpeg. Cavalry must be running with the cav-bridge script open.
metadata:
  version: "1.8.0"
---

# Cav: motion graphics in Cavalry

Use the `cav` CLI for commands and guidance. Follow these steps.

1. Run `cav doctor`.
   - If the shell reports `cav: command not found`, ask the user before installing the CLI.
     Follow `references/install.md` for installation.
   - If it reports `FAIL bridge`, ask the user to open Cavalry and select
     **Scripts > cav-bridge**. The bridge window must stay open. Then run `cav doctor` again.
   - If it reports `blocked: a sandbox`, show the user the printed fix. The user can allow
     access to 127.0.0.1:8723 or run `cav relay` outside the sandbox.
   - If it reports `warn update`, explain that `cav update` installs the newer version.
     Ask before running the update.
2. Run `cav guide`. Follow its build loop, script rules and completion checks.
   For work longer than a quick sting, first read `cav guide flow`. It lists the brief,
   mood board, storyboard, music and animatic stages. A project `flow.md` or user
   `~/.cav/flow.md` replaces the standard stages. `cav guide flow` prints the active file.
   Follow that file. `cav plan` reports completed stages and the next step.
   Read `cav guide design` before creative work. When the project uses music, read
   `cav guide music`. It explains `cav spectrogram` and `cav sync` for visual audio checks.
   Use `cav helpers` to list scripts and `cav help <command>` for command details.
   Search by symptom when a command has no visible effect or gives a wrong result.
   For example, run `cav guide --search "rotation keys ignored"`.
   For app screens, UI walkthroughs and product demos, build the UI from layers.
   Read `cav guide ui-showcase` first. Use a screenshot to check the result, as
   `cav guide design` recipe 12 describes.
3. Create work in a new scene with `cav scene new`. Do not use `--force` on the user's own scene.
4. Inspect `cav sheet` after each change. Use `cav onion <start-end>` to inspect a move
   across frames. Run `cav check` before rendering. Read `cav guide production` for
   storyboards, generated assets, music, sound effects, licences and user review notes.
   `cav music gen` makes instrumentals. Add `--lyrics` with yue2 or acestep for vocals.

5. Start diagnostics with `cav check --quick --json`. Treat performance findings as
   structural risks. Add `--profile` for a small measured sample. Inspect failures and
   skipped checks before reporting a clean scene. A native call can exceed the CLI budget.
6. When `run`, `check` or `render` times out, inspect its operation ID with
   `cav operation status <id>`. Continue with `cav operation resume <id> --timeout 30m`.
   Wait for that operation. Do not submit the original command again. Raw `cav job wait`
   completes one bridge job, including a render's preparatory metadata.
   Keep the original scene, session and inputs unchanged during recovery.
   Inspect outcomes that remain unknown.

7. Use `cav render` for final video output. Wait on its operation ID. On exit 4
   (bridge disconnect or session change), stop automatic waiting and inspect the operation.
   Keep partial artifacts. File size and age cannot prove completion or a crash.
   Reconcile an unknown outcome before abandoning it and starting a new render.
   Do not silently relaunch Cavalry, retry the command or start chunk rendering.
