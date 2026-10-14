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
	"github.com/rock3r/cav/internal/flow"
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

// workflowAliasWarning is printed on stderr by `cav guide workflow`, the old name of the
// scene guide.
const workflowAliasWarning = "To build a scene in Cavalry, use `cav guide scene`. For the production stages from brief to animatic, use `cav guide flow`."

// guideTopics lists the guides in the order `cav guide` presents them.
var guideTopics = []string{"scene", "flow", "design", "music", "traps", "native", "production", "ui-showcase"}

func init() {
	register(command{
		name:    "guide",
		args:    "[scene|flow|design|music|traps|native|production|ui-showcase] | --search <words> [-n 3]",
		summary: "Print how to make motion graphics with cav: scene, flow, design, music, traps.",
		run:     cmdGuide,
	})
	longHelp["guide"] = `
Without a topic, prints the scene guide. Read it before your first build.

  scene     the build-review-check-render loop, script rules and a done checklist
  flow      production flow: the rules, then the flow in effect (flow.md in the project,
            ~/.cav/flow.md, or the standard flow: brief, mood board, storyboard, music
            and animatic, build)
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

cav guide workflow
  The old name of the scene guide. It still works: it prints a warning on stderr, then the
  scene guide.
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
	topic := "scene"
	if len(pos) > 0 {
		topic = strings.ToLower(pos[0])
	}
	if topic == "workflow" {
		// Older instructions still name this topic.
		fmt.Fprintln(os.Stderr, workflowAliasWarning)
		topic = "scene"
	}
	b, err := assets.Guide.ReadFile("guide/" + topic + ".md")
	if err != nil {
		return fmt.Errorf("unknown guide %q: choose one of %s", topic, strings.Join(guideTopics, ", "))
	}
	text := guide.StripAliases(string(b))
	data := map[string]any{"topic": topic, "topics": guideTopics}
	if topic == "flow" {
		var info map[string]any
		var warns []string
		text, info, warns = flowGuideText(text)
		data["flow"] = info
		for _, w := range warns {
			fmt.Fprintf(os.Stderr, "warning: %s\n", w)
		}
	}
	data["text"] = text
	a.emit(data, func() {
		fmt.Print(text)
	})
	return nil
}

// flowGuideText adds the flow in effect to the rules in flow.md: the project's flow.md, then
// ~/.cav/flow.md, then the standard flow. A file that does not parse is still printed, and
// the problems come back as warnings. info is the "flow" field of --json: it always has
// the source and path of the file in effect, and "valid" says whether the file parsed.
// Stages that ask for a review that cannot hold an approval are warnings too.
func flowGuideText(rules string) (text string, info map[string]any, warns []string) {
	f, err := flow.Load(".")
	path, source := flow.Find(".")
	var body, label string
	switch {
	case err == nil:
		body, label = f.Text, f.Label()
		warns = f.Warnings(".")
		info = flowInfo(f)
	case path != "":
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return rules, map[string]any{"source": source, "path": path, "valid": false, "error": readErr.Error(), "stages": []any{}, "warnings": []string{}}, []string{readErr.Error()}
		}
		body = strings.TrimPrefix(string(b), "\xef\xbb\xbf")
		label = (&flow.Flow{Source: source, Path: path}).Label()
		warns = strings.Split(err.Error(), "\n")
		info = map[string]any{"source": source, "path": path, "valid": false, "error": err.Error(), "stages": []any{}, "warnings": warns}
	default:
		return rules, map[string]any{"source": source, "path": "", "valid": false, "error": err.Error(), "stages": []any{}, "warnings": []string{}}, []string{err.Error()}
	}
	var sb strings.Builder
	sb.WriteString(strings.TrimRight(rules, "\n"))
	sb.WriteString("\n\n## The flow in effect: " + label + "\n\n")
	if path != "" {
		sb.WriteString("cav reads its stages from " + path + ".\n\n")
	}
	sb.WriteString(strings.TrimRight(body, "\n") + "\n")
	return sb.String(), info, warns
}

// flowInfo is the "flow" field of cav guide flow --json for a flow that parsed.
func flowInfo(f *flow.Flow) map[string]any {
	stages := []map[string]any{}
	for _, s := range f.Stages {
		outs := s.Outputs
		if outs == nil {
			outs = []string{}
		}
		stages = append(stages, map[string]any{"name": s.Name, "outputs": outs, "review": s.Review, "use": s.Use})
	}
	return map[string]any{"source": f.Source, "path": f.Path, "valid": true, "stages": stages, "warnings": f.Warnings(".")}
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
	// The standard flow is not a guide topic, so index it with the flow guide.
	ix = guide.New(append(ix.Sections, guide.Split("flow", string(assets.StandardFlow))...))
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
