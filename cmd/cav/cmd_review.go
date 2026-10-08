package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/rock3r/cav/assets"
	"github.com/rock3r/cav/internal/config"
	"github.com/rock3r/cav/internal/review"
)

func init() {
	register(command{
		name:    "review",
		args:    "[video.mp4] [--port 8790] [--author NAME] [--open] | wait [video] [--timeout 30m] | export [video] | resolve <id> [--note TEXT] [video] | reopen <id> [video]",
		summary: "Review a render in the browser: step frames, draw, comment, then send the notes to the agent.",
		run:     cmdReview,
	})
	longHelp["review"] = `
cav review [video.mp4] serves a review page on http://127.0.0.1:8790 (the newest MP4 in
renders/ when no video is given). It keeps running until Ctrl-C, so start it in the
background. In the page you can:
  - play, scrub, step one frame (arrows), shuttle with J/K/L, set a range with I/O;
  - draw arrows, boxes, ellipses and freehand on the frame;
  - leave a note on a frame or a range;
  - press "Send to agent" to hand the new open notes over.
The page plays an all-intra copy of the render (made once, cached in ~/.cav/cache/review),
so every seek lands on an exact frame.

Notes live next to the video in <video>.review.json. Each note with a drawing also gets a
PNG of its frame with the drawing burned in, in review/<video>/, so you can see what the
reviewer saw.

For the agent:
  cav review wait [video] [--timeout 30m]
        blocks until the reviewer presses "Send to agent", then prints those notes (with
        frame, range, text, shapes and the snapshot path) and marks the send received.
        Exit 3 when the timeout passes with nothing sent: run it again.
  cav review export [video] [--dir renders]   every note, as JSON with --json
  cav review resolve <id> [--note "what changed"] [video]
  cav review reopen <id> [video]
  cav review hook
        a command hook for Claude Code and Codex (SessionStart, UserPromptSubmit, Stop):
        reads the hook event on stdin and, when notes were sent and nobody picked them up,
        tells the agent once. The cavalry plugin installs it.
Fix a note, re-render to the same file, then resolve it with a short note. The page
shows resolved notes, and marks notes made on an older render.`
}

func cmdReview(a *app, args []string) error {
	sub := ""
	if len(args) > 0 {
		switch args[0] {
		case "wait", "export", "resolve", "reopen", "serve", "hook":
			sub, args = args[0], args[1:]
		}
	}
	switch sub {
	case "wait":
		return reviewWait(a, args)
	case "serve":
		return reviewServe(a, args)
	case "hook":
		os.Exit(reviewHook(os.Stdin, os.Stdout, os.Stderr))
	case "export":
		return reviewExport(a, args)
	case "resolve", "reopen":
		return reviewSetStatus(a, sub, args)
	}
	return reviewServe(a, args)
}

// reviewVideo resolves the video argument. Without one it takes the video of the newest
// review file in the renders folder, or else the newest MP4 there.
func reviewVideo(pos []string) (string, error) {
	if len(pos) > 1 {
		return "", usageErr("give one video")
	}
	if len(pos) == 1 {
		if _, err := os.Stat(pos[0]); err != nil {
			return "", fail(exitError, "cannot read "+pos[0], "")
		}
		return pos[0], nil
	}
	if docs, _ := filepath.Glob(filepath.Join(outDir(), "*.review.json")); len(docs) > 0 {
		sort.Slice(docs, func(i, j int) bool {
			a, _ := os.Stat(docs[i])
			b, _ := os.Stat(docs[j])
			return a.ModTime().After(b.ModTime())
		})
		base := strings.TrimSuffix(docs[0], ".review.json")
		for _, ext := range []string{".mp4", ".mov", ".m4v", ".webm"} {
			if _, err := os.Stat(base + ext); err == nil {
				return base + ext, nil
			}
		}
	}
	matches, _ := filepath.Glob(filepath.Join(outDir(), "*.mp4"))
	if len(matches) == 0 {
		return "", fail(exitError, "no video given and no MP4 in "+outDir(), "render one with `cav render -o renders/final.mp4`, or pass the path")
	}
	sort.Slice(matches, func(i, j int) bool {
		a, _ := os.Stat(matches[i])
		b, _ := os.Stat(matches[j])
		return a.ModTime().After(b.ModTime())
	})
	return matches[0], nil
}

func defaultAuthor() string {
	for _, k := range []string{"CAV_AUTHOR", "USER", "USERNAME"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return "reviewer"
}

func reviewServe(a *app, args []string) error {
	fs := flag.NewFlagSet("review", flag.ContinueOnError)
	port := fs.Int("port", 8790, "port on 127.0.0.1 (the next free one is used when it is taken)")
	author := fs.String("author", defaultAuthor(), "name shown on new notes")
	open := fs.Bool("open", false, "open the page in the default browser")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	video, err := reviewVideo(pos)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	prepare := func(ctx context.Context) (review.Source, string, error) {
		src, err := review.Probe(ctx, video)
		if err != nil {
			return src, "", err
		}
		if src.Frames <= 0 || src.FPS <= 0 {
			return src, "", fmt.Errorf("%s has no readable frames", video)
		}
		if src.SHA256, err = review.Digest(video); err != nil {
			return src, "", err
		}
		src.Render = video
		fmt.Fprintf(os.Stderr, "preparing %s (%d frames at %g fps)…\n", video, src.Frames, src.FPS)
		proxy, err := review.Proxy(ctx, video, src.SHA256, filepath.Join(config.CacheDir(), "review"))
		return src, proxy, err
	}
	src, proxy, err := prepare(ctx)
	if err != nil {
		return fail(exitError, err.Error(), "check the file plays, and that ffmpeg is installed: "+ffmpegFix())
	}
	store := review.Open(video)
	// Record the render in the review file, so `wait` and `export` know what it was.
	if _, err := store.Update(func(d *review.Doc) error { d.Source = src; return nil }); err != nil {
		return err
	}
	srv := &review.Server{Store: store, Source: src, Proxy: proxy, Page: assets.ReviewPage, PagePath: os.Getenv("CAV_REVIEW_PAGE"), Author: *author, Prepare: prepare}
	go srv.Watch(ctx, time.Second)
	ln, err := listenFrom(*port)
	if err != nil {
		return err
	}
	url := "http://" + ln.Addr().String() + "/"
	store.Update(func(d *review.Doc) error {
		d.Server = &review.ServerInfo{URL: url, PID: os.Getpid(), Started: time.Now().UTC()}
		return nil
	})
	defer store.Update(func(d *review.Doc) error {
		if d.Server != nil && d.Server.PID == os.Getpid() {
			d.Server = nil
		}
		return nil
	})
	hs := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		hs.Shutdown(sctx)
	}()
	a.emit(map[string]any{"url": url, "video": video, "reviewFile": store.DocPath, "frames": src.Frames, "fps": src.FPS}, func() {
		fmt.Printf("review: %s\nnotes:  %s\nThe agent waits for notes with: cav review wait %s\nCtrl-C stops the server.\n", url, store.DocPath, video)
	})
	if a.json {
		os.Stdout.Sync()
	}
	if *open {
		openBrowser(url)
	}
	if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func listenFrom(port int) (net.Listener, error) {
	var last error
	for p := port; p < port+20; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			return ln, nil
		}
		last = err
		if errors.Is(err, os.ErrPermission) || strings.Contains(err.Error(), "operation not permitted") {
			return nil, fail(exitError, "cannot listen on 127.0.0.1: "+err.Error(), "a sandbox blocks local ports: run `cav review` outside it")
		}
	}
	return nil, fmt.Errorf("no free port from %d to %d: %w", port, port+19, last)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "could not open a browser (%v); open %s yourself\n", err, url)
	}
}

// noteOut is a comment as the agent sees it: with the absolute snapshot path and whether it
// was made on the current render.
type noteOut struct {
	review.Comment
	SnapshotPath  string `json:"snapshotPath,omitempty"`
	OnOlderRender bool   `json:"onOlderRender,omitempty"`
}

func notesFor(d *review.Doc, ids []string) []noteOut {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var out []noteOut
	cur := d.Source.SHA256
	if len(cur) > 12 {
		cur = cur[:12]
	}
	for _, c := range d.Comments {
		if ids != nil && !want[c.ID] {
			continue
		}
		n := noteOut{Comment: c, OnOlderRender: cur != "" && c.Version != cur}
		if c.Snapshot != "" {
			if abs, err := filepath.Abs(c.Snapshot); err == nil {
				n.SnapshotPath = abs
			}
		}
		out = append(out, n)
	}
	return out
}

func printNotes(notes []noteOut) {
	for _, n := range notes {
		where := fmt.Sprintf("frame %d (%s)", n.Frame, n.Timecode)
		if n.FrameEnd != nil {
			where = fmt.Sprintf("frames %d–%d (from %s)", n.Frame, *n.FrameEnd, n.Timecode)
		}
		older := ""
		if n.OnOlderRender {
			older = " [made on an older render]"
		}
		fmt.Printf("%s  %s  %s  by %s%s\n", n.ID, n.Status, where, n.Author, older)
		if n.Text != "" {
			for _, l := range strings.Split(n.Text, "\n") {
				fmt.Printf("    %s\n", l)
			}
		}
		if len(n.Shapes) > 0 {
			var kinds []string
			for _, s := range n.Shapes {
				kinds = append(kinds, s.Type)
			}
			fmt.Printf("    drawing: %s (%s)\n", strings.Join(kinds, ", "), n.Fragment)
		}
		if n.SnapshotPath != "" {
			fmt.Printf("    snapshot: %s\n", n.SnapshotPath)
		}
		for _, r := range n.Replies {
			fmt.Printf("    reply from %s: %s\n", r.Author, r.Text)
		}
		if n.ResolvedNote != "" {
			fmt.Printf("    resolved: %s\n", n.ResolvedNote)
		}
	}
}

func reviewWait(a *app, args []string) error {
	fs := flag.NewFlagSet("review wait", flag.ContinueOnError)
	timeout := fs.Duration("timeout", 30*time.Minute, "how long to wait for a send")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	video, err := reviewVideo(pos)
	if err != nil {
		return err
	}
	store := review.Open(video)
	deadline := time.Now().Add(*timeout)
	for {
		var got *review.Send
		var notes []noteOut
		_, err := store.Update(func(d *review.Doc) error {
			s := review.Pending(d)
			if s == nil {
				return errNoSend
			}
			now := time.Now().UTC()
			s.DeliveredAt = &now
			got = s
			notes = notesFor(d, s.Comments)
			return nil
		})
		if err == nil {
			a.emit(map[string]any{"send": got.N, "video": video, "notes": notes}, func() {
				fmt.Printf("send #%d from the review of %s: %d note(s)\n\n", got.N, video, len(notes))
				printNotes(notes)
				fmt.Printf("\nFix them, re-render to %s, then: cav review resolve <id> --note \"what changed\"\n", video)
			})
			return nil
		}
		if !errors.Is(err, errNoSend) {
			return err
		}
		if time.Now().After(deadline) {
			return &cliError{code: exitStillRunning, msg: "no notes sent within " + timeout.String(),
				hint: "the reviewer has not pressed Send to agent yet; run `cav review wait` again"}
		}
		time.Sleep(time.Second)
	}
}

var errNoSend = errors.New("nothing sent")

func reviewExport(a *app, args []string) error {
	fs := flag.NewFlagSet("review export", flag.ContinueOnError)
	dir := fs.String("dir", "", "folder to look in when no video is given (default: renders)")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if *dir != "" {
		os.Setenv("CAV_OUT_DIR", *dir)
	}
	video, err := reviewVideo(pos)
	if err != nil {
		return err
	}
	store := review.Open(video)
	d, err := store.Load()
	if err != nil {
		return err
	}
	notes := notesFor(d, nil)
	a.emit(map[string]any{"video": video, "reviewFile": store.DocPath, "source": d.Source, "notes": notes, "sends": d.Sends, "server": d.Server}, func() {
		if len(notes) == 0 {
			fmt.Printf("no notes on %s yet (%s)\n", video, store.DocPath)
			return
		}
		printNotes(notes)
	})
	return nil
}

func reviewSetStatus(a *app, sub string, args []string) error {
	fs := flag.NewFlagSet("review "+sub, flag.ContinueOnError)
	note := fs.String("note", "", "what changed (shown to the reviewer)")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 1 {
		return usageErr("usage: cav review %s <id> [video]", sub)
	}
	id := pos[0]
	video, err := reviewVideo(pos[1:])
	if err != nil {
		return err
	}
	status := "resolved"
	if sub == "reopen" {
		status = "open"
	}
	var out review.Comment
	_, err = review.Open(video).Update(func(d *review.Doc) error {
		c := review.Find(d, id)
		if c == nil {
			return fmt.Errorf("no note %s on %s", id, video)
		}
		if err := review.SetStatus(c, status, *note); err != nil {
			return err
		}
		out = *c
		return nil
	})
	if err != nil {
		return fail(exitError, err.Error(), "list the notes with `cav review export`")
	}
	a.emit(map[string]any{"id": out.ID, "status": out.Status}, func() { fmt.Printf("%s: %s\n", out.ID, out.Status) })
	return nil
}
