#!/bin/bash
# Release smoke: run every cav command once against the running Cavalry, in a new
# scratch scene and a temporary project folder.
#
#   tools/release-smoke.sh <path to cav> [--keep] [--keyed]
#
# Needs Cavalry open with the cav-bridge window, ffmpeg and ffprobe, and python3.
# The open scene must have no unsaved changes: the smoke starts a new scene without
# --force, so it never throws work away.
#
# By default it uses only free paths and reads no key at all: it sets CAV_NO_KEYS=1, so
# cav treats every key as missing without asking the environment, the keychain or
# 1Password. Generators run with --service greybox or another free service. Steps marked
# "net" need the internet: they warn on a network error. A step marked "refuse=TEXT"
# must exit with an error whose output matches TEXT (it checks an error path).
#
# --keyed adds the services that need your keys. Each of those calls can cost money and
# can ask 1Password, so --keyed works only at a terminal: it lists the calls and asks
# before it runs them.
#
# The smoke fails when a step fails, or when `cav help` lists a command that has no
# step here. Add a step for every new command.
set -uo pipefail

[[ $# -ge 1 ]] || { echo "usage: $0 <path to cav> [--keep] [--keyed]" >&2; exit 64; }
cav="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
shift
keep= keyed=
for a in "$@"; do
	case $a in
	--keep) keep=--keep ;;
	--keyed) keyed=1 ;;
	*) echo "unknown option $a" >&2; exit 64 ;;
	esac
done
if [[ -n "$keyed" ]] && ! [[ -t 0 && -t 1 ]]; then
	echo "--keyed needs a terminal: its calls can cost money and ask 1Password" >&2
	exit 64
fi
export CAV_NO_KEYS=1
[[ -x "$cav" ]] || { echo "not executable: $cav" >&2; exit 64; }
for tool in ffmpeg ffprobe python3; do
	command -v "$tool" >/dev/null || { echo "missing $tool" >&2; exit 69; }
done

work="$(mktemp -d "${TMPDIR:-/tmp}/cav-smoke.XXXXXX")"
logs="$work/logs"
mkdir -p "$logs" "$work/.cav"
cd "$work" || exit 70
# On exit, replace the smoke's own scratch scene with an empty one, so the next run (and the
# user) does not find a scene with unsaved changes. A scene that is not the smoke's is left
# alone. --keep keeps the files in $work, not the open scene.
scratch_scene=
finish() {
	# Before "scene save", the smoke's scene is untitled; after it, it is smoke.cv in $work.
	if [[ -n "$scratch_scene" ]] && "$cav" scene info 2>/dev/null | head -1 | grep -qE "\(untitled\)|/$(basename "$work")/"; then
		"$cav" scene new --force >/dev/null 2>&1 || echo "note: could not close the smoke's scratch scene; close it in Cavalry before the next run"
	fi
	[[ "$keep" == --keep ]] || rm -rf "$work"
}
trap finish EXIT

pass=0 fail=0 warn=0 n=0
net_errors='deadline exceeded|: EOF|timed? ?out|timeout|no such host|connection (refused|reset)|network is unreachable|dial tcp|tls|certificate|HTTP 5[0-9][0-9]|answered HTTP (429|5)|temporary failure|could not resolve|failed to fetch|failed to download'
covered=" "
failed=()

# step KIND NAME cav-args...
# KIND: ok (exit 0), net (exit 0, or a warning on a network error), or refuse=TEXT (a
# non-zero exit whose output matches the extended regular expression TEXT).
step() {
	local kind=$1 name=$2 expect=
	if [[ "$kind" == refuse=* ]]; then
		expect=${kind#refuse=}
		kind=refuse
	fi
	shift 2
	covered+="$1 "
	n=$((n + 1))
	local log
	log="$logs/$(printf '%02d' "$n")-${name// /_}.log"
	local start=$SECONDS rc
	"$cav" "$@" >"$log" 2>&1
	rc=$?
	local took=$((SECONDS - start))
	local verdict
	case "$kind:$rc" in
	ok:0 | net:0) verdict=pass ;;
	refuse:0) verdict=fail ;;
	refuse:*)
		if grep -qE "$expect" "$log"; then verdict=pass; else verdict=fail; fi
		;;
	net:*)
		# Only a network error is a warning; any other failure of a net step fails.
		if grep -qiE "$net_errors" "$log"; then verdict=warn; else verdict=fail; fi
		;;
	*) verdict=fail ;;
	esac
	case $verdict in
	pass) pass=$((pass + 1)); printf 'ok    %-34s %3ss\n' "$name" "$took" ;;
	warn) warn=$((warn + 1)); printf 'warn  %-34s %3ss  exit %s (needs the internet): %s\n' "$name" "$took" "$rc" "$log" ;;
	fail)
		fail=$((fail + 1))
		failed+=("$name")
		printf 'FAIL  %-34s %3ss  exit %s: %s\n' "$name" "$took" "$rc" "$log"
		tail -5 "$log" | sed 's/^/      /'
		;;
	esac
	return 0
}

# check NAME CONDITION... : a plain shell assertion about a step's output.
check() {
	local name=$1
	shift
	n=$((n + 1))
	if "$@" >/dev/null 2>&1; then
		pass=$((pass + 1))
		printf 'ok    %-34s\n' "$name"
	else
		fail=$((fail + 1))
		failed+=("$name")
		printf 'FAIL  %-34s\n' "$name"
	fi
}

json() { python3 -I -c 'import json,sys; d=json.load(open(sys.argv[1])); print(eval(sys.argv[2], {"d": d}))' "$@"; }
lastlog() { ls "$logs"/[0-9]*.log | tail -1; }

echo "cav: $("$cav" version | head -1)"
echo "work: $work"
echo

# --- Set-up and state ---------------------------------------------------------------
step ok "version" version
step net "version --check" version --check
step ok "setup" setup --no-docs
step ok "doctor" doctor
step ok "status" status
step ok "config" config
step ok "config check greybox" config check greybox

# --- Scene --------------------------------------------------------------------------
step ok "scene new" scene new --width 640 --height 360 --fps 30 --seconds 4 --bg '#101014'
if ! grep -q '640x360' "$(lastlog)"; then
	echo
	echo "cav scene new did not start a scratch scene. Save or close the open scene, then run again."
	exit 1
fi
scratch_scene=1
step ok "scene comp" scene comp --motion-blur
step ok "scene info" scene info
cat >build.js <<'EOF'
var g = cav.group('Card')
var r = cav.rect('Box', 160, 100, {parent: g, fill: '#3366ff', radius: 12})
cav.key(r, 'x', [[0, -200], [45, 200, 'SlowOut']])
cav.key(r, 'fill', [[0, undefined], [45, '#ff6633']])
var t = cav.text('Title', 'Smoke', 48, {y: 120})
cav.key(t, 'opacity', [[0, 0], [15, 100], [60, 100], [61, 0]])
var dot = cav.circle('Dot', 20, {x: 0, y: -120, fill: '#ffffff'})
cav.key(dot, 'scale', [[30, 1], [32, 1.6], [40, 1]])
return api.getCompLayers(false).length
EOF
step ok "run file" run build.js
step ok "run -e" run -e 'return cav.comp().width'
echo 'return 1 + 1' >stdin.js
covered+="run "
n=$((n + 1))
if out="$("$cav" run - <stdin.js 2>"$logs/stdin.err")" && [[ "$out" == 2 ]]; then
	pass=$((pass + 1)); printf 'ok    %-34s\n' "run - (stdin)"
else
	fail=$((fail + 1)); failed+=("run - (stdin)"); printf 'FAIL  %-34s %s\n' "run - (stdin)" "$out"
fi
step ok "run --async" run --async -e 'return 7' --json
async="$(lastlog)"
job="$(json "$async" 'd["job"]' 2>/dev/null)"
op="$(json "$async" 'd["operation"]' 2>/dev/null)"
step ok "job wait" job wait "$job" --timeout 2m
step ok "job status" job status "$job"
step ok "operation resume" operation resume "$op" --timeout 2m
step ok "operation status" operation status "$op"
step ok "scene save" scene save --timeout 2m smoke.cv
check "scene saved to smoke.cv" test -s smoke.cv
step refuse="flag provided but not defined: -bogus" "scene save: unknown flag" scene save --bogus smoke.cv
step ok "scene open" scene open smoke.cv --timeout 5m
step ok "tree" tree --depth 2
box="$("$cav" tree --json 2>/dev/null | python3 -I -c 'import json,sys
def walk(ls):
    for l in ls:
        yield l
        yield from walk(l.get("children", []))
print(next(l["id"] for l in walk(json.load(sys.stdin)["layers"]) if l["name"] == "Box"))')"
step ok "layer" layer "$box" --attrs --frame 20
step ok "types" types rect
step ok "helpers" helpers key

# --- Looking at the scene -----------------------------------------------------------
step ok "frame" frame 0 30 --scale 25 -o frames
check "frame wrote PNGs" test -s frames/f30.png -o -n "$(ls frames/*.png 2>/dev/null)"
step ok "sheet" sheet --count 4 --scale 20 -o sheet.png
check "sheet.png exists" test -s sheet.png
step ok "onion" onion 0-45 --count 4 --scale 25 -o onion.png
check "onion.png exists" test -s onion.png
step ok "frames" frames 0-20 --keep-every 10 -o sim
step ok "check --quick" check --quick
step ok "check" check --timeout 10m
step ok "render" render -o renders/smoke.mp4 --scale 50 --timeout 15m
check "render has 120 frames" test "$(ffprobe -v error -count_frames -select_streams v:0 -show_entries stream=nb_read_frames -of csv=p=0 renders/smoke.mp4)" = 120
step ok "seams --video" seams --video renders/smoke.mp4
step ok "seams" seams --max-boundaries 4

# --- Audio and music ----------------------------------------------------------------
# A 120 BPM click track: a decaying 60 Hz thump every half second.
ffmpeg -v error -y -f lavfi -i "aevalsrc='0.8*sin(2*PI*60*t)*exp(-30*mod(t,0.5))':s=48000:d=4" -c:a pcm_s16le click.wav
step ok "beats" beats click.wav --fps 30 -o grid.json
check "beats found about 120 BPM" python3 -I -c 'import json,sys; b=json.load(open("grid.json")); bpm=b.get("bpm") or b.get("tempo"); sys.exit(0 if bpm and 110 <= float(bpm) <= 130 else 1)'
step ok "beats --bpm" beats --bpm 120 --seconds 4 --fps 30
step ok "spectrogram" spectrogram click.wav --fps 30 -o spectrogram.png
step ok "render --audio" render -o renders/smoke-audio.mp4 --audio click.wav --scale 50 --timeout 15m
step ok "sync" sync renders/smoke-audio.mp4 -o sync.png
step ok "listen --no-ears" listen click.wav --no-ears --seconds 4 --fps 30 -o listen.png
cat >score.json <<'EOF'
{"bpm": 120, "beats": 8,
 "tracks": [
  {"name": "kick", "instrument": "kick", "hits": [0, 1, 2, 3, 4, 5, 6, 7]},
  {"name": "bass", "instrument": "bass", "notes": [[0, 2, 45, 0.8], [4, 2, 43, 0.8]], "duck": true}],
 "master": {"lufs": -16}}
EOF
step net "music render" music render score.json -o music/score.wav
printf '[Verse]\nla la\n' >song.txt
step refuse="lyria makes only instrumentals" "music gen: lyrics on lyria" music gen "test" --lyrics song.txt --service lyria
step refuse="elevenlabs: no key: CAV_NO_KEYS is set" "music gen: no key" music gen "test" --seconds 5 --service elevenlabs
step refuse="elevenlabs: no key: CAV_NO_KEYS is set" "sfx gen: no key" sfx gen "a click" --seconds 1

# --- Storyboard, assets, credits ----------------------------------------------------
step ok "board init" board init --bpm 120 --fps 30 --seconds 4 --shots 2 -o storyboard.json
step ok "board frames (greybox)" board frames storyboard.json --service greybox
step ok "board sheet" board sheet storyboard.json -o renders/board.png
step ok "board animatic" board animatic storyboard.json --audio click.wav -o renders/animatic.mp4
step ok "board place" board place storyboard.json
step refuse="name the shots to animate" "board motion: no shots named" board motion storyboard.json
step ok "gen image (greybox)" gen image "a smoke test card" --service greybox -o assets/card.png
# gen vector traces with vtracer or potrace (recraft needs a key). Install one, for example
# `brew install potrace`, so the smoke covers it; without both it is a skip, not a pass.
# The tracer is named, so a custom image.vector order in services.json does not matter.
tracer=
command -v potrace >/dev/null && tracer=potrace
command -v vtracer >/dev/null && tracer=vtracer
if [[ -n "$tracer" ]]; then
	step ok "gen vector ($tracer)" gen vector assets/card.png --service "$tracer" -o assets/card.svg
	check "gen vector wrote an SVG with a path" grep -q "<path" assets/card.svg
	step ok "SVG imports into Cavalry" run -e "var ids = api.convertSVGToLayers('$work/assets/card.svg'); if (!ids.length) throw new Error('no layers'); return ids.length"
else
	covered+="gen "
	echo "skip  gen vector                         neither vtracer nor potrace is installed (brew install potrace)"
fi
# gen cutout makes the card's background transparent with rembg through uv (recraft
# needs a key). The first run downloads the BiRefNet model (about 1 GB) into ~/.rembg.
# Without uv it is a skip, not a pass.
if command -v uv >/dev/null; then
	step net "gen cutout (rembg)" gen cutout assets/card.png --service rembg -o assets/card-cutout.png
	# Byte 25 of a PNG is its colour type: 6 is RGBA, so the file has an alpha channel.
	# A network warning above leaves no file, and the step already reported it.
	[[ -f assets/card-cutout.png ]] && check "gen cutout wrote an RGBA PNG" python3 -I -c 'import sys; d = open(sys.argv[1], "rb").read(26); sys.exit(d[1:4] != b"PNG" or d[25] != 6)' assets/card-cutout.png
else
	echo "skip  gen cutout                         uv is not installed (https://docs.astral.sh/uv/)"
fi
step ok "sfx add" sfx add click.wav --licence CC0-1.0 --author smoke
step ok "sfx index" sfx index . --licence CC0-1.0
step ok "sfx libraries" sfx libraries
step net "sfx search (openverse)" sfx search whoosh --service openverse --limit 2
mkdir -p moodboard && cp assets/card.png moodboard/
step ok "ref add" ref add moodboard/card.png --licence CC0-1.0 --author smoke
step net "ref search (openverse)" ref search "paper texture" --service openverse --limit 2
step ok "board mood" board mood moodboard -o renders/moodboard.png
step ok "credits" credits --all
step ok "credits list" credits list
step ok "credits licence" credits licence nc-ok
step ok "credits check" credits check
step ok "score" score storyboard.json --video renders/smoke.mp4 --audio click.wav -o score/project.rpp
check "score wrote a REAPER project" grep -q REAPER_PROJECT score/project.rpp

# --- Review, MCP, relay -------------------------------------------------------------
covered+="review "
port=$((20000 + RANDOM % 20000))
"$cav" review renders/smoke.mp4 --port "$port" >"$logs/review-serve.log" 2>&1 &
review_pid=$!
# review moves to the next free port when this one is taken: read the URL it prints.
up=
for ((i = 0; i < 60; i++)); do
	url="$(sed -n 's/^review: \(http:[^ ]*\)$/\1/p' "$logs/review-serve.log" | head -1)"
	if [[ -n "$url" ]] && curl -fsS "${url}api/state" -o review-state.json 2>/dev/null; then up=1; break; fi
	sleep 1
done
kill "$review_pid" 2>/dev/null; wait "$review_pid" 2>/dev/null
check "review serves its page" test -n "$up"
step ok "review export" review export renders/smoke.mp4
covered+="mcp "
n=$((n + 1))
mcp_out="$( (printf '%s\n' \
	'{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke","version":"1"}}}' \
	'{"jsonrpc":"2.0","method":"notifications/initialized"}' \
	'{"jsonrpc":"2.0","id":2,"method":"tools/list"}'; sleep 2) | "$cav" mcp 2>"$logs/mcp.err")"
if grep -q show_review <<<"$mcp_out" && grep -q review_notes <<<"$mcp_out"; then
	pass=$((pass + 1)); printf 'ok    %-34s\n' "mcp tools/list"
else
	fail=$((fail + 1)); failed+=("mcp tools/list"); printf 'FAIL  %-34s\n' "mcp tools/list"
fi
covered+="relay "
mkdir -p spool
CAV_SPOOL="$work/spool" "$cav" run -e 'return "via relay"' --timeout 1m >"$logs/relay-run.log" 2>&1 &
relay_run=$!
sleep 1
step ok "relay --once" relay --spool "$work/spool" --once
wait "$relay_run"
check "a spooled run went through the relay" grep -q 'via relay' "$logs/relay-run.log"

# --- Docs and guides ----------------------------------------------------------------
step ok "api" api keyframe -n 3
step ok "docs" docs duplicator -n 2
for topic in workflow design music traps native production; do
	step ok "guide $topic" guide "$topic"
done
step ok "guide --search" guide --search "keys dropped" -n 2
check "guide --search finds the rotation trap" grep -q "rotation" "$(lastlog)"
step refuse="flag needs an argument: -search" "guide --search: no words" guide --search

# --- Update (never installs here) ---------------------------------------------------
# With --json and without --yes, cav update refuses to install; when cav is current it
# only says so.
covered+="update "
n=$((n + 1))
"$cav" update --json >"$logs/update.json" 2>&1
if grep -q '"updated": false\|needs confirmation' "$logs/update.json"; then
	pass=$((pass + 1)); printf 'ok    %-34s\n' "update --json (installs nothing)"
elif grep -qiE "$net_errors" "$logs/update.json"; then
	warn=$((warn + 1)); printf 'warn  %-34s (needs the internet): %s\n' "update --json" "$logs/update.json"
else
	fail=$((fail + 1)); failed+=("update --json"); printf 'FAIL  %-34s %s\n' "update --json" "$logs/update.json"
fi

# --- Keyed services (opt-in, at a terminal) ------------------------------------------
if [[ -n "$keyed" ]]; then
	echo
	echo "--keyed: these calls use your configured services and keys. They can cost money"
	echo "and can ask 1Password:"
	echo "  cav config check                one free call per service with a key"
	echo "  cav gen image                   one still (a few cents)"
	echo "  cav gen image --alpha           one transparent still (a few cents)"
	echo "  cav gen cutout, gen vector      with a Recraft key: one cutout and one trace"
	echo "                                  (10 Recraft credits each, about 1 cent)"
	echo "  cav sfx gen --seconds 1         one sound effect"
	echo "  cav sfx search, sfx get         with a Freesound key: one search, one preview (free)"
	echo "  cav music gen --seconds 10      one short track"
	echo "  cav listen (with ears)          one audio critique, and one more with qwen-omni"
	echo "                                  when its server is set (under 1 cent on OpenRouter)"
	read -r -p "Run them? [y/N] " answer
	if [[ "$answer" == [yY]* ]]; then
		unset CAV_NO_KEYS
		step ok "keyed: config check" config check
		checks="$(lastlog)"
		# usable SERVICE: config check said "ok" for it, so its key or server works.
		usable() { grep -qE "^ok +$1 " "$checks"; }
		# rgba FILE: byte 25 of a PNG is its colour type, and 6 is RGBA.
		rgba() { python3 -I -c 'import sys; d = open(sys.argv[1], "rb").read(26); sys.exit(d[1:4] != b"PNG" or d[25] != 6)' "$1"; }
		step ok "keyed: gen image" gen image "a smoke test card, flat colours" -o assets/keyed.png
		step ok "keyed: gen image --alpha" gen image "a flat orange fox head logo on a plain grey background" --alpha -o assets/keyed-alpha.png
		[[ -f assets/keyed-alpha.png ]] && check "keyed: --alpha wrote an RGBA PNG" rgba assets/keyed-alpha.png
		if usable recraft && [[ -f assets/keyed.png ]]; then
			step ok "keyed: gen cutout (recraft)" gen cutout assets/keyed.png --service recraft -o assets/keyed-cutout.png
			[[ -f assets/keyed-cutout.png ]] && check "keyed: recraft cutout is an RGBA PNG" rgba assets/keyed-cutout.png
			step ok "keyed: gen vector (recraft)" gen vector assets/keyed.png --service recraft -o assets/keyed.svg
			check "keyed: recraft trace has a path" grep -q "<path" assets/keyed.svg
		else
			echo "skip  keyed: recraft cutout and trace    no working Recraft key, or no keyed.png"
		fi
		step ok "keyed: sfx gen" sfx gen "a short soft click" --seconds 1
		if usable freesound; then
			step ok "keyed: sfx search (freesound)" sfx search whoosh --service freesound --max-seconds 3 --limit 1 --json
			fsref="$(json "$(lastlog)" 'd["results"][0]["service"] + ":" + d["results"][0]["id"]' 2>/dev/null)"
			step ok "keyed: sfx get (freesound preview)" sfx get "$fsref" -o sfx-keyed
			check "keyed: sfx get saved an MP3 preview" test -n "$(ls sfx-keyed/freesound-*.mp3 2>/dev/null)"
		else
			echo "skip  keyed: freesound search and get   no working Freesound key"
		fi
		step ok "keyed: music gen" music gen "calm piano, ten seconds" --seconds 10 -o music
		step ok "keyed: listen with ears" listen click.wav --seconds 4 --fps 30 -o listen-ears.png
		if usable qwen-omni; then
			step ok "keyed: listen (qwen-omni)" listen click.wav --ears qwen-omni --seconds 4 --fps 30 -o listen-qwen.png
		else
			echo "skip  keyed: listen (qwen-omni)          no qwen-omni server set (cav config endpoint qwen-omni)"
		fi
		export CAV_NO_KEYS=1
	else
		echo "keyed services skipped"
	fi
fi

# --- Coverage -----------------------------------------------------------------------
missing=()
commands="$("$cav" help | awk '/^Commands:/{on=1; next} on && /^  [a-z]/{print $1} on && /^$/{exit}')"
if (($(wc -w <<<"$commands") < 30)); then
	echo
	echo "FAIL  could not read the command list from cav help, so coverage is unknown"
	fail=$((fail + 1))
fi
for c in $commands; do
	[[ "$covered" == *" $c "* ]] || missing+=("$c")
done
echo
if ((${#missing[@]})); then
	echo "FAIL  commands with no smoke step: ${missing[*]}"
	fail=$((fail + 1))
fi
echo "$pass passed, $warn warnings, $fail failed (logs: $logs)"
[[ "$keep" == --keep ]] || echo "run with --keep to keep $work"
if ((fail)); then
	printf 'failed: %s\n' "${failed[@]}"
	exit 1
fi
