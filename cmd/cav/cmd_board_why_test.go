package main

import (
	"testing"

	"github.com/rock3r/cav/internal/board"
)

func TestBoardInitWritesTheWhyPlaceholder(t *testing.T) {
	inNewProject(t)
	if err := boardInit(&app{json: true}, []string{"--no-brief", "--shots", "3"}); err != nil {
		t.Fatal(err)
	}
	sb, err := board.Load("storyboard.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(sb.Shots) != 3 {
		t.Fatalf("shots: %d", len(sb.Shots))
	}
	for _, s := range sb.Shots {
		if s.Why != whyPlaceholder || !missingWhy(s) {
			t.Errorf("%s: why %q", s.ID, s.Why)
		}
	}
}
