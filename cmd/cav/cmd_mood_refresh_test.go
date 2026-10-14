package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/library"
)

func TestBoardInitRefusesLessThanOneBeat(t *testing.T) {
	inNewProject(t)
	err := boardInit(&app{json: true}, []string{"--seconds", "0.4", "--bpm", "120", "--no-brief"})
	if err == nil || !strings.Contains(err.Error(), "less than one beat") {
		t.Fatalf("err %v", err)
	}
	if _, statErr := os.Stat("storyboard.json"); statErr == nil {
		t.Fatal("a storyboard was written")
	}
}

func TestRefAddAgainRefreshesTheEmbed(t *testing.T) {
	inNewProject(t)
	root := library.Root()
	stubSites(t, false) // offline: no thumbnail, no title
	add := func(args ...string) {
		t.Helper()
		var err error
		captureStreams(t, func() { err = sourcesAdd(&app{json: true}, "image", root, "moodboard", args) })
		if err != nil {
			t.Fatal(err)
		}
	}
	add("https://vimeo.com/123456789", "--title", "By hand")
	add("https://vimeo.com/123456789/abcdef1234", "--title", "Retitled")
	b := readMood(t, "moodboard.json")
	if len(b.Items) != 1 {
		t.Fatalf("items %+v", b.Items)
	}
	if it := b.Items[0]; it.URL != "https://vimeo.com/123456789/abcdef1234" || it.Title != "Retitled" {
		t.Fatalf("not refreshed: %+v", it)
	}

	// A YouTube video added offline, then again with the site up: the thumbnail and title arrive.
	add("https://youtu.be/aqz-KE-bpKQ", "--take", "kept take")
	stubSites(t, true)
	add("https://youtu.be/aqz-KE-bpKQ")
	b = readMood(t, "moodboard.json")
	it := b.Find("yt-aqz-KE-bpKQ")
	if len(b.Items) != 2 || it == nil || it.Title != "Launch reel" || it.Thumb != "moodboard/.thumbs/yt-aqz-KE-bpKQ.jpg" || it.Take != "kept take" {
		t.Fatalf("items %+v", b.Items)
	}
	if _, err := os.Stat(filepath.Join("moodboard", ".thumbs", "yt-aqz-KE-bpKQ.jpg")); err != nil {
		t.Error(err)
	}
}

func TestBoardMoodDrawsACardForAnEmbedWithoutAThumbnail(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	inNewProject(t)
	root := library.Root()
	stubSites(t, false)
	var err error
	captureStreams(t, func() {
		err = sourcesAdd(&app{json: true}, "image", root, "moodboard", []string{"https://vimeo.com/123456789", "--title", "Film by hand"})
	})
	if err != nil {
		t.Fatal(err)
	}
	captureStreams(t, func() { err = boardMood(&app{json: true}, nil) })
	if err != nil {
		t.Fatalf("an embed-only board must work: %v", err)
	}
	if st, err := os.Stat(filepath.Join("renders", "moodboard.png")); err != nil || st.Size() == 0 {
		t.Fatalf("no picture: %v", err)
	}
	if len(readMood(t, "moodboard.json").Items) != 1 {
		t.Error("the item is gone")
	}
}
