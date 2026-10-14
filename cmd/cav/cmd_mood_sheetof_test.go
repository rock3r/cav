package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rock3r/cav/internal/moodboard"
)

func makeClip(t *testing.T, path, seconds string) {
	t.Helper()
	if b, err := exec.Command("ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i", "testsrc=size=160x90:rate=10", "-t", seconds, "-pix_fmt", "yuv420p", path).CombinedOutput(); err != nil {
		t.Skipf("ffmpeg cannot make a clip: %v %s", err, b)
	}
}

func TestSheetFollowsTheClipBytesNotItsTime(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	inNewProject(t)
	os.MkdirAll("moodboard", 0o755)
	clip := "moodboard/reel.mp4"
	makeClip(t, clip, "2")
	ctx := context.Background()
	b, _, _, err := moodSync(ctx, "moodboard", "moodboard.json")
	if err != nil {
		t.Fatal(err)
	}
	first := *b.Find("reel")
	if first.Seconds < 1.9 || first.Seconds > 2.1 || first.SheetOf == "" {
		t.Fatalf("first %+v", first)
	}

	// An unchanged clip: no probe at all, and the sheet and length stay, even if probing would fail.
	calls := 0
	old := moodboard.Probe
	moodboard.Probe = func(context.Context, string) (float64, error) { calls++; return 0, errors.New("ffprobe is broken") }
	t.Cleanup(func() { moodboard.Probe = old })
	b, _, notes, err := moodSync(ctx, "moodboard", "moodboard.json")
	if err != nil || calls != 0 || len(notes) != 0 {
		t.Fatalf("calls %d notes %v err %v", calls, notes, err)
	}
	if it := b.Find("reel"); it.Seconds != first.Seconds || it.SheetOf != first.SheetOf {
		t.Fatalf("item changed %+v", it)
	}

	// The sheet is gone and the probe fails: nothing to keep, the digest stays, a note says why.
	sheet := filepath.Join("moodboard", ".thumbs", "reel.png")
	os.Remove(sheet)
	_, _, notes, _ = moodSync(ctx, "moodboard", "moodboard.json")
	if len(notes) != 1 || !strings.Contains(notes[0], "kept the old one") {
		t.Errorf("notes %v", notes)
	}
	moodboard.Probe = old

	// A shorter clip with an older time (cp -p, unzip): the bytes differ, so the sheet is made again.
	makeClip(t, clip, "1")
	past := time.Now().Add(-48 * time.Hour)
	os.Chtimes(clip, past, past)
	os.Chtimes(sheet, time.Now(), time.Now())
	b, _, notes, err = moodSync(ctx, "moodboard", "moodboard.json")
	if err != nil || len(notes) != 0 {
		t.Fatalf("%v %v", err, notes)
	}
	it := b.Find("reel")
	if it.Seconds < 0.9 || it.Seconds > 1.1 || it.SheetOf == first.SheetOf {
		t.Fatalf("not refreshed: %+v", it)
	}

	// The replacement cannot be read: the stale sheet goes, because the clip changed.
	os.WriteFile(clip, []byte("not a video"), 0o644)
	os.Chtimes(clip, past, past)
	b, _, notes, _ = moodSync(ctx, "moodboard", "moodboard.json")
	if it := b.Find("reel"); it.Seconds != 0 || it.SheetOf != "" {
		t.Errorf("stale values kept: %+v", it)
	}
	if _, err := os.Stat(sheet); err == nil {
		t.Error("the stale sheet is still there")
	}
	if len(notes) != 1 || !strings.HasPrefix(notes[0], "reel: no contact sheet") {
		t.Errorf("notes %v", notes)
	}
}
