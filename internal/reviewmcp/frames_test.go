package reviewmcp

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPageResourceAllowsOnlyTheVideoPlayerFrames(t *testing.T) {
	cs, _, _ := connect(t)
	res, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: PageURI})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(res.Contents[0].Meta["ui"])
	if err != nil {
		t.Fatal(err)
	}
	var ui struct {
		CSP struct {
			Frames    []string `json:"frameDomains"`
			Resources []string `json:"resourceDomains"`
		} `json:"csp"`
	}
	if err := json.Unmarshal(b, &ui); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ui.CSP.Frames, []string{"https://www.youtube-nocookie.com", "https://player.vimeo.com"}) {
		t.Errorf("frameDomains = %v", ui.CSP.Frames)
	}
	if !slices.Equal(ui.CSP.Resources, []string{"https://assets.claude.ai"}) {
		t.Errorf("resourceDomains = %v", ui.CSP.Resources)
	}
}
