package cavapp

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExplain(t *testing.T) {
	crash := &CrashReport{Path: "/logs/Cavalry-2026-10-08-120000.ips", Time: time.Date(2026, 10, 8, 12, 0, 0, 0, time.Local)}
	for _, tc := range []struct {
		name  string
		s     Status
		crash *CrashReport
		want  []string
		not   []string
	}{
		{"not running, no crash report", NotRunning, nil,
			[]string{"Cavalry is not running (it may have crashed)", "Start Cavalry, then Scripts menu > cav-bridge"},
			[]string{"crash report"}},
		{"not running, with crash report", NotRunning, crash,
			[]string{"Cavalry is not running", "Newest Cavalry crash report: /logs/Cavalry-2026-10-08-120000.ips (2026-10-08 12:00)."},
			nil},
		{"running", Running, nil,
			[]string{"Cavalry is running but cav-bridge is not", "In Cavalry, open Scripts menu > cav-bridge"},
			[]string{"not running (", "Start Cavalry"}},
		{"running ignores a crash report", Running, crash,
			[]string{"Cavalry is running but cav-bridge is not"},
			[]string{"crash report"}},
		{"unknown keeps the general advice", Unknown, nil,
			[]string{"Open Cavalry, then Scripts menu > cav-bridge"},
			[]string{"is running", "not running"}},
	} {
		got := Explain(tc.s, tc.crash).Hint()
		for _, w := range tc.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: %q does not contain %q", tc.name, got, w)
			}
		}
		for _, n := range tc.not {
			if strings.Contains(got, n) {
				t.Errorf("%s: %q contains %q", tc.name, got, n)
			}
		}
	}
}

func TestDiagnoseUsesProbe(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Cavalry-2026-10-08-120000.ips"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	oldProbe, oldDir := Probe, CrashDir
	t.Cleanup(func() { Probe, CrashDir = oldProbe, oldDir })
	CrashDir = dir

	Probe = func() Status { return NotRunning }
	if d := Diagnose(); d.Status != NotRunning || d.Crash == nil {
		t.Errorf("not running: %+v", d)
	}
	Probe = func() Status { return Running }
	if d := Diagnose(); d.Status != Running || d.Crash != nil {
		t.Errorf("running: %+v", d)
	}
}

func TestNewestCrashReport(t *testing.T) {
	if NewestCrashReport("") != nil || NewestCrashReport(filepath.Join(t.TempDir(), "missing")) != nil {
		t.Error("a missing folder must give no report")
	}
	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)
	for i, name := range []string{"Cavalry-old.ips", "Cavalry-new.ips", "Other-newest.ips", "Cavalry-newest.txt"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		mt := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
	got := NewestCrashReport(dir)
	if got == nil || filepath.Base(got.Path) != "Cavalry-new.ips" {
		t.Errorf("newest = %+v, want Cavalry-new.ips", got)
	}
}

func TestPgrepStatus(t *testing.T) {
	if pgrepStatus(nil) != Running {
		t.Error("exit 0 means a process matched")
	}
	if err := exec.Command("sh", "-c", "exit 1").Run(); pgrepStatus(err) != NotRunning {
		t.Errorf("exit 1 means no process matched: %v", err)
	}
	if err := exec.Command("sh", "-c", "exit 3").Run(); pgrepStatus(err) != Unknown {
		t.Errorf("other exit codes are errors: %v", err)
	}
	if pgrepStatus(errors.New("pgrep not found")) != Unknown {
		t.Error("a missing pgrep is unknown")
	}
}

func TestTasklistStatus(t *testing.T) {
	if s := tasklistStatus("\"Cavalry.exe\",\"4242\",\"Console\",\"1\",\"812,004 K\"\r\n"); s != Running {
		t.Errorf("CSV row: %v", s)
	}
	if s := tasklistStatus("INFO: No tasks are running which match the specified criteria.\r\n"); s != NotRunning {
		t.Errorf("no match: %v", s)
	}
}
