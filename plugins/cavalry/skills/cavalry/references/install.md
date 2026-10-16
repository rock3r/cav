# Installing and updating `cav`

Ask the user before installing software.

## Install

macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/rock3r/cav/main/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/rock3r/cav/main/install.ps1 | iex
```

The installer places `cav` in `~/.local/bin` on macOS or `%LOCALAPPDATA%\cav\bin` on
Windows. It then runs `cav setup`. Copies of the installer scripts are in this skill's
`scripts/` folder.

Rerunning `cav setup` performs these steps:

1. creates the bridge token in `~/.cav/token`;
2. installs `cav-bridge.js` into Cavalry's Scripts folder;
3. checks Cavalry and ffmpeg (install ffmpeg with `brew install ffmpeg` or `winget install Gyan.FFmpeg`);
4. builds the offline docs index (`cav docs update`);
5. checks the running bridge.

For each Cavalry session, the user opens **Scripts > cav-bridge**. On the first run,
Cavalry asks whether to trust the script. The user must click Yes.

## Update

```bash
cav version --check     # is there a newer release? (cav doctor also says so)
cav update              # shows the change and asks before installing
```

After an update, `cav doctor` can report an older running bridge. The user must close
its window in Cavalry and reopen it from the Scripts menu.

## A slow bridge

`cav doctor` can report "listening but not answering". Cavalry uses one main thread for
scripts, renders and dialogs, so these can delay a response. cav waits up to 30 s for
each answer. `cav job wait` continues waiting for a running job. If Cavalry shows a
dialog, ask the user to close it. Evaluations saw some delays without a visible cause;
an 11-minute idle test did not reproduce them. Historical retries did not provide
command recovery. For current run, check and render operations, use
`cav operation status` and `cav operation resume`. Wait until an unknown outcome is
resolved before submitting another build or render.

## Agents in a sandbox

If the agent's sandbox blocks 127.0.0.1, start `cav relay --spool <dir>` outside it.
Set `CAV_SPOOL=<dir>` for the agent. See `cav help relay`.

## macOS bundle migration

Current macOS releases contain a Developer ID signed, notarized and stapled Cav.app.
The installer keeps the full app in `.cav-bundles` under the bin folder. It creates a
`cav` symlink for terminal use. Current self-update also keeps the bundle. An older
installed updater cannot preserve the new archive, so run the current installer once
to migrate. Keep the full bundle and its security checks. Do not extract only
`Contents/MacOS/cav`, remove quarantine, repair signatures or use ad-hoc signing.
