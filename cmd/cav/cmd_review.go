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
		args:    "[video.mp4|brief.json|moodboard.json|storyboard.json] [--port 8790] [--author NAME] [--open] | wait [target] [--timeout 30m] | export [target] | resolve <id> [--note TEXT] [target] | reopen <id> [target]",
		summary: "Review a render, brief, mood board or storyboard in the browser: draw, comment, approve, then send the notes to the agent.",
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

cav review also takes stills, so that the person can approve each stage before slow or paid
work starts. The page shows a grid of items. Each item gets a verdict, and notes with
drawings. There is no video, and no ffmpeg is needed.
  target          items are       verdicts
  brief.json      its fields      approve, redo
  moodboard.json  references      keep, more (more like this), drop
  storyboard.json shots           approve, redo
The review file is <name>.review.json next to the target, as for a video. The "Approve"
button is on only when no item is marked redo or drop and no note is open. Approving writes
"approved" into the review file, with the hash of every item the person saw. When an item
changes (its words, or the bytes of its frame), its old verdict shows as outdated, and the
approval is removed, with the reason in "approvalCleared". Approval also counts as a send, so
cav review wait returns when the person approves. There is no command to approve: only the
person does that, in the page. Never edit "approved" by hand.

For the agent:
  cav review wait [target] [--timeout 30m]
        blocks until the reviewer presses "Send to agent" or approves a stage, then prints
        those notes (with frame or item, text, shapes and the snapshot path) and marks the
        send received. Exit 3 when the timeout passes with nothing sent: run it again.
  cav review export [target] [--dir renders]   every note, as JSON with --json. For stills
        it also lists the items with their verdicts, whether the stage is approved
        and what blocks it. Start nothing slow or paid until it says approved.
  cav review resolve <id> [--note "what changed"] [target]
  cav review reopen <id> [target]
  cav review hook
        a command hook for Claude Code and Codex (SessionStart, UserPromptSubmit, Stop):
        reads the hook event on stdin and, when notes were sent and nobody picked them up,
        tells the agent once. The cavalry plugin installs it.
  cav review hook --host antigravity --event PreInvocation|Stop
        the same hook for Antigravity: it reads Antigravity's camelCase event, looks in
        each workspace path, and answers with injectSteps (PreInvocation) or a "continue"
        decision (Stop). The cavalry plugin installs it in the Antigravity CLI.
Fix a note, re-render to the same file, then resolve it with a short note. The page
shows resolved notes, and marks notes made on an older render. For stills, fix the item in
its file (and its frame), then resolve the note.`
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
		os.Exit(reviewHook(args, os.Stdin, os.Stdout, os.Stderr))
	case "export":
		return reviewExport(a, args)
	case "resolve", "reopen":
		return reviewSetStatus(a, sub, args)
	}
	return reviewServe(a, args)
}

// reviewTarget resolves the target argument: a video, or brief.json, moodboard.json or
// storyboard.json. Without one it takes the video of the newest review file in the renders
// folder, or else the newest MP4 there.
func reviewTarget(pos []string) (string, error) {
	if len(pos) > 1 {
		return "", usageErr("give one target")
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
	video, err := reviewTarget(pos)
	if err != nil {
		return err
	}
	kind, err := review.KindOf(video)
	if err != nil {
		return fail(exitError, err.Error(), "")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if kind != review.KindVideo {
		return reviewServeStills(ctx, a, video, kind, *port, *author, *open)
	}
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
	if _, err := store.Update(func(d *review.Doc) error { d.Source = src; review.AddVersion(d, src); return nil }); err != nil {
		return err
	}
	srv := &review.Server{Store: store, Source: src, Proxy: proxy, Page: assets.ReviewPage, PagePath: os.Getenv("CAV_REVIEW_PAGE"), Author: *author, Prepare: prepare,
		CacheDir: filepath.Join(config.CacheDir(), "review")}
	go srv.Watch(ctx, time.Second)
	url, serve, err := startReview(ctx, store, srv, *port)
	if err != nil {
		return err
	}
	a.emit(map[string]any{"url": url, "video": video, "reviewFile": store.DocPath, "frames": src.Frames, "fps": src.FPS}, func() {
		fmt.Printf("review: %s\nnotes:  %s\nThe agent waits for notes with: cav review wait %s\nCtrl-C stops the server.\n", url, store.DocPath, video)
	})
	if a.json {
		os.Stdout.Sync()
	}
	if *open {
		openBrowser(url)
	}
	return serve()
}

// reviewServeStills serves the review of a brief, mood board or storyboard. It has no video
// to prepare, so it needs no ffmpeg.
func reviewServeStills(ctx context.Context, a *app, target string, kind review.Kind, port int, author string, open bool) error {
	items, err := review.Items(kind, target)
	if err != nil {
		return fail(exitError, err.Error(), "fix "+target+" first")
	}
	store := review.Open(target)
	sha, _ := review.Digest(target)
	if _, err := store.Update(func(d *review.Doc) error {
		d.Kind, d.Source = string(kind), review.Source{Render: target, SHA256: sha}
		review.Reconcile(d, items, time.Now().UTC())
		return nil
	}); err != nil {
		return err
	}
	srv := &review.Server{Store: store, Stills: &review.Stills{Kind: kind, Path: target}, Page: assets.ReviewPage,
		PagePath: os.Getenv("CAV_REVIEW_PAGE"), Author: author}
	url, serve, err := startReview(ctx, store, srv, port)
	if err != nil {
		return err
	}
	a.emit(map[string]any{"url": url, "target": target, "kind": kind, "reviewFile": store.DocPath, "items": len(items)}, func() {
		fmt.Printf("review: %s\nnotes:  %s\nThe agent waits for the verdicts and notes with: cav review wait %s\nCtrl-C stops the server.\n", url, store.DocPath, target)
	})
	if a.json {
		os.Stdout.Sync()
	}
	if open {
		openBrowser(url)
	}
	return serve()
}

// startReview listens on the first free port, records the server in the review file, and
// returns the page's URL and a function that serves until Ctrl-C.
func startReview(ctx context.Context, store *review.Store, srv *review.Server, port int) (string, func() error, error) {
	ln, err := listenFrom(port)
	if err != nil {
		return "", nil, err
	}
	url := "http://" + ln.Addr().String() + "/"
	store.Update(func(d *review.Doc) error {
		d.Server = &review.ServerInfo{URL: url, PID: os.Getpid(), Started: time.Now().UTC()}
		return nil
	})
	hs := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		hs.Shutdown(sctx)
	}()
	return url, func() error {
		defer store.Update(func(d *review.Doc) error {
			if d.Server != nil && d.Server.PID == os.Getpid() {
				d.Server = nil
			}
			return nil
		})
		if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}, nil
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
// was made on the current render, or, for stills, on the current version of its item.
type noteOut struct {
	review.Comment
	SnapshotPath  string `json:"snapshotPath,omitempty"`
	OnOlderRender bool   `json:"onOlderRender,omitempty"`
	ItemTitle     string `json:"itemTitle,omitempty"`
	ItemChanged   bool   `json:"itemChanged,omitempty"`
	ItemGone      bool   `json:"itemGone,omitempty"`
}

// notesFor lists the notes with the ids (all of them when ids is nil). items is the target's
// current items for a review of stills, or nil.
func notesFor(d *review.Doc, ids []string, items []review.Item) []noteOut {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	cur := map[string]review.Item{}
	for _, it := range items {
		cur[it.ID] = it
	}
	var out []noteOut
	src := d.Source.SHA256
	if len(src) > 12 {
		src = src[:12]
	}
	for _, c := range d.Comments {
		if ids != nil && !want[c.ID] {
			continue
		}
		n := noteOut{Comment: c, OnOlderRender: c.Item == "" && src != "" && c.Version != src}
		if it, ok := cur[c.Item]; ok {
			n.ItemTitle, n.ItemChanged = it.Title, c.ItemSHA != it.SHA
		} else if c.Item != "" && items != nil {
			n.ItemGone = true
		}
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
		if n.Item != "" {
			where = n.Item
			if n.Verdict != "" {
				where += " · " + n.Verdict
			}
		}
		older := ""
		switch {
		case n.OnOlderRender:
			older = " [made on an older render]"
		case n.ItemChanged:
			older = " [item changed since]"
		case n.ItemGone:
			older = " [item is gone]"
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

// openReview opens the review of a target. For stills it also loads the items and removes an
// approval that no longer holds, so that every command sees the same state as the page.
func openReview(target string) (*review.Store, review.Kind, []review.Item, error) {
	kind, err := review.KindOf(target)
	if err != nil {
		return nil, "", nil, fail(exitError, err.Error(), "")
	}
	store := review.Open(target)
	return store, kind, currentItems(store, kind, target, true), nil
}

// currentItems loads the items of a stills target and reconciles the approval with them. It
// returns nil for a video, and when the target cannot be read now (it may be mid-edit).
func currentItems(store *review.Store, kind review.Kind, target string, warn bool) []review.Item {
	if kind == review.KindVideo {
		return nil
	}
	items, err := review.Items(kind, target)
	if err == nil {
		_, err = store.ReconcileItems(items)
	}
	if err != nil {
		if warn {
			fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		}
		return nil
	}
	return items
}

func reviewWait(a *app, args []string) error {
	fs := flag.NewFlagSet("review wait", flag.ContinueOnError)
	timeout := fs.Duration("timeout", 30*time.Minute, "how long to wait for a send")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	target, err := reviewTarget(pos)
	if err != nil {
		return err
	}
	store, kind, items, err := openReview(target)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(*timeout)
	for first := true; ; first = false {
		if !first {
			items = currentItems(store, kind, target, false)
		}
		var got *review.Send
		var notes []noteOut
		var approval *review.Approval
		var cleared *review.Cleared
		_, err := store.Update(func(d *review.Doc) error {
			s := review.Pending(d)
			if s == nil {
				return errNoSend
			}
			now := time.Now().UTC()
			s.DeliveredAt = &now
			got = s
			notes = notesFor(d, s.Comments, items)
			approval, cleared = d.Approved, d.ApprovalCleared
			return nil
		})
		if err == nil {
			approved := got.Approved && approval != nil
			data := map[string]any{"send": got.N, "target": target, "kind": kind, "approved": approved, "notes": notes}
			if kind == review.KindVideo {
				data["video"] = target
			}
			if approved {
				data["approvedBy"] = approval.By
			}
			a.emit(data, func() {
				switch {
				case approved:
					fmt.Printf("%s is approved by %s (send #%d).\n", target, approval.By, got.N)
				case got.Approved && cleared != nil:
					fmt.Printf("%s was approved in send #%d, but the approval has since been removed: %s.\n", target, got.N, cleared.Reason)
				case got.Approved:
					fmt.Printf("%s was approved in send #%d, but the approval has since been removed.\n", target, got.N)
				default:
					fmt.Printf("send #%d from the review of %s: %d note(s)\n", got.N, target, len(notes))
				}
				if len(notes) > 0 {
					fmt.Println()
					if got.Approved {
						fmt.Printf("%d note(s) came with it:\n", len(notes))
					}
					printNotes(notes)
				}
				switch {
				case approved:
					fmt.Printf("\nGo on to the next stage.")
					if len(notes) > 0 {
						fmt.Printf(" Use the notes above there, then: cav review resolve <id> --note \"what changed\"")
					}
					fmt.Println()
				case kind == review.KindVideo:
					fmt.Printf("\nFix them, re-render to %s, then: cav review resolve <id> --note \"what changed\"\n", target)
				default:
					fmt.Printf("\nFix them in %s (and its frames), then: cav review resolve <id> --note \"what changed\"\n", target)
				}
			})
			return nil
		}
		if !errors.Is(err, errNoSend) {
			return err
		}
		if time.Now().After(deadline) {
			return &cliError{code: exitStillRunning, msg: "no notes sent within " + timeout.String(),
				hint: "the reviewer has not pressed Send to agent or Approve yet; run `cav review wait` again"}
		}
		time.Sleep(time.Second)
	}
}

var errNoSend = errors.New("nothing sent")

// itemRow is one item in the export, with what the reviewer said about it.
type itemRow struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	SHA        string `json:"sha"`
	Verdict    string `json:"verdict,omitempty"`
	Outdated   bool   `json:"outdated,omitempty"`
	OldVerdict string `json:"oldVerdict,omitempty"`
	OpenNotes  int    `json:"openNotes"`
}

func reviewExport(a *app, args []string) error {
	fs := flag.NewFlagSet("review export", flag.ContinueOnError)
	dir := fs.String("dir", "", "folder to look in when no target is given (default: renders)")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if *dir != "" {
		os.Setenv("CAV_OUT_DIR", *dir)
	}
	target, err := reviewTarget(pos)
	if err != nil {
		return err
	}
	store, kind, items, err := openReview(target)
	if err != nil {
		return err
	}
	d, err := store.Load()
	if err != nil {
		return err
	}
	notes := notesFor(d, nil, items)
	data := map[string]any{"target": target, "kind": kind, "reviewFile": store.DocPath, "source": d.Source, "notes": notes, "sends": d.Sends, "server": d.Server}
	if kind == review.KindVideo {
		data["video"] = target
	}
	var rows []itemRow
	var blockers []string
	if kind != review.KindVideo {
		states := review.ItemStates(d, items)
		for _, it := range items {
			s := states[it.ID]
			rows = append(rows, itemRow{ID: it.ID, Title: it.Title, SHA: it.SHA, Verdict: s.Verdict, Outdated: s.Outdated, OldVerdict: s.OldVerdict, OpenNotes: s.OpenNotes})
		}
		if blockers = review.Blockers(d, items); blockers == nil {
			blockers = []string{}
		}
		data["items"], data["approved"], data["approval"], data["approvalCleared"], data["blockers"] = rows, d.Approved != nil, d.Approved, d.ApprovalCleared, blockers
	}
	a.emit(data, func() {
		if kind == review.KindVideo {
			if len(notes) == 0 {
				fmt.Printf("no notes on %s yet (%s)\n", target, store.DocPath)
				return
			}
			printNotes(notes)
			return
		}
		switch {
		case d.Approved != nil:
			fmt.Printf("approved by %s on %s\n", d.Approved.By, d.Approved.At.Local().Format("2006-01-02 15:04"))
		case len(blockers) > 0:
			fmt.Printf("not approved: %s\n", strings.Join(blockers, "; "))
		default:
			fmt.Println("not approved yet")
		}
		if d.Approved == nil && d.ApprovalCleared != nil {
			fmt.Printf("the approval was removed: %s\n", d.ApprovalCleared.Reason)
		}
		for _, r := range rows {
			verdict := "no verdict"
			switch {
			case r.Outdated:
				verdict = "no verdict (item changed since " + r.OldVerdict + ")"
			case r.Verdict != "":
				verdict = r.Verdict
			}
			fmt.Printf("%s  %s  %s\n", r.ID, verdict, r.Title)
		}
		if len(notes) > 0 {
			fmt.Println()
			printNotes(notes)
		}
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
		return usageErr("usage: cav review %s <id> [target]", sub)
	}
	id := pos[0]
	target, err := reviewTarget(pos[1:])
	if err != nil {
		return err
	}
	store, _, items, err := openReview(target)
	if err != nil {
		return err
	}
	status := "resolved"
	if sub == "reopen" {
		status = "open"
	}
	var out review.Comment
	_, err = store.Update(func(d *review.Doc) error {
		c := review.Find(d, id)
		if c == nil {
			return fmt.Errorf("no note %s on %s", id, target)
		}
		if err := review.SetStatus(c, status, *note); err != nil {
			return err
		}
		out = *c
		// A reopened note blocks the approval again.
		if items != nil {
			review.Reconcile(d, items, time.Now().UTC())
		}
		return nil
	})
	if err != nil {
		return fail(exitError, err.Error(), "list the notes with `cav review export`")
	}
	a.emit(map[string]any{"id": out.ID, "status": out.Status}, func() { fmt.Printf("%s: %s\n", out.ID, out.Status) })
	return nil
}
