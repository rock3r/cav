# Install or update the cav CLI on Windows, then run `cav setup`.
#   irm https://raw.githubusercontent.com/rock3r/cavalry-skill/main/install.ps1 | iex
# Options (environment): CAV_VERSION=v0.1.0, CAV_BIN_DIR, CAV_REPO=owner/name, CAV_BASE_URL (mirror).
$ErrorActionPreference = 'Stop'

$Repo = if ($env:CAV_REPO) { $env:CAV_REPO } else { 'rock3r/cavalry-skill' }
$BinDir = if ($env:CAV_BIN_DIR) { $env:CAV_BIN_DIR } else { Join-Path $env:LOCALAPPDATA 'cav\bin' }
$Version = if ($env:CAV_VERSION) { $env:CAV_VERSION } else { 'latest' }

$arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq 'Arm64') { 'arm64' } else { 'amd64' }
$asset = "cav_windows_$arch.zip"
$base = if ($env:CAV_BASE_URL) { $env:CAV_BASE_URL } elseif ($Version -eq 'latest') { "https://github.com/$Repo/releases/latest/download" } else { "https://github.com/$Repo/releases/download/$Version" }

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("cav-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    Write-Host "cav: downloading $asset ($Version) from $Repo"
    Invoke-WebRequest "$base/$asset" -OutFile (Join-Path $tmp $asset) -UseBasicParsing
    Invoke-WebRequest "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt') -UseBasicParsing
    $want = (Get-Content (Join-Path $tmp 'checksums.txt') | Where-Object { $_ -match " $([regex]::Escape($asset))$" } | ForEach-Object { ($_ -split '\s+')[0] })
    $got = (Get-FileHash (Join-Path $tmp $asset) -Algorithm SHA256).Hash.ToLower()
    if (-not $want -or $want -ne $got) { throw "cav: checksum mismatch for $asset; not installed" }
    Expand-Archive (Join-Path $tmp $asset) -DestinationPath $tmp -Force
    New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
    $dest = Join-Path $BinDir 'cav.exe'
    if (Test-Path $dest) { Move-Item $dest "$dest.old" -Force }
    Copy-Item (Join-Path $tmp 'cav.exe') $dest
    Write-Host "cav: installed $dest"
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (($userPath -split ';') -notcontains $BinDir) {
        [Environment]::SetEnvironmentVariable('Path', "$userPath;$BinDir", 'User')
        Write-Host "cav: added $BinDir to your user PATH (open a new terminal)"
    }
    & $dest setup
    Write-Host "cav: next, open Cavalry and click Scripts > cav-bridge (keep its window open), then run: cav doctor"
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
