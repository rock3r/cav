package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rock3r/cavalry-skill/assets"
	"github.com/rock3r/cavalry-skill/internal/bridge"
	"github.com/rock3r/cavalry-skill/internal/config"
)

// parseFlags parses flags that may appear before, between or after positional arguments.
func parseFlags(fs *flag.FlagSet, args []string) ([]string, error) {
	fs.SetOutput(io.Discard)
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, usageErr("%s: %v", fs.Name(), err)
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		if args[0] == "--" {
			return append(pos, args[1:]...), nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

type execOpts struct {
	helpers  bool          // preload the helper library
	timeout  time.Duration // how long to wait before reporting "still running"
	async    bool          // submit and return the job id at once
	source   string        // source name for error messages
	progress bool          // print state changes to stderr
}

// helpersVersion identifies the embedded helper library by content.
func helpersVersion() string {
	sum := sha256.Sum256(assets.HelpersJS)
	return helpersSemver() + "+" + hex.EncodeToString(sum[:])[:10]
}

func helpersSemver() string {
	s := string(assets.HelpersJS)
	if i := strings.Index(s, "VERSION = '"); i >= 0 {
		rest := s[i+len("VERSION = '"):]
		if j := strings.IndexByte(rest, '\''); j >= 0 {
			return rest[:j]
		}
	}
	return "0"
}

// jobDir is where the CLI writes job files that Cavalry reads.
func jobDir(c *bridge.Client) string {
	if c.Spool != "" {
		return c.Spool
	}
	return config.JobsDir()
}

// ensurePreload writes the embedded helpers where the bridge can read them.
func ensurePreload(dir string) (string, error) {
	path := filepath.Join(dir, "cav-helpers-"+strings.ReplaceAll(helpersVersion(), "+", "-")+".js")
	if b, err := os.ReadFile(path); err == nil && string(b) == string(assets.HelpersJS) {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, assets.HelpersJS, 0o600)
}

type jobOutcome struct {
	id     string
	result *bridge.Result
	source string
	code   string
}

// execJS runs code inside Cavalry and waits for it.
func (a *app) execJS(code string, o execOpts) (*jobOutcome, error) {
	c := bridge.New()
	dir := jobDir(c)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	req := &bridge.Request{ID: bridge.NewID(), Restricted: os.Getenv("CAV_RESTRICTED") == "1"}
	jsPath := filepath.Join(dir, req.ID+".js")
	if err := os.WriteFile(jsPath, []byte(code), 0o600); err != nil {
		return nil, err
	}
	req.File = jsPath
	if o.helpers {
		p, err := ensurePreload(dir)
		if err != nil {
			return nil, err
		}
		req.Preload = p
		req.PreloadVersion = helpersVersion()
	}
	if a.logJob == nil {
		a.logJob = map[string]any{}
	}
	a.logJob["job"] = req.ID
	ctx := context.Background()
	if c.Spool == "" {
		if _, err := c.Probe(ctx); err != nil {
			_ = os.Remove(jsPath)
			return nil, err
		}
	}
	if err := c.Submit(ctx, req); err != nil {
		_ = os.Remove(jsPath)
		return nil, err
	}
	out := &jobOutcome{id: req.ID, source: o.source, code: code}
	if o.async {
		return out, nil
	}
	return a.waitJob(c, out, o.timeout, o.progress)
}

func (a *app) waitJob(c *bridge.Client, out *jobOutcome, timeout time.Duration, progress bool) (*jobOutcome, error) {
	started := time.Now()
	onState := func(s bridge.State) {
		if progress && !a.json && s == bridge.StateRunning {
			fmt.Fprintf(os.Stderr, "cav: job %s running\n", out.id)
		}
		if !a.json && s == bridge.StateBusy {
			fmt.Fprintf(os.Stderr, "cav: Cavalry is busy (not answering); still waiting for job %s\n", out.id)
		}
	}
	res, err := c.Wait(context.Background(), out.id, timeout, onState)
	if err != nil {
		if errors.Is(err, bridge.ErrStillRunning) {
			return nil, &cliError{
				code: exitStillRunning,
				msg:  err.Error(),
				hint: fmt.Sprintf("The job keeps running inside Cavalry. This is not a failure. Wait for it with `cav job wait %s`.", out.id),
				data: map[string]any{"job": out.id, "status": "running", "waitedSeconds": int(time.Since(started).Seconds())},
			}
		}
		return nil, err
	}
	out.result = res
	bridge.Cleanup(out.id)
	if c.Spool != "" {
		_ = os.Remove(filepath.Join(c.Spool, out.id+".res.json"))
		_ = os.Remove(filepath.Join(c.Spool, out.id+".state"))
		_ = os.Remove(filepath.Join(c.Spool, out.id+".js"))
	}
	if a.logJob != nil {
		a.logJob["jobOk"] = res.OK
		a.logJob["jobMs"] = res.MS
		if res.Error != nil {
			a.logJob["jobError"] = res.Error.Message
		}
	}
	return out, nil
}

// scriptErr turns a failed job into a CLI error that points at the failing line.
func scriptErr(o *jobOutcome) error {
	e := o.result.Error
	msg := e.Message
	data := map[string]any{"job": o.id, "logs": o.result.Logs}
	hint := ""
	if e.Where != nil && e.Where.Line > 0 {
		lines := strings.Split(o.code, "\n")
		data["line"] = e.Where.Line
		if e.Where.Line <= len(lines) {
			src := strings.TrimSpace(lines[e.Where.Line-1])
			data["sourceLine"] = src
			msg = fmt.Sprintf("%s (%s line %d: %s)", msg, o.source, e.Where.Line, src)
		}
	}
	hint = errorHint(e.Message)
	return &cliError{code: exitError, msg: "script error: " + msg, hint: hint, data: data}
}

// errorHint maps common script errors to the fix an agent most likely needs.
func errorHint(msg string) string {
	m := strings.ToLower(msg)
	switch {
	case strings.Contains(m, "is not defined") && strings.Contains(m, "cav"):
		return "The helper library was not loaded. Do not pass --no-helpers, or run `cav helpers` to see its API."
	case strings.Contains(m, "is not defined"):
		return "Variables do not persist between `cav run` calls unless you store them on globalThis. Look up API names with `cav api <word>`."
	case strings.Contains(m, "invalid attribute") || strings.Contains(m, "attribute") && strings.Contains(m, "not"):
		return "Check the attribute path with `cav layer <layerId> --attrs`, or search `cav docs <node name>`."
	case strings.Contains(m, "is not a function"):
		return "Look up the exact function name with `cav api <word>`."
	}
	return ""
}

func valueOf(o *jobOutcome) any {
	var v any
	if len(o.result.Value) > 0 {
		_ = json.Unmarshal(o.result.Value, &v)
	}
	return v
}

// jsCall runs internal CLI code (no helpers) and decodes the returned value into dst.
func (a *app) jsCall(code string, timeout time.Duration, dst any) error {
	o, err := a.execJS(code, execOpts{timeout: timeout, source: "cav"})
	if err != nil {
		return err
	}
	if !o.result.OK {
		return scriptErr(o)
	}
	if dst == nil {
		return nil
	}
	return json.Unmarshal(o.result.Value, dst)
}

func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
