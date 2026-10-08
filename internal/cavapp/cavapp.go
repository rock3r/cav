// Package cavapp checks whether the Cavalry app itself runs. When cav cannot reach
// cav-bridge, this tells "Cavalry is not running (it may have crashed)" apart from
// "Cavalry runs, but nobody started the cav-bridge script".
package cavapp

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Status int

const (
	// Unknown means cav could not check, for example on an unsupported system.
	Unknown Status = iota
	Running
	NotRunning
)

// Probe reports whether the Cavalry app process runs. Tests replace it.
var Probe = probe

// CrashDir is where macOS writes crash reports. Tests replace it.
var CrashDir = defaultCrashDir()

func defaultCrashDir() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "Logs", "DiagnosticReports")
}

func probe() Status {
	switch runtime.GOOS {
	case "darwin":
		// The executable is Cavalry.app/Contents/MacOS/Cavalry. Match the name exactly:
		// `pgrep -f cavalry` also matches unrelated tools such as the cavalry-mcp server.
		err := exec.Command("pgrep", "-x", "Cavalry").Run()
		return pgrepStatus(err)
	case "windows":
		out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq Cavalry.exe", "/FO", "CSV", "/NH").Output()
		if err != nil {
			return Unknown
		}
		return tasklistStatus(string(out))
	}
	return Unknown
}

// pgrepStatus reads the pgrep exit code: 0 found a process, 1 found none, other codes are errors.
func pgrepStatus(err error) Status {
	if err == nil {
		return Running
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 {
		return NotRunning
	}
	return Unknown
}

// tasklistStatus reads `tasklist /FO CSV /NH` output. With no match, tasklist prints an
// "INFO: No tasks" line instead of a CSV row.
func tasklistStatus(out string) Status {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), `"cavalry.exe"`) {
			return Running
		}
	}
	return NotRunning
}

// CrashReport is one Cavalry crash report file.
type CrashReport struct {
	Path string
	Time time.Time
}

// NewestCrashReport returns the newest Cavalry*.ips file in dir, or nil when there is none.
func NewestCrashReport(dir string) *CrashReport {
	if dir == "" {
		return nil
	}
	paths, _ := filepath.Glob(filepath.Join(dir, "Cavalry*.ips"))
	var newest *CrashReport
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil || fi.IsDir() {
			continue
		}
		if newest == nil || fi.ModTime().After(newest.Time) {
			newest = &CrashReport{Path: p, Time: fi.ModTime()}
		}
	}
	return newest
}

// Diagnosis says why cav-bridge does not answer and what to do about it.
type Diagnosis struct {
	Status Status
	// What states the case, for example "Cavalry is not running (it may have crashed)".
	// It is empty when the status is Unknown.
	What string
	// Fix says what to do, in the style of a `cav doctor` fix line.
	Fix string
	// Crash is the newest crash report when Cavalry is not running, or nil.
	Crash *CrashReport
}

// Diagnose checks the Cavalry process and builds the matching advice.
func Diagnose() Diagnosis {
	s := Probe()
	var crash *CrashReport
	if s == NotRunning {
		crash = NewestCrashReport(CrashDir)
	}
	return Explain(s, crash)
}

// Explain builds the advice for a status. It does no I/O.
func Explain(s Status, crash *CrashReport) Diagnosis {
	d := Diagnosis{Status: s}
	switch s {
	case NotRunning:
		d.What = "Cavalry is not running (it may have crashed)"
		d.Fix = "start Cavalry, then Scripts menu > cav-bridge, and keep its window open"
		d.Crash = crash
	case Running:
		d.What = "Cavalry is running but cav-bridge is not"
		d.Fix = "in Cavalry, open Scripts menu > cav-bridge, and keep its window open"
	default:
		d.Fix = "open Cavalry, then Scripts menu > cav-bridge, and keep its window open"
	}
	return d
}

// CrashNote names the crash report, or returns "" when there is none.
func (d Diagnosis) CrashNote() string {
	if d.Crash == nil {
		return ""
	}
	return "Newest Cavalry crash report: " + d.Crash.Path + " (" + d.Crash.Time.Format("2006-01-02 15:04") + ")."
}

// Hint is the advice as full sentences, for error messages.
func (d Diagnosis) Hint() string {
	var b strings.Builder
	if d.What != "" {
		b.WriteString(d.What + ". ")
	}
	b.WriteString(strings.ToUpper(d.Fix[:1]) + d.Fix[1:] + ".")
	if note := d.CrashNote(); note != "" {
		b.WriteString(" " + note)
	}
	return b.String()
}
