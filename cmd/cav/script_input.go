package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

const scriptInputReader = "__cav-script-input"

// Run blocking input in an owned child rather than a goroutine that could remain
// stuck in Read/Open after cancellation. This also covers inherited stdin and
// named pipes on platforms where os.File deadlines are unavailable.
// The private child mode exits before command dispatch, relay or bridge access.
func init() {
	if len(os.Args) != 3 || os.Args[1] != scriptInputReader {
		return
	}
	var data []byte
	var err error
	if os.Args[2] == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(os.Args[2])
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, executable, scriptInputReader, path)
	if path == "-" {
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
		return nil, fmt.Errorf("cannot read script input %s: %s: %w", path, strings.TrimSpace(stderr.String()), err)
	}
	return data, nil
}
