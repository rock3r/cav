package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/config"
	"github.com/rock3r/cav/internal/docs"
	"github.com/rock3r/cav/internal/operation"
)

func TestDocsPrefersTitleWithContextOffline(t *testing.T) {
	t.Setenv("CAV_HOME", t.TempDir())
	if err := os.MkdirAll(config.DocsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	index := docs.Index{Chunks: []docs.Chunk{
		{Title: "Stagger", Type: "stagger", Heading: "Stagger > UI", Text: strings.Repeat("look duplicator ", 40)},
		{Title: "Look At", Type: "lookAt", Heading: "Look At > UI", Text: "Connect a target to aim at"},
	}}
	data, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(docs.Path(config.DocsDir()), data, 0o600); err != nil {
		t.Fatal(err)
	}
	// Capture the command's normal output without contacting a bridge or changing
	// the user's cache. This also checks that the CLI passes titles to the ranker.
	a := &app{op: &operation.Record{}}
	if err := cmdDocs(a, []string{"Look At duplicator", "-n", "1"}); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Results []docs.Chunk `json:"results"`
	}
	if err := json.Unmarshal(a.op.Data, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 1 || result.Results[0].Title != "Look At" {
		t.Fatalf("requested node not ranked first: %+v", result.Results)
	}
}
