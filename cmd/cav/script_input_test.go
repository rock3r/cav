package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

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

func TestLiteralHyphenScriptPathDoesNotReadStdin(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("-", []byte("return 9"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	data, err := readScriptInput(ctx, "-")
	if err != nil || string(data) != "return 9" {
		t.Fatalf("literal file input=%q err=%v", data, err)
	}
}

func TestAudioDigestUsesCompleteContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audio")
	if err := os.WriteFile(path, []byte("abc"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	digest, err := fileDigest(ctx, path)
	if err != nil || digest != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("digest=%q err=%v", digest, err)
	}
}

func TestAudioHashDeadlineStopsBlockedPreparation(t *testing.T) {
	for _, stalled := range []bool{false, true} {
		t.Run(map[bool]string{false: "no writer", true: "stalled stream"}[stalled], func(t *testing.T) {
			mkfifo, err := exec.LookPath("mkfifo")
			if err != nil {
				t.Skip("POSIX FIFO fixture requires mkfifo")
			}
			path := filepath.Join(t.TempDir(), "audio.wav")
			if output, err := exec.CommandContext(t.Context(), mkfifo, path).CombinedOutput(); err != nil {
				t.Fatalf("mkfifo: %s: %v", output, err)
			}
			marker := filepath.Join(t.TempDir(), "written")
			if stalled {
				ctx, cancel := context.WithCancel(t.Context())
				writer := exec.CommandContext(ctx, "sh", "-c", `exec 3>"$1"; printf partial >&3; printf written >"$2"; exec sleep 30`, "fixture", path, marker)
				if err := writer.Start(); err != nil {
					cancel()
					t.Fatal(err)
				}
				t.Cleanup(func() { cancel(); writer.Wait() })
			}
			posts := 0
			fixtureBridge(t, func(req bridge.Request) { posts++ })
			a := &app{json: true}
			if code := a.dispatch([]string{"render", "--audio", path, "--timeout", "350ms"}); code != exitStillRunning {
				t.Fatalf("exit=%d", code)
			}
			if posts != 0 || len(a.op.Jobs) != 0 || a.op.FailureReason != "wait-timeout" {
				t.Fatalf("posts=%d record=%+v", posts, a.op)
			}
			if stalled {
				if _, err := os.Stat(marker); err != nil {
					t.Fatalf("fixture did not reach a stalled read: %v", err)
				}
			}
			unlock, err := operation.Lock(config.Home())
			if err != nil {
				t.Fatalf("hash timeout retained lock: %v", err)
			}
			unlock()
		})
	}
}

func TestInputWorkerOwnDeadlineDoesNotRequireParentCancellation(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	watchdog, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	deadline := time.Now().Add(350 * time.Millisecond)
	cmd := exec.CommandContext(watchdog, executable, scriptInputReader, "stdin", "-", fmt.Sprint(deadline.UnixNano()))
	cmd.Stdin = reader
	if err := cmd.Run(); err == nil || cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 3 || watchdog.Err() != nil {
		t.Fatalf("worker did not enforce its own budget: state=%v err=%v parent=%v", cmd.ProcessState, err, watchdog.Err())
	}
}
