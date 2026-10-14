package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// A shot id becomes a file name, so an id such as ../s1 must stop the run before anything is written.
func TestFramesRefuseShotIDsThatAreNotSimpleNames(t *testing.T) {
	_, path := sketchProject(t)
	sb := map[string]any{"schema": "cav-board/1", "bpm": 120, "seconds": 8, "width": 1920, "height": 1080,
		"shots": []map[string]any{{"id": "../s1", "beats": []float64{0, 4}, "what": "x"}}}
	data, err := json.Marshal(sb)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	f := newFakeSketchCavalry(t, savedScene())
	for _, args := range [][]string{{path}, {"--service", "greybox", path}, {"--service", "sketch", path}} {
		_, _, err := framesJSON(t, args...)
		if err == nil || !strings.Contains(err.Error(), `shot id "../s1" cannot be a file name`) {
			t.Fatalf("args %v: got %v", args, err)
		}
	}
	if jobs := f.kinds(); len(jobs) != 0 {
		t.Errorf("Cavalry was asked: %v", jobs)
	}
	if entries, _ := os.ReadDir("board"); len(entries) != 0 {
		t.Errorf("files were written: %v", entries)
	}
}
