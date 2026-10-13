package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rock3r/cav/internal/listen"
)

// Stable Audio's audio-to-audio takes MP3 or WAV, 6 to 190 seconds long (Stability's
// OpenAPI spec, read on 2026-10-09), in a request of at most 50 MB (its API reference).
// cav converts a bigger file to 16-bit 44.1 kHz stereo WAV: 190 s of that is about 34 MB.
const (
	stabilityRefMin, stabilityRefMax = 6.0, 190.0
	stabilityRefBytes                = 45 << 20 // leaves room for the rest of the form
)

// A YuE2 cover is a form of at most 40 MiB (YuE2-Turbo's limit, which cav's server copies).
// cav sends a bigger track as 192 kbps MP3: 39 MiB of that is about 28 minutes.
const yue2RefBytes = 39 << 20

// prepareRef makes a reference track fit the service. For Stable Audio, cav converts other
// formats (m4a, flac, aiff...) and files too big for one request to WAV, and keeps the
// first 190 seconds of a longer track; a track shorter than 6 seconds is an error. For a
// YuE2 cover, cav converts a file too big for the upload to MP3. Other services get the
// file as it is. It returns the file to send and a note when cav changed it.
func prepareRef(ctx context.Context, service, path, tmp string) (string, string, error) {
	if service == "yue2" {
		st, err := os.Stat(path)
		if err != nil || st.Size() <= yue2RefBytes {
			return path, "", err
		}
		dir, err := os.MkdirTemp(tmp, "ref")
		if err != nil {
			return "", "", err
		}
		out := filepath.Join(dir, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+".mp3")
		if b, err := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-y", "-i", path, "-vn", "-c:a", "libmp3lame", "-b:a", "192k", out).CombinedOutput(); err != nil {
			return "", "", fmt.Errorf("converting %s: %v: %s", path, err, strings.TrimSpace(string(b)))
		}
		return out, "converted to 192 kbps MP3 to fit the 40 MiB cover upload", nil
	}
	if service != "stability" {
		return path, "", nil
	}
	dur, err := audioSeconds(ctx, path)
	if err != nil {
		return path, "", nil // without ffprobe, send it as it is; the service says what is wrong
	}
	if dur < stabilityRefMin {
		return "", "", fmt.Errorf("%s is %.1f s long; Stable Audio needs a reference of %g to %g s", path, dur, stabilityRefMin, stabilityRefMax)
	}
	ext := strings.ToLower(filepath.Ext(path))
	st, err := os.Stat(path)
	if err != nil {
		return "", "", err
	}
	if dur <= stabilityRefMax && (ext == ".mp3" || ext == ".wav") && st.Size() <= stabilityRefBytes {
		return path, "", nil
	}
	// A folder of its own, so two references with the same name do not share a file.
	dir, err := os.MkdirTemp(tmp, "ref")
	if err != nil {
		return "", "", err
	}
	out := filepath.Join(dir, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+".wav")
	args := []string{"-v", "error", "-y", "-i", path, "-vn"}
	note := "converted to WAV for Stable Audio"
	if ext == ".mp3" || ext == ".wav" {
		note = "converted to 16-bit 44.1 kHz stereo WAV to fit Stable Audio's 50 MB request limit"
	}
	if dur > stabilityRefMax {
		args = append(args, "-t", strconv.FormatFloat(stabilityRefMax, 'f', 0, 64))
		note = fmt.Sprintf("Stable Audio takes at most %g s, so cav sent the first %g s", stabilityRefMax, stabilityRefMax)
	}
	if b, err := exec.CommandContext(ctx, "ffmpeg", append(args, "-ac", "2", "-ar", "44100", "-c:a", "pcm_s16le", out)...).CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("converting %s: %v: %s", path, err, strings.TrimSpace(string(b)))
	}
	return out, note, nil
}

// refNames gives each reference track a distinct name for its takes: the slug of its file
// name, with -2, -3... when two references would get the same one.
func refNames(paths []string) []string {
	used := map[string]bool{}
	names := make([]string, len(paths))
	for i, p := range paths {
		base := safeSlug(strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)))
		name := base
		for n := 2; used[name]; n++ {
			name = fmt.Sprintf("%s-%d", base, n)
		}
		used[name] = true
		names[i] = name
	}
	return names
}

func audioSeconds(ctx context.Context, path string) (float64, error) {
	b, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", path).Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
}

// flacToWAV converts a FLAC take to 24-bit WAV next to it and removes the FLAC. It needs
// ffmpeg.
func flacToWAV(ctx context.Context, path, wav string) error {
	if b, err := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-y", "-i", path, "-vn", "-c:a", "pcm_s24le", wav).CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg: %v: %s", err, strings.TrimSpace(string(b)))
	}
	return nil
}

// saveFLACTake reserves base.wav (or base-2.wav, …), converts the FLAC into it, and
// returns its path. When the conversion fails, it keeps the FLAC under a free name instead
// and says so in the note.
func saveFLACTake(ctx context.Context, base string, data []byte) (path, note string, err error) {
	wav, err := writeFreeData(base, ".wav", nil)
	if err != nil {
		return "", "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(base), ".take-*.flac")
	if err != nil {
		os.Remove(wav)
		return "", "", err
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	defer os.Remove(tmp.Name())
	if err == nil {
		if err = flacToWAV(ctx, tmp.Name(), wav); err == nil {
			return wav, "", nil
		}
	}
	os.Remove(wav)
	flac, ferr := writeFreeData(strings.TrimSuffix(wav, ".wav"), ".flac", data)
	if ferr != nil {
		return "", "", ferr
	}
	return flac, "kept as FLAC (" + err.Error() + ")", nil
}

// takeFade is how many seconds of fade-out cav puts at the end of a take it cuts.
const takeFade = 2.0

// cutAndFade cuts a take to seconds and fades out its last fade seconds, in place. cav does
// this to an ACE-Step take, which it asks for longer than needed because ACE-Step ends its
// takes early (see music.Track.Length). It keeps the format of the file. It needs ffmpeg.
func cutAndFade(ctx context.Context, path string, seconds, fade float64) error {
	fade = math.Min(fade, seconds)
	ext := strings.ToLower(filepath.Ext(path))
	tmp := strings.TrimSuffix(path, filepath.Ext(path)) + ".cut" + ext
	args := []string{"-v", "error", "-y", "-i", path, "-vn", "-t", strconv.FormatFloat(seconds, 'f', 3, 64),
		"-af", fmt.Sprintf("afade=t=out:st=%.3f:d=%.3f", seconds-fade, fade)}
	if b, err := exec.CommandContext(ctx, "ffmpeg", append(append(args, takeCodec(ext)...), tmp)...).CombinedOutput(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("ffmpeg: %v: %s", err, strings.TrimSpace(string(b)))
	}
	return os.Rename(tmp, path)
}

// takeCodec is the ffmpeg codec cav writes a take of this extension with.
func takeCodec(ext string) []string {
	switch ext {
	case ".wav":
		return []string{"-c:a", "pcm_s24le"}
	case ".flac":
		return []string{"-c:a", "flac"}
	}
	return []string{"-c:a", "libmp3lame", "-b:a", "320k"}
}

// Takes from every music service came back with true peaks above 0 dBFS in tests on
// 2026-10-09, so they can clip once encoded again. cav lowers the gain of such a take to
// headroomTarget (below the -1 dBTP that cav listen asks for, to leave room for the MP3
// encoder). It changes the level only: no compression or limiting.
const headroomPeak, headroomTarget = -1.0, -1.5

// addHeadroom lowers the gain of a take whose true peak is above -1 dBTP and returns the
// change in dB (0 when the take was fine). It needs ffmpeg.
func addHeadroom(ctx context.Context, path string) (float64, error) {
	l, err := listen.Measure(ctx, path)
	if err != nil {
		return 0, err
	}
	if l.TruePeak <= headroomPeak {
		return 0, nil
	}
	gain := headroomTarget - l.TruePeak
	ext := strings.ToLower(filepath.Ext(path))
	tmp := strings.TrimSuffix(path, filepath.Ext(path)) + ".level" + ext
	args := append([]string{"-v", "error", "-y", "-i", path, "-vn", "-af", fmt.Sprintf("volume=%.2fdB", gain)}, takeCodec(ext)...)
	if b, err := exec.CommandContext(ctx, "ffmpeg", append(args, tmp)...).CombinedOutput(); err != nil {
		os.Remove(tmp)
		return 0, fmt.Errorf("ffmpeg: %v: %s", err, strings.TrimSpace(string(b)))
	}
	return gain, os.Rename(tmp, path)
}
