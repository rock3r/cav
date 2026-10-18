package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// indexSoundLibrary makes a folder of (empty) sound files and registers it as a local
// library in the test's CAV_HOME.
func indexSoundLibrary(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("RIFF"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := cmdSources(&app{}, "sfx", "audio", "sfx", []string{"index", dir, "--licence", "CC0-1.0"}); err != nil {
		t.Fatal(err)
	}
	return dir
}

// searchJSON runs cav sfx search and returns the emitted JSON.
func searchJSON(t *testing.T, args ...string) map[string]any {
	t.Helper()
	a := &app{json: true}
	var err error
	out := captureStdout(t, func() {
		err = cmdSources(a, "sfx", "audio", "sfx", append([]string{"search"}, args...))
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	var data map[string]any
	if e := json.Unmarshal([]byte(out), &data); e != nil {
		t.Fatalf("not JSON: %v\n%s", e, out)
	}
	return data
}

func notesOf(t *testing.T, data map[string]any) []string {
	t.Helper()
	var out []string
	if ns, ok := data["notes"].([]any); ok { // JSON null when there are none
		for _, n := range ns {
			out = append(out, n.(string))
		}
	}
	return out
}

// Without uv the by-sound ranking cannot run: the file-name search answers instead and a
// note says so.
func TestSfxSearchBySoundFallsBackWithoutUV(t *testing.T) {
	t.Setenv("CAV_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir()) // no uv
	t.Chdir(t.TempDir())
	indexSoundLibrary(t, "deep_whoosh_01.wav", "rec_0042.wav")

	data := searchJSON(t, "--service", "local", "--by-sound", "deep", "whoosh")
	results := data["results"].([]any)
	if len(results) != 1 || !strings.Contains(results[0].(map[string]any)["id"].(string), "deep_whoosh_01.wav") {
		t.Fatalf("want the file-name match as a fallback, got %v", results)
	}
	if _, ok := results[0].(map[string]any)["sound"]; ok {
		t.Fatalf("a fallback result must not carry a by-sound score: %v", results[0])
	}
	notes := notesOf(t, data)
	if len(notes) != 1 || !strings.Contains(notes[0], "by sound unavailable") || !strings.Contains(notes[0], "uv is not installed") {
		t.Fatalf("want the fallback note, got %v", notes)
	}
}

// With uv, the helper's ranking sets the order — including files whose names say nothing
// about the sound — and each result carries the match.
func TestSfxSearchBySoundRanksLocalFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake uv is a shell script")
	}
	t.Setenv("CAV_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	indexSoundLibrary(t, "rec_a.wav", "rec_b.wav")
	// The fake uv answers with the second file first: rec_b is the better match.
	fakeUVOnPath(t, `#!/bin/sh
f1=""; f2=""
for a in "$@"; do
  case "$a" in
    *.wav) if [ -z "$f1" ]; then f1="$a"; elif [ -z "$f2" ]; then f2="$a"; fi ;;
  esac
done
printf '{"results":[{"path":"%s","z":3.1,"cosine":0.33,"verdict":"strong","soundsMostLike":"a triangle hit (z +3.2)","nearSilent":false},{"path":"%s","z":-0.7,"cosine":-0.09,"verdict":"no match","soundsMostLike":"a punchy kick drum pattern (z +4.7)","nearSilent":false}],"skipped":[]}' "$f2" "$f1"
`)

	data := searchJSON(t, "--service", "local", "--by-sound", "a glass ting")
	if n := notesOf(t, data); len(n) != 0 {
		t.Fatalf("want no notes, got %v", n)
	}
	results := data["results"].([]any)
	if len(results) != 2 {
		t.Fatalf("want both files ranked, got %v", results)
	}
	first, second := results[0].(map[string]any), results[1].(map[string]any)
	if !strings.HasSuffix(first["id"].(string), "rec_b.wav") || !strings.HasSuffix(second["id"].(string), "rec_a.wav") {
		t.Fatalf("want the ranker's order (rec_b, rec_a), got %s, %s", first["id"], second["id"])
	}
	s := first["sound"].(map[string]any)
	if s["verdict"] != "strong" || s["z"] != 3.1 || s["soundsLike"] != "a triangle hit (z +3.2)" {
		t.Fatalf("sound match not carried through: %v", s)
	}
	if first["licence"] != "CC0-1.0" {
		t.Fatalf("the library licence must carry through, got %v", first["licence"])
	}
}

// --by-sound ranks the local libraries only; with another service it would silently run
// a file-name search, so it is an error instead.
func TestSfxSearchBySoundRejectsNonLocalServices(t *testing.T) {
	t.Setenv("CAV_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	err := cmdSources(&app{}, "sfx", "audio", "sfx", []string{"search", "--service", "bank", "--by-sound", "whoosh"})
	if err == nil || !strings.Contains(err.Error(), "--by-sound ranks the local libraries") {
		t.Fatalf("want the --by-sound/--service error, got %v", err)
	}
}

// Files earshot cannot read come back as a note, not silence.
func TestSfxSearchBySoundReportsSkippedFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake uv is a shell script")
	}
	t.Setenv("CAV_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	indexSoundLibrary(t, "rec_a.wav", "rec_b.wav")
	fakeUVOnPath(t, `#!/bin/sh
f1=""
for a in "$@"; do
  case "$a" in
    *.wav) if [ -z "$f1" ]; then f1="$a"; fi ;;
  esac
done
printf '{"results":[{"path":"%s","z":3.1,"cosine":0.33,"verdict":"strong","soundsMostLike":"a triangle hit","nearSilent":false}],"skipped":["rec_b.wav"]}' "$f1"
`)

	data := searchJSON(t, "--service", "local", "--by-sound", "a glass ting")
	notes := notesOf(t, data)
	if len(notes) != 1 || !strings.Contains(notes[0], "skipped 1 file(s)") || !strings.Contains(notes[0], "rec_b.wav") {
		t.Fatalf("want the skipped-file note, got %v", notes)
	}
}

// A broken sound-libraries.json is reported like the file-name search reports it, not
// swallowed into an empty result.
func TestSfxSearchBySoundReportsABrokenLibraryList(t *testing.T) {
	t.Setenv("CAV_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	if err := os.WriteFile(librariesPath(), []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}

	data := searchJSON(t, "--service", "local", "--by-sound", "anything")
	notes := notesOf(t, data)
	if len(notes) != 1 || !strings.Contains(notes[0], "local libraries:") {
		t.Fatalf("want the broken-list note, got %v", notes)
	}
}

// The by-sound search reads at most maxBySoundFiles files and says when it stopped early.
func TestSfxSearchBySoundCapsTheLibrary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake uv is a shell script")
	}
	t.Setenv("CAV_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	names := make([]string, maxBySoundFiles+3)
	for i := range names {
		names[i] = fmt.Sprintf("rec_%04d.wav", i)
	}
	indexSoundLibrary(t, names...)
	fakeUVOnPath(t, "#!/bin/sh\nprintf '{\"results\":[],\"skipped\":[]}'\n")

	data := searchJSON(t, "--service", "local", "--by-sound", "anything")
	notes := notesOf(t, data)
	if len(notes) != 1 || !strings.Contains(notes[0], fmt.Sprintf("ranked the first %d of %d", maxBySoundFiles, maxBySoundFiles+3)) {
		t.Fatalf("want the cap note, got %v", notes)
	}
}

func TestRefSearchRejectsBySound(t *testing.T) {
	t.Setenv("CAV_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	err := cmdSources(&app{}, "ref", "image", "moodboard", []string{"search", "--by-sound", "neon"})
	if err == nil || !strings.Contains(err.Error(), "--by-sound is for cav sfx search") {
		t.Fatalf("want a usage error, got %v", err)
	}
}
