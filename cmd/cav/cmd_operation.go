package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rock3r/cav/internal/bridge"
	"github.com/rock3r/cav/internal/config"
	"github.com/rock3r/cav/internal/operation"
)

var trackedCommands = map[string]bool{"render": true, "check": true, "run": true, "frame": true, "sheet": true, "onion": true, "frames": true, "seams": true}

func trackedCommand(args []string) bool {
	return len(args) > 0 && (trackedCommands[args[0]] || (args[0] == "scene" && len(args) > 1 && args[1] == "open"))
}

func init() {
	register(command{name: "operation", args: "status <id> | resume <id> [--timeout 30m] | abandon <id> --acknowledge-unknown-outcome", summary: "Inspect or resume a render, check, run, frame, sheet or scene-open command.", run: cmdOperation})
	longHelp["operation"] = `status reads checkpoints and existing bridge state; it never submits scene work.
resume waits for recorded jobs, then continues remaining phases. It never re-submits
an uncertain job, including on older bridges. Unknown or expired results need human
inspection; do not retry the original command. A fresh timeout bounds each resume's
entire wait, including preparation. Native calls may continue after cav exits.
Records are retained in ~/.cav/operations. Resume uses the original working directory
and transport. The original restricted mode is retained across resumes; a stricter
current mode is also enforced. Legacy records without a restriction setting can wait
for existing jobs but cannot submit remaining work. Keep that directory, inputs, scene
and bridge session intact until done.
A kernel lock serializes operation clients; crashes release the lock automatically.
abandon <id> --acknowledge-unknown-outcome marks only local recovery abandoned after
manual reconciliation. It does not cancel native work.
A refused bridge connection or changed session ends the wait with exit 4 and an unknown
native outcome. Status exposes unvalidated staged render artifacts; file growth or age
is not proof of completion or a crash. Preserve them and reconcile before retrying.
For chunk renders only, resume ID --restart-chunk --acknowledge-unknown-outcome
allows explicit recovery after reconciling old native work and reopening the same saved
scene in a new idle bridge session. Inputs are rehashed. Completed segments are reused;
uncertain jobs and their artifacts are retained before a fresh job ID is submitted.
Raw cav job wait still waits for just one job; it does not continue command phases.`
}
func (a *app) checkpoint(phase string) error {
	if a.op == nil {
		return nil
	}
	a.op.Phase = phase
	return operation.Save(config.Home(), a.op)
}

// Use the command's actual parsed duration, not a second scan of raw arguments.
// Resume supplies its own fresh budget without changing recorded command inputs.
func (a *app) beginOperationBudget(parsed time.Duration) error {
	if a.op == nil {
		return nil
	}
	budget := parsed
	if a.resumeBudget > 0 {
		budget = a.resumeBudget
	}
	if budget <= 0 {
		return usageErr("timeout must be positive")
	}
	a.ctx, a.operationCancel = context.WithTimeout(context.Background(), budget)
	return nil
}
func (a *app) startOperation(args []string, run func(*app, []string) error) error {
	unlock, err := operation.Lock(config.Home())
	if err != nil {
		return err
	}
	defer unlock()
	c := bridge.New()
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defaultDir, err := filepath.Abs(outDir())
	if err != nil {
		return err
	}
	restricted := os.Getenv("CAV_RESTRICTED") == "1"
	a.op = &operation.Record{Restricted: &restricted, OutputDir: defaultDir, Schema: 1, ID: bridge.NewID(), Command: append([]string(nil), args...), Cwd: cwd, Host: c.Host, Port: c.Port, Spool: c.Spool, Phase: "preparation", Status: "running", Jobs: []*operation.Job{}}
	if err = operation.Save(config.Home(), a.op); err != nil {
		return err
	}
	if !a.json {
		fmt.Fprintf(os.Stderr, "cav: operation %s; recover with cav operation resume %s\n", a.op.ID, a.op.ID)
	}
	return a.executeOperation(run, 0)
}
func (a *app) executeOperation(run func(*app, []string) error, resumeBudget time.Duration) error {
	a.ctx = context.Background()
	a.resumeBudget = resumeBudget
	a.operationCancel = nil
	defer func() {
		if a.operationCancel != nil {
			a.operationCancel()
		}
	}()
	a.jobCursor = 0
	if a.op.Completed == nil {
		a.op.Completed = map[string]bool{}
	}
	if a.logJob == nil {
		a.logJob = map[string]any{}
	}
	a.logJob["operation"] = a.op.ID
	a.op.Error = ""
	a.op.FailureReason = ""
	a.op.Status = "running"
	err := run(a, a.op.Command[1:])
	if err == nil {
		// Async run deliberately ends submission but remains resumable until its job finishes.
		pending := false
		for _, j := range a.op.Jobs {
			if j.Result == nil {
				pending = true
			}
		}
		if pending {
			a.op.Status = "queued"
		} else {
			a.op.Phase = "completion"
			a.op.Status = "complete"
		}
	} else {
		a.op.Error = err.Error()
		a.op.FailureReason = operationFailureReason(err)
		a.op.Status = "failed"
		var ce *cliError
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, bridge.ErrStillRunning) || errors.Is(err, bridge.ErrLost) || errors.Is(err, bridge.ErrUnknownJob) || errors.Is(err, bridge.ErrUnavailable) || (errors.As(err, &ce) && ce.code == exitStillRunning) {
			a.op.Status = "unknown"
			if len(a.op.Jobs) > 0 && a.op.Jobs[len(a.op.Jobs)-1].Result == nil {
				a.op.Status = string(a.op.Jobs[len(a.op.Jobs)-1].State)
			}
		}
	}
	a.logJob["operationStatus"] = a.op.Status
	a.logJob["operationPhase"] = a.op.Phase
	if e := operation.Save(config.Home(), a.op); e != nil {
		return fmt.Errorf("checkpoint failed: %w (command result: %v)", e, err)
	}
	if err != nil {
		code := exitError
		if a.op.Status != "failed" {
			code = exitStillRunning
		}
		hint := fmt.Sprintf("Inspect with cav operation status %s; continue with cav operation resume %s. Do not retry the original command.", a.op.ID, a.op.ID)
		if a.op.Command[0] == "run" && len(a.op.Jobs) == 0 && a.op.Phase == "preparation" {
			hint = fmt.Sprintf("Input preparation did not finish; no native job was submitted. Preserve the complete script and supply it to a new run. Inspect the local record with cav operation status %s; resume cannot reconstruct an incomplete input stream.", a.op.ID)
		}
		if errors.Is(err, bridge.ErrDisconnected) || errors.Is(err, bridge.ErrSessionChanged) {
			code = exitLost
			hint = fmt.Sprintf("Inspect with cav operation status %s. Preserve staged artifacts and reconcile the native outcome before resuming or abandoning. A partial file is not completion; do not retry the original command.", a.op.ID)
		}
		return &cliError{code: code, msg: err.Error(), hint: hint, data: map[string]any{"operation": a.op.ID, "phase": a.op.Phase, "status": a.op.Status, "failureReason": a.op.FailureReason, "jobs": operation.View(a.op).Jobs, "partial": json.RawMessage(a.op.Data), "renderProgress": renderProgress(a.op)}}
	}
	if a.outputData != nil {
		a.emitting = true
		a.emit(a.outputData, a.outputHuman)
		a.emitting = false
	}
	return nil
}
func cmdOperation(a *app, args []string) error {
	fs := flag.NewFlagSet("operation", flag.ContinueOnError)
	timeout := fs.Duration("timeout", 30*time.Minute, "total resume wait budget")
	restart := fs.Bool("restart-chunk", false, "resume chunk rendering after a reconciled native restart")
	acknowledge := fs.Bool("acknowledge-unknown-outcome", false, "abandon local recovery only; does not cancel native work")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 2 || (pos[0] != "status" && pos[0] != "resume" && pos[0] != "abandon") {
		return usageErr("operation needs status|resume <id>")
	}
	if *restart && pos[0] != "resume" {
		return usageErr("--restart-chunk is only valid for operation resume")
	}
	// Status is lock-free and read-only. Atomic checkpoints make concurrent reads safe.
	if pos[0] == "status" {
		r, e := operation.Load(config.Home(), pos[1])
		if e != nil {
			return e
		}
		c := bridge.New()
		c.Host = r.Host
		c.Port = r.Port
		c.Spool = r.Spool
		c.ExpectedSession = r.Session
		if r.Status != "complete" && r.Status != "abandoned" && len(r.Jobs) > 0 {
			j := r.Jobs[len(r.Jobs)-1]
			if j.Result == nil {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				state, result, inspectionErr := c.Inspect(ctx, j.ID)
				if inspectionErr != nil {
					r.FailureReason = operationFailureReason(inspectionErr)
					r.Error = inspectionErr.Error()
				}
				r.Status = string(state)
				if state == bridge.StateDone {
					r.Status = "ready-to-resume"
					if result != nil && !result.OK {
						r.Status = "failed"
					}
				}
				j.State = state
			}
		}
		data := map[string]any{"operation": operation.View(r)}
		if progress := renderProgress(r); progress != nil {
			data["renderProgress"] = progress
		}
		if r.Progress != "" {
			if b, e := os.ReadFile(r.Progress); e == nil {
				var v any
				if json.Unmarshal(b, &v) == nil {
					data["progress"] = v
				}
			}
		}
		a.emit(data, func() {
			fmt.Printf("%s: %s (%s), updated %s\n", r.ID, r.Status, r.Phase, r.Updated.Format(time.RFC3339))
			if r.Error != "" {
				fmt.Printf("  %s: %s\n", r.FailureReason, r.Error)
			}
			if progress, ok := data["renderProgress"]; ok {
				fmt.Printf("render artifacts (file activity is not completion):\n%s\n", prettyAny(progress))
			}
			for _, j := range r.Jobs {
				fmt.Printf("  %s %s %s\n", j.ID, j.Phase, j.State)
			}
			if progress, ok := data["progress"]; ok {
				fmt.Printf("profile progress:\n%s\n", prettyAny(progress))
			}
		})
		return nil
	}
	unlock, err := operation.Lock(config.Home())
	if err != nil {
		return err
	}
	defer unlock()
	r, err := operation.Load(config.Home(), pos[1])
	if err != nil {
		return err
	}
	if pos[0] == "abandon" {
		if !*acknowledge {
			return usageErr("abandon requires --acknowledge-unknown-outcome; it does not cancel a native job")
		}
		if r.Status == "complete" {
			return usageErr("completed operation needs no abandonment")
		}
		r.Status = "abandoned"
		r.Error = "local recovery abandoned; native work was not cancelled"
		if e := operation.Save(config.Home(), r); e != nil {
			return e
		}
		a.emit(map[string]any{"operation": r.ID, "status": "abandoned", "cancelled": false}, func() { fmt.Printf("operation %s abandoned locally; native work was not cancelled\n", r.ID) })
		return nil
	}
	if r.Status == "abandoned" {
		return usageErr("operation was abandoned; inspect its native outputs manually")
	}
	if r.Status == "complete" {
		a.emit(map[string]any{"operation": operation.View(r)}, func() { fmt.Printf("operation %s already complete\n%s\n", r.ID, prettyJSON(r.Data)) })
		return nil
	}
	if !trackedCommand(r.Command) {
		return fmt.Errorf("unsupported operation command")
	}
	if *timeout <= 0 {
		return usageErr("timeout must be positive")
	}
	c := bridge.New()
	if c.Host != r.Host || c.Port != r.Port || c.Spool != r.Spool {
		return fmt.Errorf("transport differs from the recorded operation; restore its CAV_BRIDGE_HOST/PORT and spool settings")
	}
	if err = os.Chdir(r.Cwd); err != nil {
		return err
	}
	if *restart {
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()
		if err = restartChunk(ctx, r, *acknowledge); err != nil {
			return err
		}
		deadline, _ := ctx.Deadline()
		*timeout = time.Until(deadline)
		if *timeout <= 0 {
			return context.DeadlineExceeded
		}
	}
	a.op = r

	for _, c := range commands {
		if c.name == r.Command[0] {
			return a.executeOperation(c.run, *timeout)
		}
	}
	return fmt.Errorf("unknown recorded command")
}
func (a *app) operationJob(code string, o execOpts) (*jobOutcome, error) {
	idx := a.jobCursor
	a.jobCursor++
	var j *operation.Job
	if idx < len(a.op.Jobs) {
		j = a.op.Jobs[idx]
		if j.Code != code || j.Helpers != o.helpers || (j.Helpers && j.HelperVersion != helpersVersion()) {
			return nil, fmt.Errorf("operation inputs or CLI changed at phase %s; refusing to submit", j.Phase)
		}
	} else {
		j = &operation.Job{ID: bridge.NewID(), Code: code, Helpers: o.helpers, Phase: a.op.Phase, Submission: "prepared", State: bridge.StateUnknown}
		if o.helpers {
			j.HelperVersion = helpersVersion()
		}
		a.op.Jobs = append(a.op.Jobs, j)
		if err := operation.Save(config.Home(), a.op); err != nil {
			return nil, err
		}
	}
	if a.logJob == nil {
		a.logJob = map[string]any{}
	}
	a.logJob["job"] = j.ID
	out := &jobOutcome{id: j.ID, code: code, source: o.source}
	if j.Result != nil {
		out.result = j.Result
		a.recordJobResult(j.Result)
		return out, nil
	}
	c := bridge.New()
	c.ExpectedSession = a.op.Session
	dir := jobDir(c)
	if j.Submission == "prepared" {
		if a.op.Restricted == nil {
			return nil, fmt.Errorf("operation restriction mode was not recorded; refusing to submit remaining work")
		}
		if err := a.ctx.Err(); err != nil {
			return nil, err
		}
		// The CLI lock does not cover accepted native work after another CLI exits.
		// Resume may consume old results, but it must pass the gate before new POSTs.
		if err := pendingOperation(a.ctx, c, a.op.ID); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
		p := filepath.Join(dir, j.ID+".js")
		if err := os.WriteFile(p, []byte(code), 0600); err != nil {
			return nil, err
		}
		req := &bridge.Request{ID: j.ID, File: p, Restricted: *a.op.Restricted || os.Getenv("CAV_RESTRICTED") == "1"}
		if o.helpers {
			p, e := ensurePreload(dir)
			if e != nil {
				return nil, e
			}
			req.Preload = p
			req.PreloadVersion = helpersVersion()
		}
		if c.Spool == "" {
			payload, e := c.Probe(a.ctx)
			if e != nil {
				return nil, e
			}
			if payload["type"] == "running" && payload["id"] != j.ID {
				return nil, &cliError{code: exitStillRunning, msg: fmt.Sprintf("bridge is running another job %v; this job remains prepared", payload["id"])}
			}
			session, _ := payload["bridgeSession"].(string)
			if a.op.Session != "" && session != a.op.Session {
				return nil, fmt.Errorf("bridge session changed; refusing to submit remaining phases")
			}
			if a.op.Session == "" && len(a.op.Jobs) == 1 {
				a.op.Session = session
			}
		}
		c.ExpectedSession = a.op.Session
		// Record intent BEFORE POST. Lost acknowledgements never cause a second submission.
		j.Submission = "uncertain"
		if err := operation.Save(config.Home(), a.op); err != nil {
			return nil, err
		}
		if err := c.SubmitDeadline(a.ctx, req); err != nil {
			return nil, err
		}
		j.Submission = "accepted"
		j.State = bridge.StateQueued
		if err := operation.Save(config.Home(), a.op); err != nil {
			return nil, err
		}
		_ = os.WriteFile(filepath.Join(config.Home(), "last-job"), []byte(j.ID), 0600)
	}
	if o.async {
		return out, nil
	}
	onState := func(s bridge.State) {
		j.State = s
		_ = operation.Save(config.Home(), a.op)
		if !a.json {
			fmt.Fprintf(os.Stderr, "cav: %s job %s %s\n", a.op.Phase, j.ID, s)
		}
	}
	budget := o.timeout
	if dl, ok := a.ctx.Deadline(); ok {
		budget = time.Until(dl)
	}
	res, err := c.WaitAccepted(a.ctx, j.ID, budget, j.State, onState)
	if err != nil {
		if errors.Is(err, bridge.ErrUnknownJob) || errors.Is(err, bridge.ErrLost) {
			j.State = bridge.StateUnknown
		}
		return nil, err
	}
	j.Result = res
	j.State = bridge.StateDone
	if err = operation.Save(config.Home(), a.op); err != nil {
		return nil, err
	}
	out.result = res
	a.recordJobResult(res)
	return out, nil
}

// An exited CLI does not mean its native job stopped. Gate new operations against
// pending jobs on the same transport, using existing state only.
func pendingOperation(ctx context.Context, c *bridge.Client, excludeID string) error {
	files, err := filepath.Glob(filepath.Join(config.Home(), "operations", "*.json"))
	if err != nil {
		return err
	}
	for _, p := range files {
		loaded, e := operation.Load(config.Home(), strings.TrimSuffix(filepath.Base(p), ".json"))
		if e != nil {
			return fmt.Errorf("invalid operation record %s: %w", p, e)
		}
		r := *loaded
		if r.ID == excludeID || r.Status == "complete" || r.Status == "abandoned" || r.Host != c.Host || r.Port != c.Port || r.Spool != c.Spool {
			continue
		}
		for _, j := range r.Jobs {
			if j.Result == nil && j.Submission != "prepared" {
				inspectionCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				inspector := *c
				inspector.ExpectedSession = r.Session
				state, _, _ := inspector.Inspect(inspectionCtx, j.ID)
				cancel()
				if err := ctx.Err(); err != nil {
					return err
				}
				if state != bridge.StateDone {
					return &cliError{code: exitStillRunning, msg: fmt.Sprintf("operation %s still has a %s job; no new work submitted", r.ID, state), hint: "inspect or resume that operation; stale/unknown outcomes require manual reconciliation before operation abandon --acknowledge-unknown-outcome", data: map[string]any{"operation": r.ID, "job": j.ID, "status": state, "phase": r.Phase}}
				}
			}
		}
	}
	return nil
}

func (a *app) recordJobResult(r *bridge.Result) {
	a.logJob["jobOk"] = r.OK
	a.logJob["jobMs"] = r.MS
	if r.Error != nil {
		a.logJob["jobError"] = r.Error.Message
	}
}

func operationFailureReason(err error) string {
	switch {
	case errors.Is(err, bridge.ErrDisconnected):
		return "bridge-disconnected"
	case errors.Is(err, bridge.ErrSessionChanged):
		return "bridge-session-changed"
	case errors.Is(err, bridge.ErrLost):
		return "bridge-unresponsive"
	case errors.Is(err, bridge.ErrStillRunning), errors.Is(err, context.DeadlineExceeded):
		return "wait-timeout"
	default:
		return "inspection-or-command-failed"
	}
}

// File activity is diagnostic evidence only. Neither a size nor a stale timestamp
// validates a render, proves a crash, or allows resubmission of an uncertain job.
func renderProgress(r *operation.Record) map[string]any {
	if r.Render == nil {
		return nil
	}
	files := []map[string]any{}
	for _, name := range []string{"video.mp4", "mux.partial.mp4", "mux.mp4"} {
		p := filepath.Join(r.Render.Stage, name)
		st, err := os.Stat(p)
		if os.IsNotExist(err) {
			continue
		}
		f := map[string]any{"file": p, "validation": "unvalidated"}
		if err != nil {
			f["error"] = err.Error()
		} else {
			f["bytes"] = st.Size()
			f["modified"] = st.ModTime().UTC()
			f["secondsSinceModification"] = max(0, time.Since(st.ModTime()).Seconds())
		}
		files = append(files, f)
	}
	return map[string]any{"staging": r.Render.Stage, "expectedFrames": r.Render.ExpectedFrames, "fps": r.Render.FPS, "artifacts": files, "validatedChunks": r.Render.Chunks, "chunkSize": r.Render.ChunkSize}
}
