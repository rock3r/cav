package main

import (
	"context"
	"errors"
	"os"
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
