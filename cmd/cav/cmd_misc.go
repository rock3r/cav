package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/rock3r/cav/assets"
	"github.com/rock3r/cav/internal/config"

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
var guideTopics = []string{"workflow", "design", "music", "traps", "native", "production"}

func init() {
	register(command{
		name:    "guide",
		args:    "[workflow|design|music|traps|native|production]",
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
`
}

func cmdGuide(a *app, args []string) error {
	topic := "workflow"
	if len(args) > 0 {
		topic = strings.ToLower(args[0])
	}
	b, err := assets.Guide.ReadFile("guide/" + topic + ".md")
	if err != nil {
		return fmt.Errorf("unknown guide %q: choose one of %s", topic, strings.Join(guideTopics, ", "))
	}
	a.emit(map[string]any{"topic": topic, "topics": guideTopics, "text": string(b)}, func() {
		fmt.Print(string(b))
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
