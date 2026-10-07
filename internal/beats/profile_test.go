package beats

import (
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// track makes 16 s at 120 BPM over a pad: a quiet kick for 4 s, then loud kicks and a mid
// "stab" a 16th note after beat 2 of every bar (off the grid), with 0.5 s of silence at 10 s.
func track() []float64 {
	s := make([]float64, 16*SampleRate)
	hit := func(at, freq, amp, decay float64) {
		start := int(at * SampleRate)
		for i := 0; i < SampleRate/4 && start+i < len(s); i++ {
			t := float64(i) / SampleRate
			s[start+i] += amp * math.Exp(-t*decay) * math.Sin(2*math.Pi*freq*t)
		}
	}
	// A pad under everything, quiet before the drop, so the track never falls silent by itself.
	for i := range s {
		amp := 0.02
		if i >= 4*SampleRate {
			amp = 0.15
		}
		s[i] = amp * math.Sin(2*math.Pi*220*float64(i)/SampleRate)
	}
	for b := 0; b < 32; b++ {
		t := float64(b) * 0.5
		amp := 0.05
		if t >= 4 {
			amp = 0.6
		}
		hit(t, 60, amp, 18)
		if t >= 4 && b%4 == 1 {
			hit(t+0.125, 900, 0.5, 25)
		}
	}
	for i := 10 * SampleRate; i < 21*SampleRate/2; i++ {
		s[i] = 0
	}
	return s
}

func TestDescribeFindsTheDropTheSilenceAndOffbeatStabs(t *testing.T) {
	s := track()
	r := Analyze(s, 120)
	if math.Abs(r.BPM-120) > 1 {
		t.Fatalf("bpm %.2f", r.BPM)
	}
	st := NewProfile(s).Describe(r)
	if len(st.Sections) < 2 || math.Abs(st.Sections[1].Start-4) > 0.5 || st.Sections[1].Change < 10 {
		t.Fatalf("sections %+v", st.Sections)
	}
	var rise, silence bool
	for _, e := range st.Events {
		rise = rise || (e.Kind == "rise" && math.Abs(e.Start-4) < 0.3)
		silence = silence || (e.Kind == "silence" && e.Start > 9.8 && e.End < 10.7)
	}
	if !rise || !silence {
		t.Fatalf("events %+v", st.Events)
	}
	stab := false
	for _, a := range st.Accents {
		if a.Band != "low" && !a.OnGrid && math.Abs(a.Beat-math.Floor(a.Beat)-0.25) < 0.1 {
			stab = true
		}
	}
	if !stab {
		t.Fatalf("no off-beat mid stab in %+v", st.Accents)
	}
	if len(st.BarEnergy) == 0 || st.BarEnergy[0] != 0 {
		t.Fatalf("bar energy %v", st.BarEnergy)
	}
}

func TestPictureDrawsAllLanes(t *testing.T) {
	s := track()
	r := Analyze(s, 120)
	p := NewProfile(s)
	st := p.Describe(r)
	out := filepath.Join(t.TempDir(), "p.png")
	visual := make([]float64, 16*60)
	w, h, err := Picture(p, View{FPS: 60, Grid: r, Structure: &st, Visual: visual, Hits: []Mark{{Time: 4, OK: true}}}, out)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := os.Open(out)
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil || img.Bounds().Dx() != w || img.Bounds().Dy() != h || w != 1600 {
		t.Fatalf("image %v %dx%d: %v", img.Bounds(), w, h, err)
	}
	if _, _, err := Picture(p, View{Start: 5, End: 5}, out); err == nil {
		t.Fatal("an empty range must fail")
	}
}

func TestFindHits(t *testing.T) {
	c := make([]float64, 120)
	for i := range c {
		c[i] = 0.001
	}
	c[20] = 0.3 // a cut
	// A pop: change builds to a peak at frame 60 and decays.
	for i, v := range []float64{0.01, 0.03, 0.06, 0.09, 0.12, 0.09, 0.06, 0.04, 0.02, 0.01} {
		c[56+i] = v
	}
	// A move that lands at frame 100.
	for i := 90; i < 100; i++ {
		c[i] = 0.05
	}
	got := FindHits(c)
	want := map[int]string{20: "cut", 60: "peak", 100: "stop"}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for _, h := range got {
		if want[h.Frame] != h.Kind {
			t.Fatalf("got %+v, want %v", got, want)
		}
	}
}
