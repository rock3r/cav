package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/guide"
	"github.com/rock3r/cav/internal/operation"
)

func TestGuideSearchReturnsTheTrapOffline(t *testing.T) {
	a := &app{op: &operation.Record{}}
	if err := cmdGuide(a, []string{"--search", "keys", "dropped"}); err != nil {
		t.Fatal(err)
	}
	var out struct {
		Query   string      `json:"query"`
		Results []guide.Hit `json:"results"`
	}
	if err := json.Unmarshal(a.op.Data, &out); err != nil {
		t.Fatal(err)
	}
	if out.Query != "keys dropped" || len(out.Results) != 3 {
		t.Fatalf("query %q, %d results", out.Query, len(out.Results))
	}
	first := out.Results[0]
	if first.Topic != "traps" || !strings.Contains(first.Text, "'rotation.z'") || first.Path[0] != "Keyframes and easing" {
		t.Fatalf("first result is not the rotation.z trap: %+v", first)
	}
	if strings.Contains(string(a.op.Data), "aliases") {
		t.Error("alias comments leak into the output")
	}
}

func TestGuideTopicHidesAliasComments(t *testing.T) {
	a := &app{op: &operation.Record{}}
	if err := cmdGuide(a, []string{"traps"}); err != nil {
		t.Fatal(err)
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(a.op.Data, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Text, "| Keys on `'rotation'` (a 3-channel attribute). | Silently no keys. | Key `'rotation.z'`. | `cav.key` |\n") {
		t.Fatal("the rotation row is not printed as before")
	}
	if strings.Contains(out.Text, "<!--") {
		t.Fatal("an alias comment is printed")
	}
}

func TestGuideSearchNeedsWords(t *testing.T) {
	if err := cmdGuide(&app{op: &operation.Record{}}, []string{"--search", " "}); err == nil {
		t.Fatal("an empty search did not fail")
	}
}
