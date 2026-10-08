package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rock3r/cav/internal/bridge"
	"github.com/rock3r/cav/internal/config"
)

// Commands that never need Cavalry or files outside the working directory run locally,
// even in spool mode. Everything else is forwarded to `cav relay`.
var localCommands = map[string]bool{"help": true, "-h": true, "--help": true, "version": true, "-v": true, "--version": true, "helpers": true, "guide": true, "api": true, "types": true, "relay": true, "mcp": true}

// Commands the relay refuses to run for a spool client.
var relayDenied = map[string]bool{"setup": true, "update": true, "relay": true, "mcp": true}

type forwardReq struct {
	ID    string   `json:"id"`
	Argv  []string `json:"argv"`
	Cwd   string   `json:"cwd"`
	Stdin []byte   `json:"stdin,omitempty"`
}

type forwardRes struct {
	Exit   int    `json:"exit"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

// forward sends the whole command line to the relay and replays its output.
func forward(spool string, argv []string) int {
	id := bridge.NewID()
	cwd, _ := os.Getwd()
	req := forwardReq{ID: id, Argv: argv, Cwd: cwd}
	for _, a := range argv {
		if a == "-" {
			req.Stdin, _ = io.ReadAll(os.Stdin)
		}
	}
	if err := os.MkdirAll(spool, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "error: spool folder:", err)
		return exitError
	}
	b, _ := json.Marshal(req)
	tmp := filepath.Join(spool, id+".cmd.json.tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil || os.Rename(tmp, filepath.Join(spool, id+".cmd.json")) != nil {
		fmt.Fprintln(os.Stderr, "error: cannot write to the spool folder", spool)
		return exitError
	}
	outPath := filepath.Join(spool, id+".out.json")
	start := time.Now()
	warned := false
	for {
		if b, err := os.ReadFile(outPath); err == nil {
			var res forwardRes
			if json.Unmarshal(b, &res) == nil {
				_ = os.Remove(outPath)
				// Progress lines and warnings (stderr) were printed before the result (stdout).
				// Keep that order: a "job running" line after the result reads as "not done".
				os.Stderr.WriteString(res.Stderr)
				os.Stdout.WriteString(res.Stdout)
				return res.Exit
			}
		}
		if !warned && time.Since(start) > 10*time.Second {
			if _, err := os.Stat(filepath.Join(spool, id+".cmd.json")); err == nil {
				fmt.Fprintf(os.Stderr, "cav: waiting for `cav relay --spool %s` to pick up the command\n", spool)
				warned = true
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// spoolFor returns the spool folder if this invocation must be forwarded.
func spoolFor(args []string) string {
	if len(args) == 0 || localCommands[args[0]] {
		return ""
	}
	if os.Getenv("CAV_RELAYED") == "1" {
		return "" // we are the relay's own child process
	}
	return config.SpoolDir()
}
