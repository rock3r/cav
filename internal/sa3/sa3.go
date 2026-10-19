// Package sa3 makes music and sound effects locally with Stable Audio 3 on Apple silicon:
// the pure-MLX runtime of the official repository (MIT-licensed code), fetched at a
// pinned commit into the cav cache and run through uv. The model weights download once
// from Hugging Face into the shared HF cache (the same one mflux uses); after that every
// run works offline. Nothing is uploaded and no key is needed.
//
// The weights are under the Stability AI Community License; library.GeneratedTerms
// records that on every take.
package sa3

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/rock3r/cav/internal/config"
	"github.com/rock3r/cav/internal/ghfetch"
	"github.com/rock3r/cav/internal/pyrun"
	"github.com/rock3r/cav/internal/services"
)

// Commit pins the Stable Audio 3 repository (https://github.com/Stability-AI/stable-audio-3),
// main as of 2026-10-11. The MLX runtime lives in its optimized/mlx directory.
const Commit = "77e0398c26f7d22d801ec1bb036307e8b231f741"

// Kinds of audio sa3 makes.
const (
	SFX   = "sfx"
	Music = "music"
)

// The TrackType prefixes the models were trained with (the official repo's
// stable_audio_3/interface/reprompt.py). Without the SFX prefix a sound-effect request
// often comes out as music.
const (
	PrefixSFX   = "TrackType: SFX, "
	PrefixMusic = "TrackType: Music, VocalType: Instrumental, "
)

// model describes one SA3 DiT bundle: the decoder it pairs with, the longest take it
// makes well (the small models drift past about two minutes; medium runs to about 6:20),
// and the size of its first download, the shared T5Gemma encoder included.
type model struct {
	decoder string
	maxSec  float64
	size    string
}

var models = map[string]model{
	"sm-sfx":   {"same-s", 120, "about 1.9 GB"},
	"sm-music": {"same-s", 120, "about 1.9 GB"},
	"medium":   {"same-l", 380, "about 6.5 GB"},
}

// DefaultModel is the model a kind runs when the request names none.
func DefaultModel(kind string) string {
	if kind == SFX {
		return "sm-sfx"
	}
	return "sm-music"
}

// Available reports whether sa3 can run here: Apple silicon and uv.
func Available() bool {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return false
	}
	return pyrun.Available()
}

// RuntimeDir is where the pinned Stable Audio 3 source lives (versioned by commit, so a
// pin bump fetches cleanly next to the old one).
func RuntimeDir() string {
	return filepath.Join(config.CacheDir(), "sa3", Commit[:12])
}

// RuntimeReady reports whether the pinned source is fetched.
func RuntimeReady() bool {
	_, err := os.Stat(scriptPath(RuntimeDir()))
	return err == nil
}

func scriptPath(dir string) string {
	return filepath.Join(dir, "optimized", "mlx", "scripts", "sa3_mlx.py")
}

// Ensure fetches the pinned source when missing and returns its directory.
func Ensure(ctx context.Context) (string, error) {
	dir := RuntimeDir()
	if RuntimeReady() {
		return dir, nil
	}
	fmt.Fprintln(os.Stderr, "fetching the Stable Audio 3 runtime (a few MB, once)…")
	return ghfetch.Fetch(ctx, services.HTTPClient,
		"https://codeload.github.com/Stability-AI/stable-audio-3/tar.gz/"+Commit, dir, RuntimeReady)
}

// Request is one take to make.
type Request struct {
	Kind    string  // SFX or Music
	Prompt  string  // without the TrackType prefix; sa3 adds the kind's
	Seconds float64 // 0: 5 s for a sound effect, 30 s for music
	Model   string  // empty: the kind's default
	Seed    int64   // negative: pick one at random
	// InitAudio restyles an existing file (audio-to-audio). InitNoise is σmax: 0.4-0.8
	// keeps most of the file, 1 regenerates it. The file is trimmed or zero-padded to
	// Seconds.
	InitAudio string
	InitNoise float64
}

// Result is the take.
type Result struct {
	Data  []byte // 16-bit 44.1 kHz stereo WAV, exactly Seconds long
	Model string // the model that ran, for the manifest
	Seed  int64
	Notes []string
}

// promptFor adds the kind's TrackType prefix, unless the prompt already carries one.
func promptFor(kind, prompt string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(prompt)), "tracktype:") {
		return prompt
	}
	if kind == SFX {
		return PrefixSFX + prompt
	}
	return PrefixMusic + prompt
}

// Generate makes one take. Model downloads and sampling progress stream to stderr.
func Generate(ctx context.Context, r Request) (*Result, error) {
	name := r.Model
	if name == "" {
		name = DefaultModel(r.Kind)
	}
	m, ok := models[name]
	if !ok {
		return nil, fmt.Errorf("unknown sa3 model %q (models: sm-sfx, sm-music, medium)", name)
	}
	if r.Kind == SFX && name == "medium" {
		return nil, fmt.Errorf("medium is a music model; sound effects use sm-sfx")
	}
	if !Available() {
		return nil, fmt.Errorf("sa3 needs a Mac with Apple silicon and uv (https://docs.astral.sh/uv/)")
	}
	seconds := r.Seconds
	if seconds <= 0 {
		if r.Kind == SFX {
			seconds = 5
		} else {
			seconds = 30
		}
	}
	var notes []string
	if seconds > m.maxSec {
		notes = append(notes, fmt.Sprintf("%s makes at most %g s well, so the take is %g s instead of %g s", name, m.maxSec, m.maxSec, seconds))
		seconds = m.maxSec
	}
	seed := r.Seed
	if seed < 0 {
		n, err := rand.Int(rand.Reader, big.NewInt(1<<31))
		if err != nil {
			return nil, err
		}
		seed = n.Int64()
	}
	dir, err := Ensure(ctx)
	if err != nil {
		return nil, err
	}
	if cached, known, size := services.SA3Weights(name); known && !cached {
		fmt.Fprintf(os.Stderr, "note: the first run downloads the %s weights (%s) into %s; this can take a long time\n",
			name, size, services.HFHubCache())
	}
	tmp, err := os.MkdirTemp("", "cav-sa3")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	out := filepath.Join(tmp, "out.wav")
	mlxDir := filepath.Join(dir, "optimized", "mlx")
	args := []string{"run", "--no-project", "--quiet", "--python", "3.11",
		"--with-requirements", filepath.Join(mlxDir, "requirements.txt"),
		"python", scriptPath(dir),
		"--prompt", promptFor(r.Kind, r.Prompt), "--dit", name, "--decoder", m.decoder,
		"--seconds", strconv.FormatFloat(seconds, 'f', -1, 64),
		"--seed", strconv.FormatInt(seed, 10),
		"--out", out}
	if r.InitAudio != "" {
		noise := r.InitNoise
		if noise <= 0 {
			noise = 1
		}
		args = append(args, "--init-audio", r.InitAudio,
			"--init-noise-level", strconv.FormatFloat(noise, 'f', 2, 64))
	}
	cmd := exec.CommandContext(ctx, "uv", args...)
	cmd.Stdout = os.Stderr // the runtime prints download and stage progress on stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("sa3 stopped: %w (the first run downloads the model; run it again to continue: the files that finished downloading are kept)", ctx.Err())
		}
		return nil, fmt.Errorf("sa3: %w", err)
	}
	data, err := os.ReadFile(out)
	if err != nil || len(data) == 0 {
		return nil, fmt.Errorf("sa3 wrote no audio: %w", err)
	}
	return &Result{Data: data, Model: name, Seed: seed, Notes: notes}, nil
}
