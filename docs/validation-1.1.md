# cav 1.1 validation queue

The current fixes are prepared in an isolated checkout. Cavalry, the installed CLI,
the live bridge, `launch-video/` and `validation-pr1/` were not modified or contacted.
Native checks below are pending the owner's explicit signal that the promo session is
finished. No release is ready until those checks and final artifact verification pass.

## Offline evidence

- Go race fixtures cover timeouts during native scene opening and follow-up metadata,
  changed-input rejection, two competing resumes, frame/sheet publication without
  rerendering, and recovery after the publication checkpoint and sheet frame cleanup.
- JavaScript fixtures cover nested/shared pre-comp references, scan/layer/composition
  limits, membership failures and cycles without frame changes or bounds/render calls.
- Profile fixtures cover explicit late targets, chronological PNG evaluation of every
  intervening frame, separate warm-up/sample timing, budget exhaustion, failed warm-up,
  changed scene identity, and playhead restoration.
- `go vet ./...` and all explicit Node fixture globs pass. Credential-free development
  archives build for macOS arm64/amd64, Windows arm64/amd64 and Linux amd64.
- The signed macOS pipeline from PR #2 has a successful
  [notarized artifact smoke run](https://github.com/rock3r/cav/actions/runs/37152868668).
  That verifies the pipeline at its recorded revision, not final 1.1 artifacts.

## Native validation after the signal

Use a CLI built from the final branch revision with a distinct validation version and
output directory. Preserve the owner's final scene and work on a disposable copy.
Record the scene digest, original playhead and comp, CLI/bridge versions and bridge
session before/after. Keep one owner of the live bridge throughout the checks.

1. Save the finished promo scene before leaving it. Preserve copies and baseline images.
   Run bridge preflight and inspect existing operations before submitting any native work.
   Install/restart the updated bridge only now, after the promo session is finished;
   verify the new bridge's session ID and direct duplicate-ID behavior on a scratch scene.
2. Open a small scene, then the promo copy with `scene open --timeout 15m`. Repeat with
   a deliberately short budget and resume the same operation through the follow-up query.
   Count submitted job IDs: the native open must appear once. Record wall-clock time
   through actual scene-info readiness, not just the open call returning.
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

After native validation and CI pass, reconcile version files to 1.1.0, produce the
release through the mandatory signing pipeline, and verify both macOS archives:
Developer ID Application signature, Hardened Runtime, secure timestamp, accepted
notarization, validated staple and Gatekeeper assessment on the extracted `Cav.app`.
Verify checksums and install/update preservation of that exact bundle. Credential-free
`--dev` archives are not release candidates. Publish only the verified final artifacts.
