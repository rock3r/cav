package main

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestGenImageServiceSketchIsRefusedButTheDefaultStillGivesACard(t *testing.T) {
	t.Setenv("CAV_HOME", t.TempDir())
	t.Setenv("CAV_NO_KEYS", "1")
	for _, v := range []string{"HF_HUB_CACHE", "HF_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(v, t.TempDir())
	}
	t.Chdir(t.TempDir())
	err := cmdGen(&app{json: true}, []string{"image", "a fox", "--service", "sketch", "-o", "a.png"})
	var ce *cliError
	if !errors.As(err, &ce) || !strings.Contains(ce.msg, "sketch only renders storyboard SVGs") || !strings.Contains(ce.hint, "cav board frames") {
		t.Fatalf("got %v", err)
	}
	if _, statErr := os.Stat("a.png"); statErr == nil {
		t.Error("a refused run wrote an image")
	}
	// With no service named, the default order reaches sketch and the free card is made.
	captureStreams(t, func() { err = cmdGen(&app{json: true}, []string{"image", "a fox", "-o", "b.png"}) })
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat("b.png"); statErr != nil {
		t.Error("the default run made no card")
	}
}
