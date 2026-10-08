package score

import (
	"strings"
	"testing"
)

func TestRPPStructure(t *testing.T) {
	p := Project{BPM: 120, Seconds: 16, Video: "/p/renders/final.mp4", RenderTo: "/p/score/final-music.wav",
		Markers: []Marker{{"s1: lines draw in", 0}, {"s2: logo \"pops\"", 2}},
		Takes:   []Take{{Path: "/p/music/take1.mp3"}, {Path: "/p/music/take2.wav", Seconds: 17.5}}}
	rpp := p.RPP()
	for _, want := range []string{
		"<REAPER_PROJECT 0.1", "TEMPO 120 4 4", "SAMPLERATE 48000 1 0",
		`RENDER_FILE "/p/score"`, `RENDER_PATTERN "final-music"`,
		`MARKER 1 0.000000 "s1: lines draw in" 0 0 1`, `MARKER 2 2.000000 "s2: logo 'pops'" 0 0 1`,
		"<SOURCE VIDEO", `FILE "/p/renders/final.mp4"`, "<SOURCE MP3", "<SOURCE WAVE", "LENGTH 17.500000",
		`NAME "Bed 2: take2.wav"` + "\n    MUTESOLO 1 0 0", `NAME "SFX"`,
	} {
		if !strings.Contains(rpp, want) {
			t.Errorf("missing %q in:\n%s", want, rpp)
		}
	}
	// Every block that opens closes.
	if strings.Count(rpp, "<") != strings.Count(rpp, "\n  >")+strings.Count(rpp, "\n    >")+strings.Count(rpp, "\n      >")+1 {
		t.Errorf("unbalanced blocks:\n%s", rpp)
	}
}
