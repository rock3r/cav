package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const scriptInputReader = "__cav-script-input"

// Run blocking input in an owned child rather than a goroutine that could remain
// stuck in Read/Open after cancellation. This also covers inherited stdin and
// named pipes on platforms where os.File deadlines are unavailable.
// The private child mode exits before command dispatch, relay or bridge access.
func init() {
	if len(os.Args) != 5 || os.Args[1] != scriptInputReader {
		return
	}
	deadline, err := strconv.ParseInt(os.Args[4], 10, 64)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid private input deadline")
		os.Exit(1)
	}
	// The worker also owns a deadline, so a parent CLI crash cannot leave it
	// blocked forever. Parent cancellation normally kills and waits for it first.
	time.AfterFunc(time.Until(time.Unix(0, deadline)), func() { os.Exit(3) })
	var data []byte
	switch os.Args[2] {
	case "stdin":
		data, err = io.ReadAll(os.Stdin)
	case "file":
		data, err = os.ReadFile(os.Args[3])
	case "hash":
		var f *os.File
		f, err = os.Open(os.Args[3])
		if err == nil {
			h := sha256.New()
			_, err = io.Copy(h, f)
			f.Close()
			data = []byte(hex.EncodeToString(h.Sum(nil)))
		}
	default:
		err = fmt.Errorf("invalid private input mode")
	}
	if err == nil {
		_, err = os.Stdout.Write(data)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func readScriptInput(ctx context.Context, path string) ([]byte, error) {
	return runInputWorker(ctx, "file", path)
}

func readStdinInput(ctx context.Context) ([]byte, error) {
	return runInputWorker(ctx, "stdin", "-")
}

func runInputWorker(ctx context.Context, mode, path string) ([]byte, error) {
	if ctx == nil {
		return nil, fmt.Errorf("input preparation requires an operation context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil, fmt.Errorf("input preparation requires a deadline")
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, executable, scriptInputReader, mode, path, strconv.FormatInt(deadline.UnixNano(), 10))
	if mode == "stdin" {
		cmd.Stdin = os.Stdin
	}
	cmd.WaitDelay = 200 * time.Millisecond
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if err != nil {
		if cause := ctx.Err(); cause != nil {
			return nil, cause
		}
		if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 3 {
			return nil, context.DeadlineExceeded
		}
		return nil, fmt.Errorf("cannot prepare input %s: %s: %w", path, strings.TrimSpace(stderr.String()), err)
	}
	return data, nil
}
