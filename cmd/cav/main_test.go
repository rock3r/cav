package main

import (
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/rock3r/cavalry-skill/assets"
)

func TestHelperSignature(t *testing.T) {
	for _, name := range []string{"rect", "text", "create", "clear", "keys"} {
		if sig := helperSignature(name); !strings.HasPrefix(sig, "cav."+name+"(") {
			t.Errorf("helperSignature(%q) = %q", name, sig)
		}
	}
	if sig := helperSignature("noSuchHelper"); sig != "" {
		t.Errorf("unknown helper gave %q", sig)
	}
}

// Every guide topic has a file, and every embedded guide is listed as a topic.
func TestGuideTopics(t *testing.T) {
	files, err := fs.Glob(assets.Guide, "guide/*.md")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range files {
		names = append(names, strings.TrimSuffix(strings.TrimPrefix(f, "guide/"), ".md"))
	}
	slices.Sort(names)
	topics := slices.Clone(guideTopics)
	slices.Sort(topics)
	if !slices.Equal(names, topics) {
		t.Errorf("guide files %v, topics %v", names, topics)
	}
	for _, name := range guideTopics {
		b, _ := assets.Guide.ReadFile("guide/" + name + ".md")
		if strings.Contains(string(b), "references/") {
			t.Errorf("guide %s still points to a skill reference file", name)
		}
	}
}
