package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rock3r/cav/internal/bridge"
	"github.com/rock3r/cav/internal/config"
	"github.com/rock3r/cav/internal/operation"
)

type chunkOptions struct{ Start, End, Scale, MaxWarmup int }

func (a *app) renderChunks(st *sceneState, options chunkOptions) error {
	start, end, scale, maxWarmup := options.Start, options.End, options.Scale, options.MaxWarmup
	a.requireSaved = true
	defer func() { a.requireSaved = false }()
	r := a.op.Render
	if st.ScenePath == "" || st.Unsaved {
		return usageErr("chunk rendering requires a saved named scene; use --save")
	}
	if start < st.Comp.StartFrame || end > st.Comp.EndFrame || end-st.Comp.StartFrame+1 > maxWarmup {
		return usageErr("chunk range exceeds comp or --max-warmup replay budget")
	}
	if err := a.bindSceneInput(st.ScenePath); err != nil {
		return err
	}
	if (end-start)/r.ChunkSize+1 > 1000 {
		return usageErr("chunk count exceeds 1000; increase --chunk-frames")
	}
	for first, i := start, 0; first <= end; i++ {
		if err := a.bindSceneInput(st.ScenePath); err != nil {
			return err
		}
		last := min(end, first+r.ChunkSize-1)
		frames := make([]int, 0, last-first+1)
		for f := first; f <= last; f++ {
			frames = append(frames, f)
		}
		dir := filepath.Join(r.Stage, fmt.Sprintf("chunk-%04d", i))
		segment := filepath.Join(dir, "segment.mp4")
		if err := a.checkpoint(fmt.Sprintf("chunk-%04d-render", i)); err != nil {
			return err
		}
		// Replay from comp start for every chunk. This is deliberately more work than
		// jumping into a fresh simulation after a process restart.
		if _, err := a.renderChronologicalFrames(frames, scale, dir, st); err != nil {
			return err
		}
		if i < len(r.Chunks) {
			chunk := r.Chunks[i]
			if chunk.Start != first || chunk.End != last || chunk.Video != segment {
				return fmt.Errorf("chunk manifest differs")
			}
			digest, err := fileDigest(a.ctx, segment)
			if err != nil {
				return err
			}
			if digest != chunk.Digest {
				return fmt.Errorf("validated chunk changed: %s", segment)
			}
		} else {
			if err := a.checkpoint(fmt.Sprintf("chunk-%04d-encode", i)); err != nil {
				return err
			}
			partial := filepath.Join(dir, "segment.partial.mp4")
			args := []string{
				"-y", "-v", "error", "-framerate", fmt.Sprintf("%.12g", st.Comp.FPS),
				"-start_number", fmt.Sprint(first), "-i", filepath.Join(dir, "f_%d.png"),
				"-frames:v", fmt.Sprint(len(frames)), "-c:v", "libx264", "-crf", "18",
				"-vf", "pad=ceil(iw/2)*2:ceil(ih/2)*2", "-pix_fmt", "yuv420p", partial,
			}
			if err := runFFmpeg(a.ctx, args); err != nil {
				return err
			}
			n, err := probeFramesContext(a.ctx, partial)
			if err != nil {
				return err
			}
			if n != len(frames) {
				return fmt.Errorf("chunk has %d frames, expected %d", n, len(frames))
			}
			if err = os.Rename(partial, segment); err != nil {
				return err
			}
			digest, err := fileDigest(a.ctx, segment)
			if err != nil {
				return err
			}
			r.Chunks = append(r.Chunks, operation.Chunk{Start: first, End: last, Video: segment, Digest: digest})
			if err = a.checkpoint(fmt.Sprintf("chunk-%04d-validated", i)); err != nil {
				return err
			}
		}
		first = last + 1
	}
	if err := a.bindSceneInput(st.ScenePath); err != nil {
		return err
	}
	// Concatenation happens only after every segment has passed count/hash checks.
	list := filepath.Join(r.Stage, "chunks.txt")
	var text strings.Builder
	for i := range r.Chunks {
		fmt.Fprintf(&text, "file 'chunk-%04d/segment.mp4'\n", i)
	}
	if err := os.WriteFile(list, []byte(text.String()), 0o600); err != nil {
		return err
	}
	if err := a.checkpoint("concatenating-chunks"); err != nil {
		return err
	}
	return runFFmpeg(a.ctx, []string{
		"-y", "-v", "error", "-f", "concat", "-safe", "0", "-i", list,
		"-c", "copy", filepath.Join(r.Stage, "video.mp4"),
	})
}
func runFFmpeg(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffmpeg failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
func (a *app) bindSceneInput(path string) error {
	digest, err := fileDigest(a.ctx, path)
	if err != nil {
		return err
	}
	if a.op.Inputs == nil {
		a.op.Inputs = map[string]string{}
	}
	if old, ok := a.op.Inputs[path]; ok && old != digest {
		return fmt.Errorf("render scene file changed")
	}
	a.op.Inputs[path] = digest
	return a.checkpoint(a.op.Phase)
}

// Restart is explicit reconciliation, never a default response to a timeout.
// It requires a different idle bridge session and retains the uncertain attempt.
func restartChunk(r *operation.Record, acknowledge bool, timeout time.Duration) error {
	if !acknowledge {
		return usageErr("--restart-chunk requires --acknowledge-unknown-outcome after reconciling the old native job")
	}
	if r.Render == nil || r.Render.ChunkSize == 0 || r.Spool != "" {
		return usageErr("restart-chunk requires an HTTP chunk render")
	}
	ctx, cancel := context.WithTimeout(context.Background(), min(timeout, 10*time.Second))
	defer cancel()
	c := bridge.New()
	payload, err := c.Probe(ctx)
	if err != nil {
		return err
	}
	session, _ := payload["bridgeSession"].(string)
	if session == "" || session == r.Session || payload["type"] == "running" || payload["type"] == "queued" {
		return fmt.Errorf("restart-chunk requires a different idle bridge session")
	}
	if err = pendingOperation(ctx, c, r.ID); err != nil {
		return err
	}
	for path, expected := range r.Inputs {
		digest, e := fileDigest(ctx, path)
		if e != nil {
			return e
		}
		if digest != expected {
			return fmt.Errorf("restart input changed: %s", path)
		}
	}
	for _, j := range r.Jobs {
		if (j.Result == nil || !j.Result.OK) && !strings.HasPrefix(j.Phase, "chunk-") {
			return fmt.Errorf("interrupted phase is not a native chunk; use ordinary resume")
		}
	}
	for _, j := range r.Jobs {
		if j.Result != nil && j.Result.OK {
			continue
		}
		if !strings.HasPrefix(j.Phase, "chunk-") {
			return fmt.Errorf("interrupted phase is not a native chunk; use ordinary resume")
		}
		old := *j
		r.ReconciledJobs = append(r.ReconciledJobs, &old)
		dir := filepath.Join(r.Render.Stage, strings.TrimSuffix(j.Phase, "-render"))
		if _, err = os.Stat(dir); err == nil {
			if err = os.Rename(dir, dir+"-attempt-"+j.ID); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		j.ID = bridge.NewID()
		j.Result = nil
		j.Submission = "prepared"
		j.State = bridge.StateUnknown
	}
	// Completed results remain at their sequence positions; only uncertain chunk
	// work gets a fresh ID. Native guards and input hashes bind the reopened scene.
	r.Session = session
	r.Status = "running"
	return operation.Save(config.Home(), r)
}
