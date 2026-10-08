package cavapp

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// With fakeCavalryEnv set, the test binary acts as a fake Cavalry: it only waits.
const fakeCavalryEnv = "CAVAPP_FAKE_CAVALRY"

func TestMain(m *testing.M) {
	if os.Getenv(fakeCavalryEnv) == "1" {
		time.Sleep(2 * time.Minute)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// TestProbeFindsFakeCavalry runs a copy of the test binary as a fake Cavalry, then
// checks that the real pgrep or tasklist call sees it start and stop. The fake has its
// own name, so a real Cavalry on the test machine does not change the result.
func TestProbeFindsFakeCavalry(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("cav checks the Cavalry process on macOS and Windows only")
	}
	old := exeName
	t.Cleanup(func() { exeName = old })
	exeName = "CavFakeTest" // macOS truncates process names to 16 characters
	name := exeName
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if s := probe(); s != NotRunning {
		t.Fatalf("probe before the fake starts = %v, want NotRunning", s)
	}

	fake := filepath.Join(t.TempDir(), name)
	copyFile(t, os.Args[0], fake)
	cmd := exec.Command(fake)
	cmd.Env = append(os.Environ(), fakeCavalryEnv+"=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	stopped := false
	defer func() {
		if !stopped {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	waitFor(t, Running)
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	stopped = true
	waitFor(t, NotRunning)
}

func waitFor(t *testing.T, want Status) {
	t.Helper()
	var got Status
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if got = probe(); got == want {
			return
		}
	}
	t.Fatalf("probe = %v, want %v", got, want)
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	in, err := os.Open(from)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}
