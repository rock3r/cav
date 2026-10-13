package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/library"
	"github.com/rock3r/cav/internal/music"
)

// With --lyrics and no --service, cav tries only the services that sing, so a configured
// ElevenLabs key is not picked, and the hint names the singing servers.
func TestMusicLyricsPicksOnlySingers(t *testing.T) {
	t.Setenv("CAV_HOME", t.TempDir())
	t.Setenv("ELEVENLABS_API_KEY", "k")
	song := filepath.Join(t.TempDir(), "song.txt")
	if err := os.WriteFile(song, []byte("[Verse]\nla la"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := cmdMusic(&app{}, []string{"gen", "pop", "--lyrics", song})
	var ce *cliError
	if !errors.As(err, &ce) || !strings.Contains(ce.msg, "yue2") || !strings.Contains(ce.hint, "--lyrics needs a server that sings") {
		t.Fatalf("got %v", err)
	}
	err = cmdMusic(&app{}, []string{"gen", "pop", "--lyrics", song, "--service", "elevenlabs"})
	if !errors.As(err, &ce) || !strings.Contains(ce.msg, "makes only instrumentals") {
		t.Fatalf("--service elevenlabs: %v", err)
	}
}

// A named service that cannot sing is refused before cav reads its key, so a key in
// 1Password is never asked for. Here Lyria has no key at all: a key lookup would fail
// with "no key" instead of the refusal.
func TestMusicLyricsRefusesANonSingerBeforeItsKey(t *testing.T) {
	t.Setenv("CAV_HOME", t.TempDir())
	for _, v := range []string{"GEMINI_API_KEY", "GOOGLE_API_KEY"} {
		t.Setenv(v, "")
	}
	song := filepath.Join(t.TempDir(), "song.txt")
	if err := os.WriteFile(song, []byte("[Verse]\nla la"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := cmdMusic(&app{}, []string{"gen", "pop", "--lyrics", song, "--service", "lyria"})
	var ce *cliError
	if !errors.As(err, &ce) || !strings.Contains(ce.msg, "lyria makes only instrumentals") {
		t.Fatalf("got %v", err)
	}
	// An unknown name, or a service for another job, gets services.Pick's own error.
	for _, name := range []string{"lyriaa", "openai"} {
		err = cmdMusic(&app{}, []string{"gen", "pop", "--lyrics", song, "--service", name})
		if !errors.As(err, &ce) || strings.Contains(ce.msg, "makes only instrumentals") {
			t.Fatalf("--service %s: %v", name, err)
		}
	}
}

// The manifest keeps the words of a take, so a song can be made again from its record.
func TestSaveTakeRecordsTheLyrics(t *testing.T) {
	root := t.TempDir()
	tr := &music.Track{Data: []byte("RIFFsong"), Ext: ".wav", Provider: "acestep", Model: "m"}
	path, _, err := saveTake(context.Background(), root, filepath.Join(root, "music"), "song", tr, "pop", "[Verse]\nla la", "", false)
	if err != nil {
		t.Fatal(err)
	}
	m, err := library.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range m.Assets {
		if e.Path == library.Rel(root, path) {
			if e.Generated == nil || e.Generated.Lyrics != "[Verse]\nla la" || e.Generated.Prompt != "pop" {
				t.Fatalf("record: %+v", e.Generated)
			}
			return
		}
	}
	t.Fatalf("no record for %s in %+v", path, m.Assets)
}

// An order set without a singer stays as it is: cav does not add YuE2 or ACE-Step back.
func TestMusicLyricsRespectsAnOrderWithoutSingers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAV_HOME", home)
	cfg := `{"order":{"music":["elevenlabs"]},"endpoints":{"yue2":{"baseUrl":"http://127.0.0.1:1"}}}`
	if err := os.WriteFile(filepath.Join(home, "services.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	song := filepath.Join(t.TempDir(), "song.txt")
	os.WriteFile(song, []byte("[Verse]\nla la"), 0o600)
	err := cmdMusic(&app{}, []string{"gen", "pop", "--lyrics", song})
	var ce *cliError
	if !errors.As(err, &ce) || !strings.Contains(ce.msg, "has no service that sings") {
		t.Fatalf("got %v", err)
	}
}

// Lyrics longer than ACE-Step's 4096 characters skip it when cav picks a service.
func TestMusicLyricsSkipsASingerWithTooSmallALimit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAV_HOME", home)
	cfg := `{"order":{"music":["acestep"]},"endpoints":{"acestep":{"baseURL":"http://127.0.0.1:1"}}}`
	if err := os.WriteFile(filepath.Join(home, "services.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	song := filepath.Join(t.TempDir(), "song.txt")
	os.WriteFile(song, []byte("[Verse]\n"+strings.Repeat("la ", 1500)), 0o600)
	err := cmdMusic(&app{}, []string{"gen", "pop", "--lyrics", song})
	var ce *cliError
	if !errors.As(err, &ce) || !strings.Contains(ce.msg, "lyrics this long") {
		t.Fatalf("got %v", err)
	}
}

// In the paid check for #11, a Stable Audio take replaced an ElevenLabs take made from
// the same prompt. A second take with the same name now gets name-2.
func TestSaveTakeKeepsAnEarlierTake(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "music")
	first := &music.Track{Data: []byte("ID3elevenlabs"), Ext: ".mp3", Provider: "elevenlabs"}
	second := &music.Track{Data: []byte("ID3stability"), Ext: ".mp3", Provider: "stability"}
	p1, _, err := saveTake(context.Background(), root, dir, "synthwave", first, "synthwave", "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	p2, _, err := saveTake(context.Background(), root, dir, "synthwave", second, "synthwave", "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p1) != "synthwave.mp3" || filepath.Base(p2) != "synthwave-2.mp3" {
		t.Fatalf("paths %s, %s", p1, p2)
	}
	if b, _ := os.ReadFile(p1); string(b) != "ID3elevenlabs" {
		t.Fatal("the first take was replaced")
	}
	m, err := library.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Assets) != 2 {
		t.Fatalf("manifest has %d entries, want 2", len(m.Assets))
	}
}

// YuE2 sends FLAC and cav converts it to WAV. A second take must not convert over the
// first take's WAV (review of #108).
func TestSaveTakeKeepsAnEarlierFLACTake(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("needs ffmpeg")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "music")
	var flacs [][]byte
	for _, hz := range []string{"440", "880"} {
		p := filepath.Join(t.TempDir(), "t.flac")
		if b, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "sine=frequency="+hz+":duration=0.5", p).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, b)
		}
		data, _ := os.ReadFile(p)
		flacs = append(flacs, data)
	}
	var paths []string
	for _, data := range flacs {
		p, note, err := saveTake(context.Background(), root, dir, "song", &music.Track{Data: data, Ext: ".flac", Provider: "yue2"}, "pop", "", "", false)
		if err != nil || note != "" {
			t.Fatalf("saveTake: %v %q", err, note)
		}
		paths = append(paths, p)
	}
	if filepath.Base(paths[0]) != "song.wav" || filepath.Base(paths[1]) != "song-2.wav" {
		t.Fatalf("paths %v", paths)
	}
	a, _ := os.ReadFile(paths[0])
	b, _ := os.ReadFile(paths[1])
	if len(a) == 0 || bytes.Equal(a, b) {
		t.Fatal("the second take replaced the first")
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*.flac")); len(left) != 0 {
		t.Fatalf("FLAC files left over: %v", left)
	}
}
