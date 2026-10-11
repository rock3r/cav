package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
