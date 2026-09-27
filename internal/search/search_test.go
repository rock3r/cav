package search

import (
	"reflect"
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
