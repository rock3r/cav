package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/rock3r/cav/assets"
	"github.com/rock3r/cav/internal/config"
	"github.com/rock3r/cav/internal/guide"

	"github.com/rock3r/cav/internal/bridge"
)

func newClient() *bridge.Client { return bridge.New() }

func init() {
	register(command{
		name:    "version",
		args:    "[--check]",
		summary: "Print the cav, helper library and bridge versions.",
		run:     cmdVersion,
	})
}

func cmdVersion(a *app, args []string) error {
	data := map[string]any{
		"cav":     version,
		"helpers": helpersVersion(),
		"bridge":  bridgeVersionFromJS(),
		"os":      runtime.GOOS + "/" + runtime.GOARCH,
	}
	data["warnings"] = bridgeWarnings(nil)
	for _, s := range args {
		if s == "--check" {
			return checkUpdate(a, data)
		}
	}
	a.emit(data, func() {
		fmt.Printf("cav %s (%s)\nhelpers %s\nbridge %s\n", version, data["os"], data["helpers"], data["bridge"])
		for _, w := range bridgeWarnings(nil) {
			fmt.Println("warning: " + w)
		}
	})
	return nil
}

func prettyJSON(raw []byte) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

func prettyAny(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

func init() {
	register(command{
		name:    "helpers",
		args:    "[word]",
		summary: "Print the helper library reference (the global `cav` inside scripts).",
		run:     cmdHelpers,
	})
}

// guideTopics lists the guides in the order `cav guide` presents them.
var guideTopics = []string{"workflow", "design", "music", "traps", "native", "production", "ui-showcase"}

func init() {
	register(command{
		name:    "guide",
		args:    "[workflow|design|music|traps|native|production|ui-showcase] | --search <words> [-n 3]",
		summary: "Print how to make motion graphics with cav: workflow, design, music, traps.",
		run:     cmdGuide,
	})
	longHelp["guide"] = `
Without a topic, prints the workflow guide. Read it before your first build.

  workflow  the build-review-check-render loop, script rules and a done checklist
  design    motion recipes: entrances, impacts, kinetic type, transitions, holds
  music     beat grids and syncing motion to music
  traps     Cavalry scripting behaviours that fail silently, and how cav avoids them
  native    duplicators, stagger, text effects, filters, 2.5D, particles and other nodes
  production  services and keys, licences, storyboards, assets, music, listening, review
  ui-showcase  software videos: launch videos, product demos and UI walkthroughs

cav guide --search <words> [-n 3]
  Search every guide and print the best sections, with their topic and heading. Describe
  the symptom when you do not know the topic: cav guide --search "rotation keys ignored".
  Table rows (each trap) come back one by one. The output is capped at about 4 KB.
`
}

// Search output limits: one long section, and all hits together.
const (
	guideHitBytes   = 1500
	guideTotalBytes = 4000
)

func cmdGuide(a *app, args []string) error {
	fs := flag.NewFlagSet("guide", flag.ContinueOnError)
	query := fs.String("search", "", "search all guides")
	n := fs.Int("n", 3, "number of sections")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if flagSet(fs, "search") {
		return guideSearch(a, strings.TrimSpace(strings.Join(append([]string{*query}, pos...), " ")), *n)
	}
	topic := "workflow"
	if len(pos) > 0 {
		topic = strings.ToLower(pos[0])
	}
	b, err := assets.Guide.ReadFile("guide/" + topic + ".md")
	if err != nil {
		return fmt.Errorf("unknown guide %q: choose one of %s", topic, strings.Join(guideTopics, ", "))
	}
	text := guide.StripAliases(string(b))
	a.emit(map[string]any{"topic": topic, "topics": guideTopics, "text": text}, func() {
		fmt.Print(text)
	})
	return nil
}

func flagSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}

func guideSearch(a *app, q string, n int) error {
	if q == "" {
		return usageErr("usage: cav guide --search <words>, for example: cav guide --search \"rotation keys ignored\"")
	}
	if n < 1 {
		n = 1
	}
	ix, err := guide.Load(assets.Guide, "guide", guideTopics)
	if err != nil {
		return err
	}
	hits, dropped := guide.Cap(ix.Search(q, n), guideHitBytes, guideTotalBytes)
	if hits == nil {
		hits = []guide.Hit{}
	}
	apiHint := "For API functions and their notes, try `cav api " + q + "`."
	data := map[string]any{"query": q, "results": hits, "topics": guideTopics, "hint": apiHint}
	if dropped > 0 {
		data["omitted"] = dropped
	}
	a.emit(data, func() {
		if len(hits) == 0 {
			fmt.Printf("no guide section matches. Topics: %s. %s\n", strings.Join(guideTopics, ", "), apiHint)
			return
		}
		for _, h := range hits {
			fmt.Printf("== %s  (cav guide %s)\n%s\n\n", strings.Join(append([]string{h.Topic}, h.Path...), " › "), h.Topic, h.Text)
		}
		if dropped > 0 {
			fmt.Printf("(%d more sections did not fit; search with more specific words)\n", dropped)
		}
		fmt.Println(apiHint)
	})
	return nil
}

func cmdHelpers(a *app, args []string) error {
	var lines []string
	for _, l := range strings.Split(string(assets.HelpersJS), "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "//@") {
			lines = append(lines, strings.TrimPrefix(strings.TrimPrefix(t, "//@"), " "))
		}
	}
	if len(args) > 0 {
		q := strings.ToLower(strings.Join(args, " "))
		var keep []string
		for i, l := range lines {
			if strings.Contains(strings.ToLower(l), q) {
				keep = append(keep, l)
				// Include continuation lines (indented) that follow a match.
				for j := i + 1; j < len(lines) && strings.HasPrefix(lines[j], "    "); j++ {
					keep = append(keep, lines[j])
				}
			}
		}
		lines = keep
	}
	text := strings.Join(lines, "\n")
	a.emit(map[string]any{"version": helpersVersion(), "reference": text}, func() {
		fmt.Println(text)
	})
	return nil
}

func bridgeWarnings(payload map[string]any) []string {
	warnings := make([]string, 0)
	target := filepath.Join(config.ScriptsDir(), config.BridgeFile)
	b, err := os.ReadFile(target)
	if err == nil && !bytes.Equal(b, assets.BridgeJS) {
		warnings = append(warnings, "installed bridge script differs from this CLI; run cav setup and reopen Scripts > cav-bridge")
	}
	if v, ok := payload["bridgeVersion"].(string); ok && v != bridgeVersionFromJS() {
		warnings = append(warnings, "running bridge "+v+" differs from embedded "+bridgeVersionFromJS()+"; reopen Scripts > cav-bridge")
	}
	return warnings
}
