// Package board holds a storyboard (shots timed in beats) and turns it into a board image,
// an animatic video and, from a folder of references, a mood board.
package board

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rock3r/cav/internal/sheet"
)

const Schema = "cav-board/1"

type Style struct {
	Prompt  string   `json:"prompt,omitempty"`  // the look every generated frame shares
	Refs    []string `json:"refs,omitempty"`    // reference images sent with every generated frame
	Palette []string `json:"palette,omitempty"` // background, foreground, accent, ...
}

type Shot struct {
	ID     string     `json:"id"`
	Beats  [2]float64 `json:"beats"` // start and end beat; the shot ends where the next starts
	What   string     `json:"what"`
	Camera string     `json:"camera,omitempty"`
	Prompt string     `json:"prompt,omitempty"` // extra words for the generated frame
	Frame  string     `json:"frame,omitempty"`  // the image; cav board frames fills it
	Source string     `json:"source,omitempty"` // greybox, gemini, openai, file, ...
}

type Board struct {
	Schema  string  `json:"schema"`
	Title   string  `json:"title,omitempty"`
	BPM     float64 `json:"bpm"`
	FPS     float64 `json:"fps"`
	Seconds float64 `json:"seconds"`
	Width   int     `json:"width"`
	Height  int     `json:"height"`
	// Offset is the time of beat 0 in the music, in seconds.
	Offset float64 `json:"offset,omitempty"`
	// Grid is a cav beats --json file; when set, beat n is its nth detected beat.
	Grid  string `json:"grid,omitempty"`
	Style Style  `json:"style"`
	Shots []Shot `json:"shots"`

	gridBeats []float64
	dir       string
}

// Load reads a storyboard; relative paths inside it are relative to its folder.
func Load(path string) (*Board, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sb := &Board{}
	if err := json.Unmarshal(b, sb); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	sb.dir = filepath.Dir(path)
	if sb.Width == 0 || sb.Height == 0 {
		sb.Width, sb.Height = 1920, 1080
	}
	if sb.FPS == 0 {
		sb.FPS = 60
	}
	if sb.Grid != "" {
		var g struct {
			Beats []float64 `json:"beats"`
		}
		gb, err := os.ReadFile(sb.Path(sb.Grid))
		if err != nil {
			return nil, fmt.Errorf("beat grid: %w", err)
		}
		if err := json.Unmarshal(gb, &g); err != nil || len(g.Beats) == 0 {
			return nil, fmt.Errorf("beat grid %s has no beats", sb.Grid)
		}
		sb.gridBeats = g.Beats
	}
	return sb, sb.Validate()
}

func (sb *Board) Save(path string) error {
	sb.Schema = Schema
	b, err := json.MarshalIndent(sb, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// Path resolves a path written in the storyboard.
func (sb *Board) Path(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(sb.dir, p)
}

// Time returns the time of a beat in seconds.
func (sb *Board) Time(beat float64) float64 {
	if len(sb.gridBeats) > 1 {
		i := int(beat)
		frac := beat - float64(i)
		n := len(sb.gridBeats)
		if i >= n-1 {
			step := sb.gridBeats[n-1] - sb.gridBeats[n-2]
			return sb.gridBeats[n-1] + (beat-float64(n-1))*step
		}
		return sb.gridBeats[i] + frac*(sb.gridBeats[i+1]-sb.gridBeats[i])
	}
	if sb.BPM <= 0 {
		return beat
	}
	return sb.Offset + beat*60/sb.BPM
}

func (sb *Board) Validate() error {
	if len(sb.Shots) == 0 {
		return errors.New("the storyboard has no shots")
	}
	if sb.BPM <= 0 && sb.Grid == "" {
		return errors.New("set bpm (or grid) so beats can become times")
	}
	seen := map[string]bool{}
	for i, s := range sb.Shots {
		if s.ID == "" {
			return fmt.Errorf("shot %d has no id", i+1)
		}
		if seen[s.ID] {
			return fmt.Errorf("two shots are called %s", s.ID)
		}
		seen[s.ID] = true
		if s.Beats[1] <= s.Beats[0] {
			return fmt.Errorf("%s: the end beat (%g) must come after the start (%g)", s.ID, s.Beats[1], s.Beats[0])
		}
		if i > 0 && s.Beats[0] < sb.Shots[i-1].Beats[1] {
			return fmt.Errorf("%s starts at beat %g, before %s ends (%g)", s.ID, s.Beats[0], sb.Shots[i-1].ID, sb.Shots[i-1].Beats[1])
		}
	}
	return nil
}

// Label is the caption under a frame.
func (sb *Board) Label(s Shot) string {
	return fmt.Sprintf("%s  beats %g-%g  %.1f-%.1fs", s.ID, s.Beats[0], s.Beats[1], sb.Time(s.Beats[0]), sb.Time(s.Beats[1]))
}

// Normalize converts any image ffmpeg can read into a PNG of exactly w x h, letterboxed.
func Normalize(ctx context.Context, in, out string, w, h int) error {
	vf := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=0x16181d,format=rgba", w, h, w, h)
	b, err := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-y", "-i", in, "-vf", vf, "-frames:v", "1", out).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg could not read %s: %v: %s", in, err, strings.TrimSpace(string(b)))
	}
	return nil
}

// Caption draws a strip with text along the bottom of a PNG, in place.
func Caption(path, text string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	src, err := png.Decode(f)
	f.Close()
	if err != nil {
		return err
	}
	b := src.Bounds()
	img := image.NewRGBA(b)
	draw.Draw(img, b, src, b.Min, draw.Src)
	s := max(2, b.Dy()/270)
	y := b.Dy() - 11*s - s*2
	draw.Draw(img, image.Rect(0, y-s, b.Dx(), b.Dy()), &image.Uniform{color.RGBA{0, 0, 0, 170}}, image.Point{}, draw.Over)
	maxChars := (b.Dx() - 4*s) / (6 * s)
	if len(text) > maxChars {
		text = text[:max(0, maxChars-3)] + "..."
	}
	sheet.Label(img, 2*s, y, text, s, color.Transparent)
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	return png.Encode(out, img)
}
