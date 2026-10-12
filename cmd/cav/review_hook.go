package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"

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

// antigravityInput is the part of an Antigravity hook event that cav reads. Antigravity
// sends camelCase fields and does not name the event, so the hook command passes --event.
type antigravityInput struct {
	ConversationID    string   `json:"conversationId"`
	WorkspacePaths    []string `json:"workspacePaths"`
	TerminationReason string   `json:"terminationReason"`
}

type pendingSend struct {
	key, video string
	n, notes   int
	url        string
}

// reviewHook runs as a command hook. When the reviewer has pressed "Send to agent" and no
// agent has picked the notes up, it tells the agent once. In Claude Code and Codex
// (SessionStart, UserPromptSubmit, Stop) it adds context to a prompt or session start, or
// continues the turn on Stop. With --host antigravity it does the same for Antigravity
// (PreInvocation, Stop). It never fails the host: problems go to stderr and it exits 0.
func reviewHook(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("review hook", flag.ContinueOnError)
	fs.SetOutput(stderr)
	host := fs.String("host", "", "the agent host: empty for Claude Code and Codex, or antigravity")
	event := fs.String("event", "", "the Antigravity event: PreInvocation or Stop")
	if err := fs.Parse(args); err != nil {
		return exitOK
	}
	switch *host {
	case "":
		return claudeHook(stdin, stdout, stderr)
	case "antigravity":
		return antigravityHook(*event, stdin, stdout, stderr)
	}
	fmt.Fprintf(stderr, "cav review hook: unknown host %q\n", *host)
	return exitOK
}

func claudeHook(stdin io.Reader, stdout, stderr io.Writer) int {
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
	state := loadHookState()
	fresh := freshSends(state, in.Cwd)
	if len(fresh) == 0 || (in.Event == "Stop" && in.StopHookActive) {
		return quiet()
	}
	text, msg := hookMessage(fresh)
	var out map[string]any
	switch in.Event {
	case "Stop":
		out = map[string]any{"decision": "block", "reason": text, "systemMessage": msg}
	case "SessionStart", "UserPromptSubmit":
		out = map[string]any{"systemMessage": msg, "hookSpecificOutput": map[string]any{"hookEventName": in.Event, "additionalContext": text}}
	default:
		return quiet()
	}
	markSent(state, fresh)
	saveHookState(state)
	b, _ := json.Marshal(out)
	fmt.Fprintln(stdout, string(b))
	return exitOK
}

// antigravityHook speaks the Antigravity hook protocol. PreInvocation runs before every
// model call, so the hook only reads the review files and the state file, and it tells
// the agent about each send once: the state file remembers what it already told. Stop has
// no stop_hook_active field, so the same state ends the loop: a Stop continues the turn
// only for sends no agent has heard about, and only after the state file records them.
func antigravityHook(event string, stdin io.Reader, stdout, stderr io.Writer) int {
	var in antigravityInput
	if err := json.NewDecoder(io.LimitReader(stdin, 1<<20)).Decode(&in); err != nil {
		fmt.Fprintln(stderr, "cav review hook: no hook event on stdin:", err)
	}
	quiet := func() int {
		if event == "Stop" {
			fmt.Fprintln(stdout, `{"decision":"stop"}`) // decision is required; any value but "continue" lets the agent stop
		} else {
			fmt.Fprintln(stdout, "{}")
		}
		return exitOK
	}
	if event != "PreInvocation" && event != "Stop" {
		fmt.Fprintf(stderr, "cav review hook: unknown Antigravity event %q\n", event)
		return quiet()
	}
	// After an error or the step limit, let the agent stop. The next PreInvocation
	// delivers the notes.
	if event == "Stop" && in.TerminationReason != "" && in.TerminationReason != "model_stop" {
		return quiet()
	}
	dirs := in.WorkspacePaths
	if len(dirs) == 0 {
		wd, _ := os.Getwd()
		dirs = []string{wd}
	}
	state := loadHookState()
	var fresh []pendingSend
	seen := map[string]bool{} // an absolute CAV_OUT_DIR is the same folder for every workspace
	for _, dir := range dirs {
		for _, p := range freshSends(state, dir) {
			if seen[p.key] {
				continue
			}
			seen[p.key] = true
			// The agent may run in another workspace folder, so name the render in full.
			if !filepath.IsAbs(p.video) {
				p.video = filepath.Join(dir, p.video)
			}
			fresh = append(fresh, p)
		}
	}
	if len(fresh) == 0 {
		return quiet()
	}
	text, _ := hookMessage(fresh)
	markSent(state, fresh)
	// Without a saved state, every model call and every Stop would repeat the notes.
	if !saveHookState(state) {
		fmt.Fprintln(stderr, "cav review hook: cannot save", hookStatePath())
		return quiet()
	}
	var out map[string]any
	if event == "Stop" {
		out = map[string]any{"decision": "continue", "reason": text}
	} else {
		out = map[string]any{"injectSteps": []map[string]any{{"ephemeralMessage": text}}}
	}
	b, _ := json.Marshal(out)
	fmt.Fprintln(stdout, string(b))
	return exitOK
}

// freshSends lists the sends under dir that are not received and that no hook has told an
// agent about yet.
func freshSends(state map[string]time.Time, dir string) []pendingSend {
	var fresh []pendingSend
	for _, p := range findPending(dir) {
		if _, done := state[p.key]; !done {
			fresh = append(fresh, p)
		}
	}
	return fresh
}

func markSent(state map[string]time.Time, sends []pendingSend) {
	for _, p := range sends {
		state[p.key] = time.Now().UTC()
	}
}

// hookMessage returns the text for the agent and a short message for the person.
func hookMessage(fresh []pendingSend) (text, msg string) {
	var lines []string
	total := 0
	for _, p := range fresh {
		total += p.notes
		lines = append(lines, fmt.Sprintf("- %d note(s) on %s (send #%d): run `cav review wait %s --timeout 10s` to read them", p.notes, plainPath(p.video), p.n, shellArg(p.video)))
	}
	text = "The reviewer pressed \"Send to agent\" in cav review:\n" + strings.Join(lines, "\n") +
		"\n`cav review wait` prints each note (frame, text, drawing, snapshot path) and marks the send received; look at each snapshot." +
		" Fix the notes, re-render to the same file, then resolve each one with `cav review resolve <id> --note \"what changed\"`."
	msg = fmt.Sprintf("cav review: the reviewer sent %d note(s)", total)
	if len(fresh) == 1 {
		msg += " on " + plainPath(filepath.Base(fresh[0].video))
	}
	return text, msg
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
		} else if !filepath.IsAbs(video) {
			// cav review stores the path as it was typed, relative to where it ran. The review
			// file always sits next to its render, so take the folder from the review file.
			video = filepath.Join(filepath.Dir(path), filepath.Base(video))
		}
		if rel, err := filepath.Rel(dir, video); err == nil && !strings.HasPrefix(rel, "..") && filepath.IsAbs(video) {
			video = filepath.ToSlash(rel) // cav accepts / on Windows too, and the message stays the same everywhere
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

// plainPath replaces control characters and line separators, such as a line break or U+2028, with "?". A review file
// could carry them, and they would start a new line in the agent's message.
func plainPath(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp) {
			return '?'
		}
		return r
	}, s)
}

// shellArg quotes a path for the command the agent runs: single quotes in POSIX shells
// and in PowerShell, which expands neither $ nor ` inside them. Control characters, such
// as a line break, could start another command, so they become "?".
func shellArg(s string) string {
	s = plainPath(s)
	if runtime.GOOS == "windows" {
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// saveHookState writes the state through a temporary file of its own, so hooks that run
// at the same time never rename each other's file. When two of them race, the last one
// wins, and at worst a send is announced twice.
func saveHookState(st map[string]time.Time) bool {
	os.MkdirAll(config.Home(), 0o700)
	b, _ := json.Marshal(st)
	f, err := os.CreateTemp(config.Home(), "review-hook-*.tmp")
	if err != nil {
		return false
	}
	_, werr := f.Write(b)
	cerr := f.Close()
	if werr != nil || cerr != nil || os.Rename(f.Name(), hookStatePath()) != nil {
		os.Remove(f.Name())
		return false
	}
	return true
}
