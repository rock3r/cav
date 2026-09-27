package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rock3r/cavalry-skill/internal/bridge"
)

func init() {
	register(command{
		name:    "relay",
		args:    "--spool <dir> [--once] [--restricted]",
		summary: "Forward jobs from a spool folder to the bridge (for agents in a sandbox).",
		run:     cmdRelay,
	})
	longHelp["relay"] = `
Some agent sandboxes block connections to 127.0.0.1, so cav inside them cannot reach
the bridge. Run the relay OUTSIDE the sandbox, and give the agent CAV_SPOOL=<dir>:

  cav relay --spool /path/the/agent/can/write      (outside the sandbox)
  CAV_SPOOL=/path/the/agent/can/write cav run x.js  (inside the sandbox)

The agent's cav writes <id>.req.json into the folder; the relay sends the job to the
bridge and writes <id>.res.json back. Job files must be in a folder Cavalry can read.

Not a security boundary: a job can use every Cavalry scripting API, which can read
and write files. --restricted makes the bridge disable api.runProcess and similar
calls during relayed jobs, to catch accidents, not attacks.`
}

func cmdRelay(a *app, args []string) error {
	fs := flag.NewFlagSet("relay", flag.ContinueOnError)
	spool := fs.String("spool", "", "folder to watch")
	once := fs.Bool("once", false, "process the waiting jobs, then exit")
	restricted := fs.Bool("restricted", false, "disable process-launching APIs during relayed jobs")
	timeout := fs.Duration("timeout", 2*time.Hour, "longest time one job may run")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	if *spool == "" {
		return usageErr("relay needs --spool <dir>")
	}
	dir, _ := filepath.Abs(*spool)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	c := bridge.New()
	c.Spool = "" // the relay itself talks HTTP
	if _, err := c.Probe(context.Background()); err != nil {
		return err
	}
	if !a.json {
		fmt.Fprintf(os.Stderr, "cav relay: watching %s (Ctrl+C to stop)\n", dir)
	}
	handled := 0
	for {
		reqs, _ := filepath.Glob(filepath.Join(dir, "*.req.json"))
		cmds, _ := filepath.Glob(filepath.Join(dir, "*.cmd.json"))
		all := append(reqs, cmds...)
		sort.Slice(all, func(i, j int) bool { return filepath.Base(all[i]) < filepath.Base(all[j]) })
		for _, p := range all {
			if strings.HasSuffix(p, ".cmd.json") {
				relayCommand(p, *restricted, *timeout)
			} else {
				relayOne(c, p, *restricted, *timeout)
			}
			handled++
		}
		if *once {
			a.emit(map[string]any{"handled": handled}, func() { fmt.Printf("relayed %d jobs\n", handled) })
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func relayOne(c *bridge.Client, reqPath string, restricted bool, timeout time.Duration) {
	taken := strings.TrimSuffix(reqPath, ".req.json") + ".taken"
	if err := os.Rename(reqPath, taken); err != nil {
		return // another relay took it
	}
	defer os.Remove(taken)
	base := strings.TrimSuffix(reqPath, ".req.json")
	writeRes := func(r *bridge.Result) {
		b, _ := json.Marshal(r)
		_ = os.WriteFile(base+".res.json.tmp", b, 0o600)
		_ = os.Rename(base+".res.json.tmp", base+".res.json")
		_ = os.Remove(base + ".state")
	}
	failRes := func(id, msg string) {
		writeRes(&bridge.Result{Type: "result", ID: id, OK: false, Error: &bridge.ScriptError{Message: "cav relay: " + msg}})
	}
	b, err := os.ReadFile(taken)
	if err != nil {
		return
	}
	var req bridge.Request
	if err := json.Unmarshal(b, &req); err != nil || req.ID == "" {
		failRes(filepath.Base(base), "bad request file")
		return
	}
	// Only accept job files that live in the spool folder.
	dir := filepath.Dir(reqPath)
	for _, p := range []string{req.File, req.Preload} {
		if p == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil || filepath.Dir(abs) != dir {
			failRes(req.ID, "job files must be inside the spool folder: "+p)
			return
		}
	}
	req.Restricted = restricted
	ctx := context.Background()
	if err := c.Submit(ctx, &req); err != nil {
		failRes(req.ID, err.Error())
		return
	}
	res, err := c.Wait(ctx, req.ID, timeout, func(s bridge.State) {
		_ = os.WriteFile(base+".state", []byte(s), 0o600)
	})
	if err != nil {
		failRes(req.ID, err.Error())
		return
	}
	bridge.Cleanup(req.ID)
	writeRes(res)
}

// relayCommand runs one forwarded cav command line outside the sandbox.
func relayCommand(cmdPath string, restricted bool, timeout time.Duration) {
	taken := strings.TrimSuffix(cmdPath, ".cmd.json") + ".cmdtaken"
	if err := os.Rename(cmdPath, taken); err != nil {
		return
	}
	defer os.Remove(taken)
	base := strings.TrimSuffix(cmdPath, ".cmd.json")
	write := func(r forwardRes) {
		b, _ := json.Marshal(r)
		_ = os.WriteFile(base+".out.json.tmp", b, 0o600)
		_ = os.Rename(base+".out.json.tmp", base+".out.json")
	}
	b, err := os.ReadFile(taken)
	var req forwardReq
	if err != nil || json.Unmarshal(b, &req) != nil || len(req.Argv) == 0 {
		write(forwardRes{Exit: exitError, Stderr: "cav relay: bad command file\n"})
		return
	}
	name := req.Argv[0]
	for _, a := range req.Argv {
		if !strings.HasPrefix(a, "-") {
			name = a
			break
		}
	}
	if relayDenied[name] {
		write(forwardRes{Exit: exitError, Stderr: "cav relay: `cav " + name + "` is not allowed through the relay; ask the user to run it outside the sandbox\n"})
		return
	}
	if st, err := os.Stat(req.Cwd); err != nil || !st.IsDir() {
		write(forwardRes{Exit: exitError, Stderr: "cav relay: working folder does not exist: " + req.Cwd + "\n"})
		return
	}
	self, _ := os.Executable()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, self, req.Argv...)
	cmd.Dir = req.Cwd
	cmd.Env = append(os.Environ(), "CAV_RELAYED=1")
	if restricted {
		cmd.Env = append(cmd.Env, "CAV_RESTRICTED=1")
	}
	if len(req.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(req.Stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	code := 0
	if err != nil {
		code = exitError
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
	}
	write(forwardRes{Exit: code, Stdout: stdout.String(), Stderr: stderr.String()})
}
