# Installing and updating `cav`

Ask the user before you install anything.

## Install

macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/rock3r/cavalry-skill/main/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/rock3r/cavalry-skill/main/install.ps1 | iex
```

The installer puts `cav` in `~/.local/bin` (macOS) or `%LOCALAPPDATA%\cav\bin` (Windows), then
runs `cav setup`. The same scripts are in this skill's `scripts/` folder.

`cav setup` is safe to run again at any time. It:

1. creates the bridge token in `~/.cav/token`;
2. installs `cav-bridge.js` into Cavalry's Scripts folder;
3. checks Cavalry and ffmpeg (install ffmpeg with `brew install ffmpeg` or `winget install Gyan.FFmpeg`);
4. builds the offline docs index (`cav docs update`);
5. checks the running bridge.

Then the user opens Cavalry and clicks **Scripts > cav-bridge** once per Cavalry session. The
first time, Cavalry asks whether to trust the script; the user must click Yes.

## Update

```bash
cav version --check     # is there a newer release?
cav update              # shows the change and asks before installing
```

After an update, `cav doctor` may say the running bridge is older than the installed one. Then
the user closes the cav-bridge window in Cavalry and starts it again from the Scripts menu.

## Agents in a sandbox

If the agent's sandbox blocks 127.0.0.1, run `cav relay --spool <dir>` outside the sandbox and
set `CAV_SPOOL=<dir>` for the agent. See `cav help relay`.
