package main

import (
	"encoding/json"
	"fmt"
	"runtime"

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
