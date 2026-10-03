package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rock3r/cav/internal/bridge"
	"github.com/rock3r/cav/internal/config"
	"github.com/rock3r/cav/internal/operation"
)

func TestBlockedScriptInputHonorsDeadlineAndReleasesLock(t *testing.T) {
	for _, source := range []string{"stdin", "named pipe"} {
		t.Run(source, func(t *testing.T) {
			posts := 0
			fixtureBridge(t, func(req bridge.Request) { posts++ })
			input := "-"
			var reader, writer *os.File
			if source == "stdin" {
				var err error
				reader, writer, err = os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				original := os.Stdin
				os.Stdin = reader
				t.Cleanup(func() { os.Stdin = original; reader.Close(); writer.Close() })
			} else {
				mkfifo, err := exec.LookPath("mkfifo")
				if err != nil {
					t.Skip("POSIX named-pipe fixture requires mkfifo")
				}
				input = filepath.Join(t.TempDir(), "script.js")
				if output, err := exec.CommandContext(t.Context(), mkfifo, input).CombinedOutput(); err != nil {
					t.Fatalf("mkfifo: %s: %v", output, err)
				}
			}
			a := &app{json: true}
			if code := a.dispatch([]string{"run", input, "--no-helpers", "--timeout", "350ms"}); code != exitStillRunning {
				t.Fatalf("exit=%d", code)
			}
			if posts != 0 || len(a.op.Jobs) != 0 || a.op.FailureReason != "wait-timeout" || a.op.Status == "failed" {
				t.Fatalf("posts=%d record=%+v", posts, a.op)
			}
			unlock, err := operation.Lock(config.Home())
			if err != nil {
				t.Fatalf("timed-out input retained the operation lock: %v", err)
			}
			unlock()
			if code := (&app{json: true}).dispatch([]string{"operation", "resume", a.op.ID, "--timeout", "1s"}); code != exitError {
				t.Fatalf("incomplete input was resumed: exit=%d", code)
			}
			if posts != 0 {
				t.Fatal("incomplete input submitted native work")
			}
			if source == "stdin" {
				// Cancellation kills only the owned helper. The borrowed stdin pipe still
				// works, with no abandoned reader to consume input after the command exits.
				if _, err := writer.Write([]byte("after")); err != nil {
					t.Fatal(err)
				}
				b := make([]byte, 5)
				if _, err := reader.Read(b); err != nil || string(b) != "after" {
					t.Fatalf("stdin not preserved: %q %v", b, err)
				}
			}
		})
	}
}

func TestCompletedStdinSubmitsOriginalScriptExactlyOnce(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdin
	os.Stdin = reader
	t.Cleanup(func() { os.Stdin = original; reader.Close(); writer.Close() })
	if _, err := writer.Write([]byte("return 5")); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	posts := 0
	fixtureBridge(t, func(req bridge.Request) {
		posts++
		code, err := os.ReadFile(req.File)
		if err != nil || string(code) != "return 5" {
			t.Errorf("script=%q err=%v", code, err)
		}
		fixtureResult(t, req.ID, 5)
	})
	a := &app{json: true}
	if code := a.dispatch([]string{"run", "-", "--no-helpers", "--timeout", "3s"}); code != exitOK {
		t.Fatal(code)
	}
	if posts != 1 {
		t.Fatalf("posts=%d", posts)
	}
}
