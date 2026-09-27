package main

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"github.com/rock3r/cavalry-skill/assets"

	"github.com/rock3r/cavalry-skill/internal/bridge"
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
	for _, s := range args {
		if s == "--check" {
			return checkUpdate(a, data)
		}
	}
	a.emit(data, func() {
		fmt.Printf("cav %s (%s)\nhelpers %s\nbridge %s\n", version, data["os"], data["helpers"], data["bridge"])
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
