package beats

import (
	"math"
	"math/rand"
	"testing"
)

// synth makes a drum loop: kick on beats 1 and 3, snare-ish noise on 2 and 4, hats on 8ths.
func synth(bpm, offset, seconds float64) []float64 {
	n := int(seconds * SampleRate)
	s := make([]float64, n)
	r := rand.New(rand.NewSource(1))
	period := 60 / bpm
	for i := 0; ; i++ {
		t := offset + float64(i)*period/2
		if t >= seconds {
			break
		}
		start := int(t * SampleRate)
		beat := i / 2
		for k := 0; k < SampleRate/5 && start+k < n; k++ {
			tt := float64(k) / SampleRate
			env := math.Exp(-tt * 30)
			var v float64
			switch {
			case i%2 == 1:
				v = 0.15 * (r.Float64()*2 - 1) * math.Exp(-tt*80) // hat
			case beat%2 == 0:
				v = 0.9 * math.Sin(2*math.Pi*55*tt) * env // kick
			default:
				v = 0.5 * (r.Float64()*2 - 1) * math.Exp(-tt*20) // snare
			}
			s[start+k] += v
		}
	}
	return s
}

func TestAnalyzeFindsTempoAndPhase(t *testing.T) {
	for _, bpm := range []float64{90, 120, 128, 140} {
		res := Analyze(synth(bpm, 0.25, 16), 0)
		if math.Abs(res.BPM-bpm) > 1.5 {
			t.Fatalf("bpm %v: got %v", bpm, res.BPM)
		}
		period := 60 / bpm
		for _, b := range res.Beats[1 : len(res.Beats)-1] {
			ph := math.Mod(b-0.25+period*100, period)
			if ph > period/2 {
				ph -= period
			}
			if math.Abs(ph) > 0.035 {
				t.Fatalf("bpm %v: beat %.3f is %.3f s off the grid", bpm, b, ph)
			}
		}
	}
}

func TestGrid(t *testing.T) {
	g := Grid(120, 0.5, 4)
	if len(g.Beats) != 8 || g.Beats[1] != 1.0 || len(g.Downbeats) != 2 {
		t.Fatalf("grid: %+v", g)
	}
}
