# cav 1.1 validation queue

PR #3 contains the merged offline improvements. Native validation uses Cavalry 2.8.0
on macOS arm64 with a separate 1.1.0 CLI and bridge. The team saved and backed up the
finished promo. Scene tests use a disposable copy and separate outputs. The installed
older CLI remains in place. Publication requires final signing and artifact checks.

## Native evidence (2026-10-04)

- Core live helpers passed 65/65 checks, and native features passed 14/14. The first
  short scratch timeline clamped an out-of-range sample. A 180-frame scratch passed.
- Quick diagnostics kept the scene path, active comp, frame and saved-state flag
  on both the scratch and promo copy. A two-entry membership scan reported explicit
  incomplete coverage and did not claim a clean result. The default 1,000-layer cap
  also explicitly reported the 14 omitted poster layers. Raising the cap to 2,000
  included all three poster duplicators (`r1_d`, `r2_d`, `r3_d`) and their per-copy
  JavaScript connections under comp path `[compNode#1, compNode#5]`, with no membership
  or layer omissions. Quick mode still truthfully omits visual and blank-run inspection.
- Short scene-open, frame, sheet, run and render waits resumed the same operations.
  Each recorded native opening/image/execution/render phase ran once. Competing resume
  clients were serialized by the operation lock. The mutation produced one layer;
  the video contained exactly 30 frames at 30 fps and an AAC audio stream.
- Reposting a completed job ID returned its stored result and left a native
  counter at one. A real bridge restart changed its session. Resuming a **synthetic,
  never-submitted** accepted checkpoint bound to the old session returned exit 4 with
  the session-change reason. That checkpoint was explicitly reconciled and abandoned;
  no real native call was lost for this test.
- The frame-export script failed at an unwritable PNG destination. It reported the
  failure and restored the original playhead at frame 7.
- Promo profiling rendered all 1,906 intervening warm-up frames and measured frames 840, 1560
  and 1908. Warm-up took 124,268 ms; sample PNG calls took 53, 45 and 84 ms, separately
  from setFrame calls (3, 4 and 4 ms). Frame zero was restored. All three images were
  inspected. These are timings from one API run. They do not assign whole-render cost
  to any component or establish a benchmark.
- The first full run found a native progress bug: `api.writeToFile` defaults to
  refusing existing files, so the progress JSON stopped at the first checkpoint while
  rendering continued. The fix passes explicit overwrite and records false-return
  failures once while retrying subsequent checkpoints. Fixtures also cover permanent
  refusal and recovery after a transient refusal. Regression fixtures model the real overwrite behavior. The corrected native
  run published advancing checkpoints (690 and 1,320 warm-up frames, then 1,906 and all
  three samples with restoration). Its warm-up took 120,157 ms; PNG samples took 51,
  46 and 86 ms. The three image hashes matched the initial run exactly.

An initial helper call ran after scratch-scene creation was refused. It changed the
owner's in-memory scene. The saved file and backups remained intact. The changed
memory state was saved separately. Later mutations require a confirmed disposable
scene. The owner's saved-file SHA-256 remains
`4119a9ef0607bf2a63cfef081f4546355d4fdedc9780d5284ffabcd1ff6cdc7b`.
The owner's saved scene was reopened at frame zero in `compNode#1`, with all 940 root
comp layers and `unsaved: false`. Its saved-file hash is unchanged, and no native jobs
remain pending. The intentional PNG failure retains its native failure result.

## Offline evidence

- Go race fixtures cover timeouts during native scene opening and later metadata reads.
  They also cover changed-input rejection, two competing resumes and frame/sheet publication
  without rerendering. Recovery is tested after the publication checkpoint and sheet frame cleanup.
- JavaScript fixtures cover nested and shared pre-comp references, scan/layer/composition
  limits, membership failures and cycles without frame changes or bounds/render calls.
- Profile fixtures cover later target frames, chronological PNG evaluation of every
  intervening frame, separate warm-up/sample timing, budget exhaustion, failed warm-up,
  changed scene identity, and playhead restoration.
- `go vet ./...`, Go race tests and all 35 Node fixtures passed. Development archives
  build without credentials for macOS arm64/amd64, Windows arm64/amd64 and Linux amd64.
- The signed macOS pipeline from PR #2 has a successful
  [notarized artifact smoke run](https://github.com/rock3r/cav/actions/runs/37152868668).
  That run checks the pipeline at its recorded revision. It does not check final 1.1 artifacts.

## Native validation checklist

Build a CLI from the final branch revision with a separate validation version and
output directory. Keep the owner's final scene and test a disposable copy. Record the
scene digest, original playhead and comp, CLI and bridge versions, and bridge session
before and after. Assign one owner to the live bridge during these checks.

1. Save the finished promo scene before leaving it. Preserve copies and baseline images.
   Run bridge preflight and inspect existing operations before submitting any native work.
   Install/restart the updated bridge only now, after the promo session is finished;
   verify the new bridge's session ID and direct duplicate-ID behavior on a scratch scene.
2. Open a small scene, then the promo copy with `scene open --timeout 15m`. Repeat with
   a deliberately short budget and resume the same operation through the follow-up query.
   Count submitted job IDs: the native open must appear once. Record wall-clock time
   until scene-info reports readiness. The open call can return before then.
3. Run `check --quick --json` on the promo copy. Confirm `s09_poster` drivers/duplicators
   appear with the correct `compPath`. Verify scene, active comp and playhead are unchanged.
   Exercise a small scan limit and confirm explicit incomplete coverage.
4. Run `check --profile-frames 840,1560,1908 --max-warmup 2000 --timeout 30m --json` on
   a freshly opened promo copy. For a start frame of zero, expect 1,906 intervening
   warm-up frames plus three measured PNG samples. Verify partial warm-up progress via
   operation status, independent sample timings, and original playhead restoration.
   Save the three images and review them for valid simulation state.
5. On a scratch scene, force a native PNG failure and confirm frame/sheet restore the
   playhead. Test a short frame/sheet timeout followed by operation resume, including
   two competing resumes. Verify final PNGs/sheet, output replacement and one native
   image job. Do not retry a pending original command.
6. Recheck run/render recovery against the updated bridge with scratch mutations and a
   short video. Verify no duplicate layer/render-queue item, expected frame count/audio,
   session-change refusal and preserved unknown-outcome artifacts. Reconcile every
   pending operation before leaving the validation scene.
7. Keep scene-open/keyframe-density and freezing claims unconfirmed until repeated,
   controlled measurements exist. If reducing keys is tested, rebuild a separate scene
   variant and compare load/rebuild timings plus matching images; do not edit the final
   promo to obtain a benchmark. Restore the owner's saved scene after validation.

## 1.1 release gate

After native validation and CI pass, set version files to 1.1.0. Use the required
signing pipeline and check both macOS archives for:
Developer ID Application signature, Hardened Runtime, secure timestamp, accepted
notarization, validated staple and Gatekeeper assessment on the extracted `Cav.app`.
Check checksums and confirm that install and update keep that exact bundle. Development
archives made with `--dev` cannot be published as releases. Publish the checked artifacts.
