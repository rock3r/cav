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
		// The video's own soundtrack stays silent, and the render has no tail.
		`NAME "Video"` + "\n    VOLPAN 0 0 -1 -1 1", "RENDER_RANGE 1 0 0 0 1000",
	} {
		if !strings.Contains(rpp, want) {
			t.Errorf("missing %q in:\n%s", want, rpp)
		}
	}
	// REAPER 7 shows a "Project Load Warning" for tokens it does not know.
	for _, unknown := range []string{"RENDER_SRATE"} {
		if strings.Contains(rpp, unknown) {
			t.Errorf("REAPER does not know %s:\n%s", unknown, rpp)
		}
	}
	// Every block that opens closes.
	if strings.Count(rpp, "<") != strings.Count(rpp, "\n  >")+strings.Count(rpp, "\n    >")+strings.Count(rpp, "\n      >")+1 {
		t.Errorf("unbalanced blocks:\n%s", rpp)
	}
}

func TestWithAudioDevice(t *testing.T) {
	for _, c := range []struct{ name, goos, in, want string }{
		{"mac, no file", "darwin", "", "[reaper]\ncoreaudioindevnew=<default>\ncoreaudiooutdevnew=<default>\ncoreaudiobs=512\ncoreaudiosrate=48000\n"},
		{"mac, fresh install", "darwin",
			"[nag]\nnag=x\n\n[reaper]\nwnd_w=1024\n\n[Recent]\nrecent01=a.rpp\n",
			"[nag]\nnag=x\n\n[reaper]\nwnd_w=1024\ncoreaudioindevnew=<default>\ncoreaudiooutdevnew=<default>\ncoreaudiobs=512\ncoreaudiosrate=48000\n\n[Recent]\nrecent01=a.rpp\n"},
		{"mac, keeps the user's buffer size", "darwin",
			"[reaper]\ncoreaudiobs=256\n",
			"[reaper]\ncoreaudiobs=256\ncoreaudioindevnew=<default>\ncoreaudiooutdevnew=<default>\ncoreaudiosrate=48000\n"},
		{"windows, fresh install keeps CRLF", "windows",
			"[reaper]\r\nrenderclosewhendone=4\r\nmixwnd_vis=1\r\n",
			"[reaper]\r\nrenderclosewhendone=4\r\nmixwnd_vis=1\r\n[audioconfig]\r\nmode=2\r\n"},
		{"windows, section without a mode", "windows",
			"[audioconfig]\nwasapi_bs=512\n[reaper]\nx=1\n",
			"[audioconfig]\nwasapi_bs=512\nmode=2\n[reaper]\nx=1\n"},
	} {
		got, changed := WithAudioDevice(c.in, c.goos)
		if !changed || got != c.want {
			t.Errorf("%s: changed=%v\n got: %q\nwant: %q", c.name, changed, got, c.want)
		}
	}
	for _, c := range []struct{ goos, ini string }{
		{"darwin", "[reaper]\ncoreaudiooutdevnew=MOTU\n"},
		{"windows", "[audioconfig]\nmode=0\n"},
		{"linux", "[reaper]\n"},
	} {
		if got, changed := WithAudioDevice(c.ini, c.goos); changed || got != c.ini {
			t.Errorf("%s: changed a device the user picked: %q", c.goos, got)
		}
	}
}
