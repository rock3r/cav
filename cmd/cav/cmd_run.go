package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/rock3r/cavalry-skill/internal/bridge"
)

func init() {
	register(command{
		name:    "run",
		args:    "<file.js>... | -e <code> | -  [--timeout 10m] [--async] [--no-helpers]",
		summary: "Run JavaScript inside Cavalry and print its return value.",
		run:     cmdRun,
	})
	longHelp["run"] = `
The code runs inside a function, so use "return" to send a value back.
Several files run in order as one job. "-" reads the code from stdin.

The helper library (global "cav", see "cav helpers") is loaded before your code.
It is loaded once per bridge session and reloaded when its version changes.
Values on globalThis survive between runs until the bridge restarts.

A long job is not a failed job. When --timeout passes, cav exits with code 3 and
prints the job id; the job keeps running in Cavalry. Wait for it with
"cav job wait <id>". Use --async to return the job id at once.

Examples:
  cav run build.js
  cav run -e "return api.getCompLayers(true).length"
  cav run scene1.js scene2.js --timeout 30m`
	register(command{
		name:    "job",
		args:    "wait <id> [--timeout 30m]",
		summary: "Wait for a job that is still running and print its result.",
		run:     cmdJob,
	})
}

func cmdRun(a *app, args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	expr := fs.String("e", "", "code to run")
	timeout := fs.Duration("timeout", 10*time.Minute, "wait this long before reporting the job as still running")
	async := fs.Bool("async", false, "return the job id without waiting")
	noHelpers := fs.Bool("no-helpers", false, "do not preload the helper library")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	var code, source string
	switch {
	case *expr != "":
		code, source = *expr, "-e"
	case len(pos) == 1 && pos[0] == "-":
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		code, source = string(b), "stdin"
	case len(pos) > 0:
		var parts []string
		for _, p := range pos {
			b, err := os.ReadFile(p)
			if err != nil {
				return usageErr("cannot read %s: %v", p, err)
			}
			parts = append(parts, string(b))
		}
		code, source = strings.Join(parts, "\n;\n"), strings.Join(pos, "+")
	default:
		return usageErr("run needs a file, -e <code>, or - for stdin")
	}
	o, err := a.execJS(code, execOpts{helpers: !*noHelpers, timeout: *timeout, async: *async, source: source, progress: true})
	if err != nil {
		return err
	}
	if *async {
		a.emit(map[string]any{"job": o.id, "status": "submitted"}, func() {
			fmt.Printf("submitted job %s\nwait for it with: cav job wait %s\n", o.id, o.id)
		})
		return nil
	}
	return a.printResult(o)
}

func (a *app) printResult(o *jobOutcome) error {
	r := o.result
	if !r.OK {
		if !a.json {
			printLogs(r.Logs)
		}
		return scriptErr(o)
	}
	a.emit(map[string]any{"job": o.id, "value": valueOf(o), "logs": r.Logs, "ms": r.MS}, func() {
		printLogs(r.Logs)
		if len(r.Value) > 0 && string(r.Value) != "null" {
			fmt.Println(prettyJSON(r.Value))
		} else {
			fmt.Printf("ok (%d ms)\n", r.MS)
		}
	})
	return nil
}

func printLogs(logs []bridge.LogLine) {
	for _, l := range logs {
		if l.Level == "info" && strings.HasPrefix(l.Message, "cav: loaded helpers") {
			continue
		}
		prefix := ""
		if l.Level == "warn" || l.Level == "error" {
			prefix = l.Level + ": "
		}
		fmt.Fprintln(os.Stderr, prefix+l.Message)
	}
}

func cmdJob(a *app, args []string) error {
	fs := flag.NewFlagSet("job", flag.ContinueOnError)
	timeout := fs.Duration("timeout", 30*time.Minute, "wait this long before reporting the job as still running")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 2 || pos[0] != "wait" {
		return usageErr("usage: cav job wait <id>")
	}
	c := bridge.New()
	if c.Spool == "" {
		if _, err := c.Probe(context.Background()); err != nil {
			return err
		}
	}
	o, err := a.waitJob(c, &jobOutcome{id: pos[1], source: "job " + pos[1]}, *timeout, true)
	if err != nil {
		return err
	}
	return a.printResult(o)
}
