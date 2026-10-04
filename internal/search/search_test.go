package search

import (
	"reflect"
	"strings"
	"testing"
)

func TestTokens(t *testing.T) {
	got := Tokens("api.getBoundingBox(layerId)")
	want := []string{"api", "get", "bound", "box", "getboundingbox", "layer", "id", "layerid"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if got := Tokens("keyframes"); got[0] != "keyframe" {
		t.Fatalf("stem: %v", got)
	}
}

func TestSearchRanksNameFirst(t *testing.T) {
	ix := New([]Doc{
		{Name: "setFrame", Head: "setFrame(frame: number): void", Body: "move the playhead"},
		{Name: "keyframe", Head: "keyframe(layerId, frame, dict)", Body: "set a keyframe"},
		{Name: "getKeyframeTimes", Head: "getKeyframeTimes(layerId, attrId)", Body: "keyframe times"},
	})
	hits := ix.Search("keyframe", 3)
	if len(hits) == 0 || ix.docs[hits[0].Index].Name != "keyframe" {
		t.Fatalf("exact name should win: %+v", hits)
	}
	hits = ix.Search("bounding box", 3)
	if len(hits) != 0 {
		t.Fatalf("no doc mentions bounding box: %+v", hits)
	}
}

func TestSearchTitleWithContext(t *testing.T) {
	ix := New([]Doc{
		{Name: "Stagger stagger", Title: "Stagger", Body: strings.Repeat("look duplicator ", 40)},
		{Name: "Look look", Title: "Look", Body: "look at duplicator"},
		{Name: "Look At lookAt", Title: "Look At", Body: "aim at a target"},
	})
	for _, query := range []string{"Look At", "Look At duplicator", "  LOOK   At\tduplicator  "} {
		t.Run(query, func(t *testing.T) {
			hits := ix.Search(query, 3)
			if len(hits) == 0 || hits[0].Index != 2 {
				t.Fatalf("complete longest title should win: %+v", hits)
			}
			for i := 1; i < len(hits); i++ {
				if hits[i].Score > hits[i-1].Score {
					t.Fatalf("scores must follow result order: %+v", hits)
				}
			}
		})
	}
}

func TestSearchTitleRequiresCompletePrefix(t *testing.T) {
	ix := New([]Doc{
		{Name: "Look node", Title: "Look", Body: "looking duplicator"},
		{Name: "Duplicator", Title: "Duplicator", Body: strings.Repeat("looking duplicator ", 40)},
	})
	for _, query := range []string{"Looking duplicator", "duplicator look"} {
		hits := ix.Search(query, 2)
		if len(hits) == 0 || hits[0].Index != 1 {
			t.Fatalf("partial or later title must not displace relevant result for %q: %+v", query, hits)
		}
	}
}

func TestSearchRanksSectionsWithinTitle(t *testing.T) {
	ix := New([]Doc{
		{Name: "Look At lookAt", Title: "Look At", Head: "Intro", Body: "rotate shapes"},
		{Name: "Look At lookAt", Title: "Look At", Head: "UI", Body: "duplicator target"},
		{Name: "Stagger", Title: "Stagger", Body: strings.Repeat("duplicator ", 40)},
	})
	hits := ix.Search("Look At duplicator", 3)
	if len(hits) != 3 || hits[0].Index != 1 || hits[1].Index != 0 {
		t.Fatalf("context should rank sections inside the preferred title: %+v", hits)
	}
}
