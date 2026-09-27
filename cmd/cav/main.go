// Command cav drives Cavalry from a shell, for people and coding agents.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/rock3r/cavalry-skill/internal/bridge"
)

// Set by the release build with -ldflags "-X main.version=...".
var version = "0.1.0-dev"

// Exit codes. Agents can branch on them without parsing text.
const (
	exitOK           = 0
	exitError        = 1 // usage error or script error
	exitUnavailable  = 2 // bridge not reachable
	exitStillRunning = 3 // wait timed out, job still queued or running
	exitLost         = 4 // bridge vanished while a job was in flight
)

type command struct {
	name    string
	args    string
	summary string
	run     func(a *app, args []string) error
}

var commands []command

func register(c command) { commands = append(commands, c) }

type app struct {
	json    bool
	started time.Time
	argv    []string
	logJob  map[string]any
}

// cliError carries an exit code and an optional hint.
type cliError struct {
	code int
	msg  string
	hint string
	data map[string]any
}

func (e *cliError) Error() string { return e.msg }

func fail(code int, msg, hint string) error { return &cliError{code: code, msg: msg, hint: hint} }

func usageErr(format string, a ...any) error {
	return &cliError{code: exitError, msg: fmt.Sprintf(format, a...), hint: "run `cav help` for usage"}
}

func main() {
	a := &app{started: time.Now(), argv: os.Args[1:]}
	args := make([]string, 0, len(os.Args))
	for _, s := range os.Args[1:] {
		if s == "--json" {
			a.json = true
			continue
		}
		args = append(args, s)
	}
	if os.Getenv("CAV_JSON") == "1" {
		a.json = true
	}
	code := a.dispatch(args)
	a.writeLog(code)
	os.Exit(code)
}

func (a *app) dispatch(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		topic := ""
		if len(args) > 1 {
			topic = args[1]
		}
		printHelp(topic)
		return exitOK
	}
	if args[0] == "--version" || args[0] == "-v" {
		args[0] = "version"
	}
	for _, c := range commands {
		if c.name == args[0] {
			if len(args) > 1 && (args[1] == "-h" || args[1] == "--help") {
				printHelp(c.name)
				return exitOK
			}
			err := c.run(a, args[1:])
			return a.finish(err)
		}
	}
	return a.finish(usageErr("unknown command %q", args[0]))
}

func (a *app) finish(err error) int {
	if err == nil {
		return exitOK
	}
	var ce *cliError
	if !errors.As(err, &ce) {
		ce = &cliError{code: exitError, msg: err.Error()}
		switch {
		case errors.Is(err, bridge.ErrUnavailable):
			ce.code = exitUnavailable
		case errors.Is(err, bridge.ErrStillRunning):
			ce.code = exitStillRunning
		case errors.Is(err, bridge.ErrLost):
			ce.code = exitLost
		}
	}
	if a.json {
		out := map[string]any{"ok": false, "error": ce.msg, "exitCode": ce.code}
		if ce.hint != "" {
			out["hint"] = ce.hint
		}
		for k, v := range ce.data {
			out[k] = v
		}
		printJSON(out)
	} else {
		label := "error: "
		if ce.code == exitStillRunning {
			label = "cav: "
		}
		fmt.Fprintln(os.Stderr, label+ce.msg)
		if ce.hint != "" {
			fmt.Fprintln(os.Stderr, "hint: "+ce.hint)
		}
	}
	return ce.code
}

// emit prints a successful result: JSON when --json is set, otherwise the human form.
func (a *app) emit(data map[string]any, human func()) {
	if a.json {
		out := map[string]any{"ok": true}
		for k, v := range data {
			out[k] = v
		}
		printJSON(out)
		return
	}
	human()
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// writeLog appends one JSON line per invocation to $CAV_LOG. Evals use it to see what
// an agent did; users can use it to debug a session.
func (a *app) writeLog(code int) {
	path := os.Getenv("CAV_LOG")
	if path == "" {
		return
	}
	rec := map[string]any{
		"ts":   a.started.UTC().Format(time.RFC3339Nano),
		"argv": a.argv,
		"exit": code,
		"ms":   time.Since(a.started).Milliseconds(),
	}
	for k, v := range a.logJob {
		rec[k] = v
	}
	b, _ := json.Marshal(rec)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}

func printHelp(topic string) {
	if topic != "" {
		for _, c := range commands {
			if c.name == topic {
				fmt.Printf("cav %s %s\n\n%s\n", c.name, c.args, c.summary)
				if h, ok := longHelp[c.name]; ok {
					fmt.Println()
					fmt.Println(strings.TrimSpace(h))
				}
				return
			}
		}
		fmt.Fprintf(os.Stderr, "no help for %q\n", topic)
		return
	}
	fmt.Printf(`cav %s: drive Cavalry from the shell.

Usage: cav <command> [arguments] [--json]

Commands:
`, version)
	cs := append([]command(nil), commands...)
	sort.SliceStable(cs, func(i, j int) bool { return order(cs[i].name) < order(cs[j].name) })
	for _, c := range cs {
		fmt.Printf("  %-8s %s\n", c.name, firstLine(c.summary))
	}
	fmt.Print(`
Every command accepts --json (machine-readable output on stdout).
Exit codes: 0 ok, 1 error, 2 bridge not reachable, 3 job still running, 4 bridge lost.
Run "cav help <command>" for details.
`)
}

var helpOrder = []string{"setup", "doctor", "status", "run", "job", "scene", "tree", "layer", "frame", "sheet", "render", "beats", "api", "docs", "helpers", "relay", "version", "update"}

func order(name string) int {
	for i, n := range helpOrder {
		if n == name {
			return i
		}
	}
	return len(helpOrder)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

var longHelp = map[string]string{}
