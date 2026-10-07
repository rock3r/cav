package review

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Probe reads the size, frame rate and frame count of a video with ffprobe.
func Probe(ctx context.Context, path string) (Source, error) {
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0", "-count_packets",
		"-show_entries", "stream=width,height,r_frame_rate,nb_read_packets", "-of", "json", path).Output()
	if err != nil {
		return Source{}, fmt.Errorf("ffprobe %s: %w", path, err)
	}
	var r struct {
		Streams []struct {
			Width   int    `json:"width"`
			Height  int    `json:"height"`
			Rate    string `json:"r_frame_rate"`
			Packets string `json:"nb_read_packets"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &r); err != nil || len(r.Streams) == 0 {
		return Source{}, fmt.Errorf("%s has no video stream", path)
	}
	s := r.Streams[0]
	fps := 0.0
	if n, d, ok := strings.Cut(s.Rate, "/"); ok {
		a, _ := strconv.ParseFloat(n, 64)
		b, _ := strconv.ParseFloat(d, 64)
		if b > 0 {
			fps = a / b
		}
	}
	frames, _ := strconv.Atoi(s.Packets)
	return Source{Render: path, FPS: fps, Frames: frames, Width: s.Width, Height: s.Height}, nil
}

// Proxy makes (once per render content) an all-intra copy of the video, so a browser can
// seek to any frame by decoding only that frame. It lives in cacheDir/<sha256>.mp4.
func Proxy(ctx context.Context, video, sha, cacheDir string) (string, error) {
	out := filepath.Join(cacheDir, sha+".mp4")
	if info, err := os.Stat(out); err == nil && info.Size() > 0 {
		return out, nil
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	tmp := filepath.Join(cacheDir, sha+".tmp.mp4")
	cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-y", "-i", video,
		"-map", "0:v:0", "-map", "0:a:0?",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "18", "-g", "1", "-bf", "0", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart", tmp)
	if b, err := cmd.CombinedOutput(); err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("making the review proxy: %v: %s", err, strings.TrimSpace(string(b)))
	}
	return out, os.Rename(tmp, out)
}
