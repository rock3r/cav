package beats

import (
	"math"
	"math/cmplx"
	"sort"
)

// The profile uses a longer window than beat detection, for a finer frequency picture.
const (
	profWin = 2048
	// Bands of the spectrogram picture: log-spaced like musical octaves.
	Bands  = 96
	bandLo = 40.0
	bandHi = 11000.0
	// Split points of the three coarse bands (kick and bass / body / hats and air).
	lowHz, highHz = 150.0, 2000.0
)

// HopRate is the number of profile columns per second.
const HopRate = float64(SampleRate) / hop

// Profile holds what a track sounds like over time, one column per hop.
type Profile struct {
	Duration float64
	Spec     [][Bands]float32 // power per band in dB (0 = loudest point of the track)
	BandHz   [Bands + 1]float64
	Loud     []float64 // RMS loudness in dBFS
	Low      []float64 // dB per coarse band, same scale as Spec
	Mid      []float64
	High     []float64
	Flux     []float64    // onset strength, 0..1: the strongest of the three coarse bands
	FluxBand []int        // which coarse band (0 low, 1 mid, 2 high) that is
	BandFlux [3][]float64 // onset strength per coarse band, each scaled to 0..1 on its own
}

// Time returns the centre time of a profile column in seconds.
func (p *Profile) Time(i int) float64 { return (float64(i*hop) + profWin/2) / SampleRate }

// Column returns the profile column at time t, clamped to the track.
func (p *Profile) Column(t float64) int {
	i := int(math.Round((t*SampleRate - profWin/2) / hop))
	return max(0, min(len(p.Loud)-1, i))
}

// NewProfile computes the spectrogram, loudness and onset curves of decoded samples.
func NewProfile(samples []float64) *Profile {
	p := &Profile{Duration: float64(len(samples)) / SampleRate}
	for b := 0; b <= Bands; b++ {
		p.BandHz[b] = bandLo * math.Pow(bandHi/bandLo, float64(b)/Bands)
	}
	n := 0
	if len(samples) >= profWin {
		n = 1 + (len(samples)-profWin)/hop
	}
	window := make([]float64, profWin)
	for i := range window {
		window[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/profWin)
	}
	bins := profWin/2 + 1
	binHz := float64(SampleRate) / profWin
	// Which FFT bins feed each band. A band narrower than one bin uses the nearest bin.
	var bandBins [Bands][2]int
	for b := 0; b < Bands; b++ {
		lo, hi := int(math.Ceil(p.BandHz[b]/binHz)), int(math.Ceil(p.BandHz[b+1]/binHz))
		if hi <= lo {
			lo = int(math.Round(math.Sqrt(p.BandHz[b]*p.BandHz[b+1]) / binHz))
			hi = lo + 1
		}
		bandBins[b] = [2]int{lo, min(hi, bins)}
	}
	lowBin, highBin := int(lowHz/binHz), int(highHz/binHz)
	p.Spec = make([][Bands]float32, n)
	p.Loud, p.Low, p.Mid, p.High = make([]float64, n), make([]float64, n), make([]float64, n), make([]float64, n)
	bandFlux := &p.BandFlux
	for b := range bandFlux {
		bandFlux[b] = make([]float64, n)
	}
	p.FluxBand = make([]int, n)
	buf := make([]complex128, profWin)
	power := make([]float64, bins)
	prev := make([]float64, bins)
	db := func(x float64) float64 { return 10 * math.Log10(x+1e-12) }
	for f := 0; f < n; f++ {
		off := f * hop
		rms := 0.0
		for i := 0; i < profWin; i++ {
			s := samples[off+i]
			rms += s * s
			buf[i] = complex(s*window[i], 0)
		}
		p.Loud[f] = db(rms / profWin)
		fft(buf)
		var coarse, rise [3]float64
		for k := 0; k < bins; k++ {
			power[k] = math.Pow(cmplx.Abs(buf[k]), 2)
			band := 1
			if k < lowBin {
				band = 0
			} else if k >= highBin {
				band = 2
			}
			coarse[band] += power[k]
			m := math.Log1p(100 * math.Sqrt(power[k]))
			if d := m - prev[k]; d > 0 {
				rise[band] += d
			}
			prev[k] = m
		}
		for b := 0; b < Bands; b++ {
			s := 0.0
			for k := bandBins[b][0]; k < bandBins[b][1]; k++ {
				s += power[k]
			}
			p.Spec[f][b] = float32(db(s / float64(bandBins[b][1]-bandBins[b][0])))
		}
		p.Low[f], p.Mid[f], p.High[f] = db(coarse[0]), db(coarse[1]), db(coarse[2])
		for b := range rise {
			bandFlux[b][f] = rise[b]
		}
	}
	// Shift every dB curve so that 0 dB is the loudest point of the spectrogram.
	top := math.Inf(-1)
	for f := range p.Spec {
		for b := 0; b < Bands; b++ {
			top = math.Max(top, float64(p.Spec[f][b]))
		}
	}
	for f := range p.Spec {
		for b := 0; b < Bands; b++ {
			p.Spec[f][b] -= float32(top)
		}
	}
	// Each band is scaled on its own, so a kick or a brass stab can outrank the many small
	// onsets of hi-hats, which touch far more frequency bins.
	for b := range bandFlux {
		bandFlux[b] = normalize(bandFlux[b])
	}
	p.Flux = make([]float64, n)
	for f := 0; f < n; f++ {
		for b := range bandFlux {
			if bandFlux[b][f] > p.Flux[f] {
				p.Flux[f], p.FluxBand[f] = bandFlux[b][f], b
			}
		}
	}
	return p
}

// Section is a stretch of the track that sounds alike: a verse, a drop, a break.
type Section struct {
	Start    float64 `json:"start"` // seconds
	End      float64 `json:"end"`
	Bar      int     `json:"bar"`      // 1-based bar where it starts (0 without downbeats)
	Loudness float64 `json:"loudness"` // mean RMS loudness in dBFS
	Level    string  `json:"level"`    // quiet, medium or loud, relative to this track
	Change   float64 `json:"change"`   // loudness change from the section before, in dB
}

// Event is a moment worth animating that the beat grid does not show.
type Event struct {
	Kind  string  `json:"kind"` // silence, rise (a sudden jump in loudness) or fall
	Start float64 `json:"start"`
	End   float64 `json:"end,omitempty"` // for silence
	DB    float64 `json:"db,omitempty"`  // size of a rise or fall
}

// Accent is a strong onset: a hit, stab or transient.
type Accent struct {
	Time     float64 `json:"time"`
	Strength float64 `json:"strength"` // 0..1
	Band     string  `json:"band"`     // low (kick, bass), mid (snare, stabs, voice) or high (hats)
	Beat     float64 `json:"beat"`     // position on the beat grid, 0 = first beat; .5 is half way
	OnGrid   bool    `json:"onGrid"`   // within 40 ms of a beat or half beat
}

// Structure is the shape of a track: sections, events, accents and energy per bar.
type Structure struct {
	Sections  []Section `json:"sections"`
	Events    []Event   `json:"events"`
	Accents   []Accent  `json:"accents"`
	BarEnergy []float64 `json:"barEnergy"` // mean loudness per bar, 0 = quietest bar, 1 = loudest
}

// mean averages a dB curve from time a to b as power, then returns dB: a short silence
// lowers a loud stretch a little, not to the floor.
//
// Only columns whose whole window lies inside a..b count, so a loud hit just after b does
// not leak into the stretch before it.
func (p *Profile) mean(x []float64, a, b float64) float64 {
	half := profWin / 2.0 / SampleRate
	i, j := p.Column(a+half), p.Column(b-half)
	if j <= i {
		i, j = p.Column((a+b)/2), p.Column((a+b)/2)+1
	}
	s, n := 0.0, 0
	for k := i; k < j && k < len(x); k++ {
		s += math.Pow(10, x[k]/10)
		n++
	}
	return 10 * math.Log10(s/float64(max(n, 1))+1e-12)
}

// Describe finds sections, events and accents, using the beat grid of r.
func (p *Profile) Describe(r Result) Structure {
	var st Structure
	if len(p.Loud) == 0 {
		return st
	}
	beats := r.Beats
	if len(beats) < 2 {
		// No grid: use half-second steps so sections and events still work.
		beats = Grid(120, 0, p.Duration).Beats
	}
	period := (beats[len(beats)-1] - beats[0]) / float64(len(beats)-1)
	edges := append(append([]float64{}, beats...), math.Min(p.Duration, beats[len(beats)-1]+period))

	// One feature vector per beat: loudness, the three coarse bands and 8 spectral bands.
	const nf = 12
	feats := make([][nf]float64, len(beats))
	loudBeat := make([]float64, len(beats))
	for i := range beats {
		a, b := edges[i], edges[i+1]
		loudBeat[i] = p.mean(p.Loud, a, b)
		feats[i][0] = loudBeat[i]
		feats[i][1], feats[i][2], feats[i][3] = p.mean(p.Low, a, b), p.mean(p.Mid, a, b), p.mean(p.High, a, b)
		ia, ib := p.Column(a), max(p.Column(a)+1, p.Column(b))
		for g := 0; g < 8; g++ {
			s, n := 0.0, 0
			for k := ia; k < ib && k < len(p.Spec); k++ {
				for band := g * Bands / 8; band < (g+1)*Bands/8; band++ {
					s += float64(p.Spec[k][band])
					n++
				}
			}
			feats[i][4+g] = s / float64(max(n, 1))
		}
	}
	// Scale each feature to unit spread so that no single one dominates.
	for c := 0; c < nf; c++ {
		m, v := 0.0, 0.0
		for i := range feats {
			m += feats[i][c]
		}
		m /= float64(len(feats))
		for i := range feats {
			v += (feats[i][c] - m) * (feats[i][c] - m)
		}
		sd := math.Sqrt(v/float64(len(feats))) + 1e-9
		for i := range feats {
			feats[i][c] = (feats[i][c] - m) / sd
		}
	}
	dist := func(i, j int) float64 {
		s := 0.0
		for c := 0; c < nf; c++ {
			d := feats[i][c] - feats[j][c]
			s += d * d
		}
		return math.Sqrt(s)
	}
	// Novelty (Foote 2000): how different the 8 beats before each beat are from the 8 after.
	const L = 8
	nov := make([]float64, len(beats))
	for i := L; i+L <= len(beats); i++ {
		cross, within := 0.0, 0.0
		for a := i - L; a < i; a++ {
			for b := i; b < i+L; b++ {
				cross += dist(a, b)
			}
		}
		for a := i - L; a < i+L; a++ {
			for b := a + 1; b < i+L; b++ {
				if (a < i) == (b < i) {
					within += dist(a, b)
				}
			}
		}
		nov[i] = cross/(L*L) - within/(L*(L-1))
	}
	m, v := 0.0, 0.0
	for _, x := range nov {
		m += x
	}
	m /= float64(len(nov))
	for _, x := range nov {
		v += (x - m) * (x - m)
	}
	threshold := m + 0.75*math.Sqrt(v/float64(len(nov)))
	downbeat := map[float64]bool{}
	for _, t := range r.Downbeats {
		downbeat[t] = true
	}
	cuts := []int{0}
	for i := range nov {
		if nov[i] <= threshold || nov[i] <= 0 {
			continue
		}
		peak := true
		for j := max(0, i-4); j <= min(len(nov)-1, i+4); j++ {
			if nov[j] > nov[i] {
				peak = false
			}
		}
		if !peak {
			continue
		}
		// Sections start on a downbeat: move a boundary to one within a beat.
		for _, d := range []int{0, -1, 1} {
			if i+d >= 0 && i+d < len(beats) && downbeat[beats[i+d]] {
				i += d
				break
			}
		}
		if i-cuts[len(cuts)-1] >= 8 {
			cuts = append(cuts, i)
		}
	}
	sorted := append([]float64{}, loudBeat...)
	sort.Float64s(sorted)
	q1, q2 := sorted[len(sorted)/3], sorted[2*len(sorted)/3]
	for k, c := range cuts {
		end := len(beats)
		if k+1 < len(cuts) {
			end = cuts[k+1]
		}
		s := Section{Start: beats[c], End: edges[end]}
		if c == 0 {
			s.Start = 0
		}
		if k == len(cuts)-1 {
			s.End = p.Duration
		}
		s.Loudness = p.mean(p.Loud, s.Start, s.End)
		for b, d := range r.Downbeats {
			if d <= beats[c]+1e-6 {
				s.Bar = b + 1
			}
		}
		switch {
		case s.Loudness < q1:
			s.Level = "quiet"
		case s.Loudness < q2:
			s.Level = "medium"
		default:
			s.Level = "loud"
		}
		if k > 0 {
			s.Change = s.Loudness - st.Sections[k-1].Loudness
		}
		s.Start, s.End, s.Loudness, s.Change = round3(s.Start), round3(s.End), round1(s.Loudness), round1(s.Change)
		st.Sections = append(st.Sections, s)
	}

	// Silences: at least 0.15 s more than 35 dB below the loud parts of the track.
	floor := sorted[len(sorted)*9/10] - 35
	for i := 0; i < len(p.Loud); {
		if p.Loud[i] >= floor {
			i++
			continue
		}
		j := i
		for j < len(p.Loud) && p.Loud[j] < floor {
			j++
		}
		if a, b := p.Time(i), p.Time(j-1); b-a >= 0.15 {
			st.Events = append(st.Events, Event{Kind: "silence", Start: round3(a), End: round3(b)})
		}
		i = j
	}
	// Rises and falls: a beat at least 6 dB louder or quieter than the two beats before it.
	for i := 2; i < len(loudBeat); i++ {
		d := loudBeat[i] - (loudBeat[i-1]+loudBeat[i-2])/2
		if d >= 6 {
			st.Events = append(st.Events, Event{Kind: "rise", Start: round3(beats[i]), DB: round1(d)})
		} else if d <= -6 {
			st.Events = append(st.Events, Event{Kind: "fall", Start: round3(beats[i]), DB: round1(d)})
		}
	}
	// A fade or a build makes several rises or falls in a row: keep the biggest of each run.
	var merged []Event
	for _, e := range st.Events {
		if n := len(merged); n > 0 && e.Kind != "silence" && merged[n-1].Kind == e.Kind && e.Start-merged[n-1].Start <= 2.5*period {
			if math.Abs(e.DB) > math.Abs(merged[n-1].DB) {
				merged[n-1].DB = e.DB
			}
			continue
		}
		merged = append(merged, e)
	}
	st.Events = merged
	sort.SliceStable(st.Events, func(i, j int) bool { return st.Events[i].Start < st.Events[j].Start })

	// Accents: the strongest onset peaks, at least 100 ms apart, about two per second at most.
	type peak struct {
		i int
		v float64
	}
	var peaks []peak
	w := int(math.Round(0.05 * HopRate))
	for i := range p.Flux {
		if p.Flux[i] < 0.2 {
			continue
		}
		top := true
		for j := max(0, i-w); j <= min(len(p.Flux)-1, i+w); j++ {
			if p.Flux[j] > p.Flux[i] || (p.Flux[j] == p.Flux[i] && j < i) {
				top = false
				break
			}
		}
		if top {
			peaks = append(peaks, peak{i, p.Flux[i]})
		}
	}
	sort.Slice(peaks, func(a, b int) bool { return peaks[a].v > peaks[b].v })
	limit := max(8, int(1.5*p.Duration))
	var kept []peak
	for _, pk := range peaks {
		if len(kept) == limit {
			break
		}
		near := false
		for _, k := range kept {
			if math.Abs(p.Time(k.i)-p.Time(pk.i)) < 0.1 {
				near = true
				break
			}
		}
		if !near {
			kept = append(kept, pk)
		}
	}
	sort.Slice(kept, func(a, b int) bool { return kept[a].i < kept[b].i })
	names := [3]string{"low", "mid", "high"}
	for _, pk := range kept {
		t := p.Time(pk.i)
		beat := (t - beats[0]) / period
		half := math.Round(beat*2) / 2
		st.Accents = append(st.Accents, Accent{Time: round3(t), Strength: round2(pk.v), Band: names[p.FluxBand[pk.i]],
			Beat: round2(beat), OnGrid: math.Abs(beat-half)*period <= 0.04})
	}

	// Energy per bar, scaled so the quietest bar is 0 and the loudest 1.
	bars := r.Downbeats
	if len(bars) == 0 {
		return st
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for i, a := range bars {
		b := p.Duration
		if i+1 < len(bars) {
			b = bars[i+1]
		}
		e := p.mean(p.Loud, a, b)
		st.BarEnergy = append(st.BarEnergy, e)
		lo, hi = math.Min(lo, e), math.Max(hi, e)
	}
	for i := range st.BarEnergy {
		st.BarEnergy[i] = round2((st.BarEnergy[i] - lo) / math.Max(hi-lo, 1e-9))
	}
	return st
}

func round1(x float64) float64 { return math.Round(x*10) / 10 }
func round2(x float64) float64 { return math.Round(x*100) / 100 }
func round3(x float64) float64 { return math.Round(x*1000) / 1000 }
