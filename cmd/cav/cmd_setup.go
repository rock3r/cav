package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/rock3r/cavalry-skill/assets"
	"github.com/rock3r/cavalry-skill/internal/config"
)

func init() {
	register(command{
		name:    "setup",
		args:    "[--no-docs]",
		summary: "Install or repair everything cav needs (safe to run again).",
		run:     func(a *app, args []string) error { return cmdCheck(a, args, true) },
	})
	longHelp["setup"] = `
Setup is idempotent. It:
  - creates the bridge token in ~/.cav/token (never printed, never committed);
  - installs or updates cav-bridge.js in Cavalry's Scripts folder;
  - checks that Cavalry and ffmpeg are installed;
  - builds the local docs index for "cav docs" (skip with --no-docs);
  - checks whether the bridge is running, and whether it runs the current version.
After setup, start the bridge once per Cavalry session: Scripts menu > cav-bridge.`
	register(command{
		name:    "doctor",
		args:    "",
		summary: "Check every prerequisite without changing anything.",
		run:     func(a *app, args []string) error { return cmdCheck(a, args, false) },
	})
}

type check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
	// Optional checks do not make the whole result fail.
	Optional bool `json:"optional,omitempty"`
}

func cmdCheck(a *app, args []string, fix bool) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	noDocs := fs.Bool("no-docs", false, "do not build the docs index")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	var checks []check

	// 1. Token.
	if fix {
		created, err := config.EnsureToken()
		switch {
		case err != nil:
			checks = append(checks, check{Name: "token", Detail: err.Error()})
		case created:
			checks = append(checks, check{Name: "token", OK: true, Detail: "created " + config.TokenPath()})
		default:
			checks = append(checks, check{Name: "token", OK: true, Detail: config.TokenPath()})
		}
	} else if _, err := config.ReadToken(); err != nil {
		checks = append(checks, check{Name: "token", Detail: err.Error(), Fix: "cav setup"})
	} else {
		checks = append(checks, check{Name: "token", OK: true, Detail: config.TokenPath()})
	}

	// 2. Bridge script in Cavalry's Scripts folder.
	target := filepath.Join(config.ScriptsDir(), config.BridgeFile)
	current, err := os.ReadFile(target)
	switch {
	case err == nil && bytes.Equal(current, assets.BridgeJS):
		checks = append(checks, check{Name: "bridge-script", OK: true, Detail: "installed: " + target})
	case fix:
		if err := os.MkdirAll(config.ScriptsDir(), 0o755); err != nil {
			checks = append(checks, check{Name: "bridge-script", Detail: err.Error()})
		} else if err := os.WriteFile(target, assets.BridgeJS, 0o644); err != nil {
			checks = append(checks, check{Name: "bridge-script", Detail: err.Error()})
		} else {
			verb := "installed"
			if len(current) > 0 {
				verb = "updated"
			}
			checks = append(checks, check{Name: "bridge-script", OK: true, Detail: verb + ": " + target})
		}
	case err == nil:
		checks = append(checks, check{Name: "bridge-script", Detail: "outdated: " + target, Fix: "cav setup"})
	default:
		checks = append(checks, check{Name: "bridge-script", Detail: "missing: " + target, Fix: "cav setup"})
	}
	_ = os.MkdirAll(config.JobsDir(), 0o700)

	// 3. Cavalry itself.
	checks = append(checks, cavalryInstallCheck())

	// 4. ffmpeg and ffprobe (needed for full renders with audio and for `cav beats`).
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if p, err := exec.LookPath(tool); err == nil {
			checks = append(checks, check{Name: tool, OK: true, Detail: p})
		} else {
			checks = append(checks, check{Name: tool, Detail: "not found on PATH", Fix: ffmpegFix()})
		}
	}

	// 5. Docs index.
	checks = append(checks, docsCheck(fix && !*noDocs))

	// 6. Running bridge.
	checks = append(checks, bridgeCheck(a))

	allOK := true
	for _, c := range checks {
		if !c.OK && !c.Optional {
			allOK = false
		}
	}
	a.emit(map[string]any{"ready": allOK, "checks": checks, "version": version}, func() {
		for _, c := range checks {
			mark := "ok  "
			if !c.OK {
				mark = "FAIL"
				if c.Optional {
					mark = "warn"
				}
			}
			fmt.Printf("%s %-14s %s\n", mark, c.Name, c.Detail)
			if !c.OK && c.Fix != "" {
				fmt.Printf("     %-14s fix: %s\n", "", c.Fix)
			}
		}
		if allOK {
			fmt.Println("\nready: cav can drive Cavalry.")
		} else {
			fmt.Println("\nnot ready: fix the lines marked FAIL, then run `cav doctor` again.")
		}
	})
	if !allOK && a.json {
		// JSON output already carries ready=false; keep the exit code useful too.
		return &cliError{code: exitError, msg: "not ready", data: map[string]any{"checks": checks}}
	}
	return nil
}

func ffmpegFix() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install ffmpeg"
	case "windows":
		return "winget install Gyan.FFmpeg"
	}
	return "install ffmpeg with your package manager"
}

var plistVersion = regexp.MustCompile(`<key>CFBundleShortVersionString</key>\s*<string>([^<]+)</string>`)

func cavalryInstallCheck() check {
	switch runtime.GOOS {
	case "darwin":
		plist := "/Applications/Cavalry.app/Contents/Info.plist"
		b, err := os.ReadFile(plist)
		if err != nil {
			return check{Name: "cavalry", Detail: "Cavalry.app not found in /Applications", Fix: "install Cavalry from https://cavalry.studio"}
		}
		v := "unknown version"
		if m := plistVersion.FindSubmatch(b); m != nil {
			v = string(m[1])
		}
		return versionCheck(v)
	case "windows":
		for _, p := range []string{`C:\Program Files\Cavalry\Cavalry.exe`, filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Cavalry", "Cavalry.exe")} {
			if _, err := os.Stat(p); err == nil {
				return check{Name: "cavalry", OK: true, Detail: "found " + p + " (version is checked when the bridge runs)"}
			}
		}
		return check{Name: "cavalry", Detail: "Cavalry.exe not found in the usual places", Fix: "install Cavalry from https://cavalry.studio", Optional: true}
	}
	return check{Name: "cavalry", Detail: "Cavalry supports macOS and Windows only", Optional: true}
}

// Tested against 2.7.2. The bridge needs 2.4 or newer.
const testedCavalry = "2.7.2"

func versionCheck(v string) check {
	c := check{Name: "cavalry", OK: true, Detail: "version " + v}
	if versionLess(v, "2.4.0") {
		c.OK = false
		c.Detail += " is too old (needs 2.4.0 or newer)"
		c.Fix = "update Cavalry"
	} else if v != testedCavalry {
		c.Detail += " (cav is tested on " + testedCavalry + ")"
	}
	return c
}

func versionLess(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		var x, y int
		if i < len(pa) {
			fmt.Sscanf(pa[i], "%d", &x)
		}
		if i < len(pb) {
			fmt.Sscanf(pb[i], "%d", &y)
		}
		if x != y {
			return x < y
		}
	}
	return false
}

func bridgeVersionFromJS() string {
	m := regexp.MustCompile(`BRIDGE_VERSION = '([^']+)'`).FindSubmatch(assets.BridgeJS)
	if m == nil {
		return "?"
	}
	return string(m[1])
}

func bridgeCheck(a *app) check {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	c := newClient()
	if c.Spool != "" {
		return check{Name: "bridge", OK: true, Optional: true, Detail: "spool mode (" + c.Spool + "); `cav relay` forwards jobs outside the sandbox"}
	}
	payload, err := c.Probe(ctx)
	if err != nil && strings.Contains(err.Error(), "no answer within") {
		return check{Name: "bridge", Detail: fmt.Sprintf("listening on %s:%d but not answering (Cavalry is busy: a long script, a render or an open dialog box)", c.Host, c.Port),
			Fix: "wait for the running script or render to finish, or close any open dialog box in Cavalry, then try again"}
	}
	if err != nil {
		return check{Name: "bridge", Detail: fmt.Sprintf("not running on %s:%d", c.Host, c.Port),
			Fix: "open Cavalry, then Scripts menu > cav-bridge, and keep its window open"}
	}
	want := bridgeVersionFromJS()
	got, _ := payload["bridgeVersion"].(string)
	if got == "" {
		return check{Name: "bridge", Detail: "something answers on the port but it is not cav-bridge", Fix: "close the other program or set CAV_BRIDGE_PORT"}
	}
	if got != want {
		// The request protocol has not changed since 0.1, so an older running bridge still
		// works; restarting it only brings its improvements.
		return check{Name: "bridge", OK: true, Optional: true, Detail: fmt.Sprintf("running v%s; v%s is installed (works; restart the cav-bridge window to update)", got, want),
			Fix: "close the cav-bridge window in Cavalry and start it again from the Scripts menu"}
	}
	return check{Name: "bridge", OK: true, Detail: fmt.Sprintf("running v%s on %s:%d", got, c.Host, c.Port)}
}
