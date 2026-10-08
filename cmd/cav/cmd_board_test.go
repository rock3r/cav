package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rock3r/cav/internal/board"
)

func TestRemadeFrameDropsItsClip(t *testing.T) {
	t.Setenv("CAV_HOME", t.TempDir())
	dir := t.TempDir()
	t.Chdir(dir)
	os.Mkdir(".cav", 0o755)
	os.MkdirAll("board", 0o755)
	os.WriteFile("board/s1.png", []byte("old frame"), 0o644)
	os.WriteFile("board/s1.mp4", []byte("old clip"), 0o644)
	sb := &board.Board{BPM: 120, Seconds: 2, Shots: []board.Shot{{ID: "s1", Beats: [2]float64{0, 4}, What: "a logo", Frame: "board/s1.png", Clip: "board/s1.mp4"}}}
	path := filepath.Join(dir, "storyboard.json")
	if err := sb.Save(path); err != nil {
		t.Fatal(err)
	}
	// Without --force the frame is kept, and so is its clip.
	if err := boardFrames(&app{json: true}, []string{"--service", "greybox", path}); err != nil {
		t.Fatal(err)
	}
	if got, _ := board.Load(path); got.Shots[0].Clip != "board/s1.mp4" {
		t.Fatalf("a kept frame must keep its clip: %+v", got.Shots[0])
	}
	if err := boardFrames(&app{json: true}, []string{"--service", "greybox", "--force", path}); err != nil {
		t.Fatal(err)
	}
	got, err := board.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Shots[0].Clip != "" || got.Shots[0].Source != "greybox" {
		t.Fatalf("a remade frame must drop the clip made from the old one: %+v", got.Shots[0])
	}
}
