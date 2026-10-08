package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMusicRenderNamesMissingFilesBeforeRunningPython(t *testing.T) {
	dir := t.TempDir()
	score := filepath.Join(dir, "score.json")
	present := filepath.Join(dir, "kick.wav")
	os.WriteFile(present, []byte("RIFF"), 0o644)
	os.WriteFile(score, []byte(`{"bpm": 120, "beats": 4, "tracks": [
	  {"name": "kick", "instrument": "sample:`+present+`", "hits": [0]},
	  {"name": "fx", "instrument": "sample:/nope/whoosh.wav", "hits": [1]},
	  {"name": "lead", "instrument": "vst3:/nope/Synth.vst3", "preset": "/nope/lead.vstpreset", "notes": []},
	  {"name": "bass", "instrument": "bass", "notes": []}],
	 "master": {"reference": "/nope/ref.wav"}}`), 0o644)
	t.Setenv("PATH", t.TempDir()) // no uv: the check must fail first
	err := cmdMusicRender(&app{}, []string{"-o", filepath.Join(dir, "out.wav"), score})
	if err == nil {
		t.Fatal("want an error for the missing files")
	}
	for _, want := range []string{"track fx: sample /nope/whoosh.wav", "track lead: vst3 /nope/Synth.vst3",
		"track lead: preset /nope/lead.vstpreset", "master.reference /nope/ref.wav"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
	if strings.Contains(err.Error(), present) {
		t.Errorf("error names a file that exists: %q", err)
	}
}
