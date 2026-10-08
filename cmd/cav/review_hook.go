package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rock3r/cav/internal/config"
	"github.com/rock3r/cav/internal/review"
)

// hookInput is the part of a Claude Code or Codex command-hook event that cav reads. Both
// hosts send the same fields on stdin.
type hookInput struct {
	Event          string `json:"hook_event_name"`
	Cwd            string `json:"cwd"`
	SessionID      string `json:"session_id"`
	StopHookActive bool   `json:"stop_hook_active"`
}

type pendingSend struct {
	key, video string
	n, notes   int
	url        string
}

// reviewHook runs as a command hook (SessionStart, UserPromptSubmit, Stop) in Claude Code
// and Codex. When the reviewer has pressed "Send to agent" and no agent has picked the notes
// up, it tells the agent once: as context on a prompt or session start, or by continuing
// the turn on Stop. It never fails the host: problems go to stderr and it exits 0.
func reviewHook(stdin io.Reader, stdout, stderr io.Writer) int {
	var in hookInput
	if err := json.NewDecoder(io.LimitReader(stdin, 1<<20)).Decode(&in); err != nil {
		fmt.Fprintln(stderr, "cav review hook: no hook event on stdin:", err)
		return exitOK
	}
	quiet := func() int {
		if in.Event == "Stop" {
			fmt.Fprintln(stdout, "{}") // Stop expects JSON
		}
		return exitOK
	}
	if in.Cwd == "" {
		in.Cwd, _ = os.Getwd()
	}
	pending := findPending(in.Cwd)
	state := loadHookState()
	var fresh []pendingSend
	for _, p := range pending {
		if _, done := state[p.key]; !done {
			fresh = append(fresh, p)
		}
	}
	if len(fresh) == 0 || (in.Event == "Stop" && in.StopHookActive) {
		return quiet()
	}
	var lines []string
	total := 0
	for _, p := range fresh {
		total += p.notes
		lines = append(lines, fmt.Sprintf("- %d note(s) on %s (send #%d): run `cav review wait %s --timeout 10s` to read them", p.notes, p.video, p.n, p.video))
	}
	text := "The reviewer pressed \"Send to agent\" in cav review:\n" + strings.Join(lines, "\n") +
		"\n`cav review wait` prints each note (frame, text, drawing, snapshot path) and marks the send received; look at each snapshot." +
		" Fix the notes, re-render to the same file, then resolve each one with `cav review resolve <id> --note \"what changed\"`."
	msg := fmt.Sprintf("cav review: the reviewer sent %d note(s)", total)
	if len(fresh) == 1 {
		msg += " on " + filepath.Base(fresh[0].video)
	}
	var out map[string]any
	switch in.Event {
	case "Stop":
		out = map[string]any{"decision": "block", "reason": text, "systemMessage": msg}
	case "SessionStart", "UserPromptSubmit":
		out = map[string]any{"systemMessage": msg, "hookSpecificOutput": map[string]any{"hookEventName": in.Event, "additionalContext": text}}
	default:
		return quiet()
	}
	for _, p := range fresh {
		state[p.key] = time.Now().UTC()
	}
	saveHookState(state)
	b, _ := json.Marshal(out)
	fmt.Fprintln(stdout, string(b))
	return exitOK
}

// findPending lists the sends not yet received in the review files under dir's renders
// folder (or $CAV_OUT_DIR).
func findPending(dir string) []pendingSend {
	out := outDir()
	if !filepath.IsAbs(out) {
		out = filepath.Join(dir, out)
	}
	docs, _ := filepath.Glob(filepath.Join(out, "*.review.json"))
	sort.Strings(docs)
	var res []pendingSend
	for _, path := range docs {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var d review.Doc
		if json.Unmarshal(b, &d) != nil {
			continue
		}
		video := d.Source.Render
		if video == "" {
			video = strings.TrimSuffix(path, ".review.json") + ".mp4"
		}
		if rel, err := filepath.Rel(dir, video); err == nil && !strings.HasPrefix(rel, "..") && filepath.IsAbs(video) {
			video = rel
		}
		for _, s := range d.Sends {
			if s.DeliveredAt != nil {
				continue
			}
			p := pendingSend{key: fmt.Sprintf("%s#%d@%s", path, s.N, s.At.Format(time.RFC3339Nano)), video: video, n: s.N, notes: len(s.Comments)}
			if d.Server != nil {
				p.url = d.Server.URL
			}
			res = append(res, p)
		}
	}
	return res
}

func hookStatePath() string { return filepath.Join(config.Home(), "review-hook.json") }

func loadHookState() map[string]time.Time {
	st := map[string]time.Time{}
	if b, err := os.ReadFile(hookStatePath()); err == nil {
		json.Unmarshal(b, &st)
	}
	// Forget announcements older than a week.
	for k, t := range st {
		if time.Since(t) > 7*24*time.Hour {
			delete(st, k)
		}
	}
	return st
}

func saveHookState(st map[string]time.Time) {
	os.MkdirAll(config.Home(), 0o700)
	b, _ := json.Marshal(st)
	tmp := hookStatePath() + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		os.Rename(tmp, hookStatePath())
	}
}
