// Package beats finds the tempo and beat times of a music track.
//
// Method (a small version of what librosa does):
//  1. onset strength: positive spectral flux of a log-magnitude spectrogram;
//  2. tempo: autocorrelation of the onset curve, weighted towards 120 BPM;
//  3. beats: dynamic programming that rewards onsets and a steady period (Ellis 2007);
//  4. downbeats: the 4/4 bar phase whose beats carry the most low-frequency energy.
package beats

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/cmplx"
	"os"
	"os/exec"
)

const (
	SampleRate = 22050
	hop        = 256
	win        = 1024
)

type Result struct {
	BPM        float64   `json:"bpm"`
	Duration   float64   `json:"duration"`
	Beats      []float64 `json:"beats"`      // seconds
	Downbeats  []float64 `json:"downbeats"`  // seconds, first beat of each 4/4 bar
	Confidence float64   `json:"confidence"` // 0..1, how clearly one tempo wins
	Steady     bool      `json:"steady"`     // beats were snapped to a least-squares grid
}

// fitGrid fits beat_i = offset + i*period. When the track keeps a steady tempo (small
// residual), the fitted grid is more useful for animation than the jittery detections.
func fitGrid(b []float64) ([]float64, float64, bool) {
	n := float64(len(b))
	if n < 8 {
		return nil, 0, false
	}
	var sx, sy, sxx, sxy float64
	for i, t := range b {
		x := float64(i)
		sx, sy, sxx, sxy = sx+x, sy+t, sxx+x*x, sxy+x*t
	}
	period := (n*sxy - sx*sy) / (n*sxx - sx*sx)
	offset := (sy - period*sx) / n
	var ss float64
	for i, t := range b {
		d := t - (offset + float64(i)*period)
		ss += d * d
	}
	if math.Sqrt(ss/n) > 0.03 {
		return nil, 0, false
	}
	for offset-period > -0.02 {
		offset -= period
	}
	for offset < -0.02 {
		offset += period
	}
	out := make([]float64, len(b))
	for i := range b {
		out[i] = offset + float64(i)*period
	}
	return out, period, true
}

// Decode reads any audio file through ffmpeg as mono float samples at SampleRate.
func Decode(path string) ([]float64, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	cmd := exec.Command("ffmpeg", "-v", "error", "-i", path, "-ac", "1", "-ar", fmt.Sprint(SampleRate), "-f", "s16le", "-")
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("ffmpeg is needed to read audio: %w", err)
		}
		return nil, fmt.Errorf("ffmpeg: %v: %s", err, stderr.String())
	}
	raw := out.Bytes()
	s := make([]float64, len(raw)/2)
	for i := range s {
		s[i] = float64(int16(binary.LittleEndian.Uint16(raw[2*i:]))) / 32768
	}
	return s, nil
}

// Onsets returns the onset strength per hop, plus a low-band (kick) curve.
func Onsets(samples []float64) (flux, low []float64) {
	window := make([]float64, win)
	for i := range window {
		window[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(win))
	}
	nFrames := 0
	if len(samples) >= win {
		nFrames = 1 + (len(samples)-win)/hop
	}
	bins := win/2 + 1
	lowBins := int(150 * win / SampleRate) // up to about 150 Hz
	prev := make([]float64, bins)
	prevLin := make([]float64, bins)
	buf := make([]complex128, win)
	flux = make([]float64, nFrames)
	low = make([]float64, nFrames)
	for f := 0; f < nFrames; f++ {
		off := f * hop
		for i := 0; i < win; i++ {
			buf[i] = complex(samples[off+i]*window[i], 0)
		}
		fft(buf)
		sum, lsum := 0.0, 0.0
		for k := 0; k < bins; k++ {
			mag := cmplx.Abs(buf[k])
			m := math.Log1p(100 * mag)
			if d := m - prev[k]; d > 0 {
				sum += d
			}
			// The low band uses linear magnitude: kicks carry far more energy than the
			// noise of hats and cymbals, and log compression would hide that.
			if k <= lowBins {
				if d := mag - prevLin[k]; d > 0 {
					lsum += d
				}
			}
			prev[k], prevLin[k] = m, mag
		}
		flux[f], low[f] = sum, lsum
	}
	return normalize(flux), normalize(low)
}

func normalize(x []float64) []float64 {
	// Subtract a moving average, clip at zero, scale to unit peak.
	out := make([]float64, len(x))
	const w = 16
	for i := range x {
		a, b := max(0, i-w), min(len(x), i+w+1)
		m := 0.0
		for _, v := range x[a:b] {
			m += v
		}
		m /= float64(b - a)
		out[i] = math.Max(0, x[i]-m)
	}
	peak := 0.0
	for _, v := range out {
		peak = math.Max(peak, v)
	}
	if peak > 0 {
		for i := range out {
			out[i] /= peak
		}
	}
	return out
}

func fft(a []complex128) {
	n := len(a)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		ang := -2 * math.Pi / float64(size)
		wl := complex(math.Cos(ang), math.Sin(ang))
		for i := 0; i < n; i += size {
			w := complex(1, 0)
			for k := 0; k < size/2; k++ {
				u, v := a[i+k], a[i+k+size/2]*w
				a[i+k], a[i+k+size/2] = u+v, u-v
				w *= wl
			}
		}
	}
}

const frameRate = float64(SampleRate) / hop // onset frames per second

// Tempo estimates BPM from the onset curve. hint > 0 restricts the search to ±8 %.
func Tempo(onset []float64, hint float64) (bpm, confidence float64) {
	lo, hi := 60.0, 200.0
	if hint > 0 {
		lo, hi = hint*0.92, hint*1.08
	}
	minLag := int(math.Floor(60 * frameRate / hi))
	maxLag := int(math.Ceil(60 * frameRate / lo))
	if maxLag >= len(onset) {
		maxLag = len(onset) - 1
	}
	best, bestLag, total := 0.0, 0.0, 0.0
	for lag := minLag; lag <= maxLag; lag++ {
		ac := 0.0
		for i := lag; i < len(onset); i++ {
			ac += onset[i] * onset[i-lag]
		}
		// Add the double period: real beats repeat at 2x the lag too.
		if 2*lag < len(onset) {
			for i := 2 * lag; i < len(onset); i++ {
				ac += 0.5 * onset[i] * onset[i-2*lag]
			}
		}
		b := 60 * frameRate / float64(lag)
		prior := 1.0
		if hint <= 0 {
			// Log-normal prior around 120 BPM, one octave wide.
			prior = math.Exp(-0.5 * math.Pow(math.Log2(b/120), 2))
		}
		score := ac * prior
		total += score
		if score > best {
			best, bestLag = score, float64(lag)
		}
	}
	if bestLag == 0 {
		return 0, 0
	}
	// Octave check: fast hi-hat patterns often win at twice the real tempo. Prefer the
	// half tempo when it explains the onsets almost as well and stays in a usual range.
	if hint <= 0 {
		l := int(bestLag)
		if 2*l < len(onset) && 60*frameRate/float64(2*l) >= 70 {
			ac := func(lag int) float64 {
				s := 0.0
				for i := lag; i < len(onset); i++ {
					s += onset[i] * onset[i-lag]
				}
				return s / float64(len(onset)-lag)
			}
			// Off-beat strength: onsets half-way between the slow beats.
			if ac(2*l) >= 0.9*ac(l) {
				bestLag *= 2
			}
		}
	}
	// Parabolic refinement of the peak lag.
	l := int(bestLag)
	sc := func(lag int) float64 {
		ac := 0.0
		for i := lag; i < len(onset); i++ {
			ac += onset[i] * onset[i-lag]
		}
		return ac
	}
	if l > minLag && l < maxLag {
		y0, y1, y2 := sc(l-1), sc(l), sc(l+1)
		if d := y0 - 2*y1 + y2; d != 0 {
			bestLag = float64(l) + 0.5*(y0-y2)/d
		}
	}
	n := float64(maxLag - minLag + 1)
	confidence = math.Min(1, (best/(total/n)-1)/4)
	return 60 * frameRate / bestLag, confidence
}

// Track places beats with dynamic programming, given the tempo.
func Track(onset []float64, bpm float64) []float64 {
	period := 60 * frameRate / bpm
	n := len(onset)
	score := make([]float64, n)
	back := make([]int, n)
	const tightness = 100.0
	for i := 0; i < n; i++ {
		best, arg := 0.0, -1
		lo, hi := i-int(2*period), i-int(period/2)
		for j := max(0, lo); j <= hi && j < i; j++ {
			d := math.Log(float64(i-j) / period)
			s := score[j] - tightness*d*d
			if arg < 0 || s > best {
				best, arg = s, j
			}
		}
		score[i] = onset[i]
		back[i] = -1
		if arg >= 0 && best > 0 {
			score[i] += best
			back[i] = arg
		}
	}
	// Start from the best score in the last period.
	end := 0
	for i := max(0, n-int(period)); i < n; i++ {
		if score[i] > score[end] {
			end = i
		}
	}
	var idx []int
	for i := end; i >= 0; i = back[i] {
		idx = append([]int{i}, idx...)
		if back[i] < 0 {
			break
		}
	}
	// Extend the grid to the start of the track at the same period.
	for len(idx) > 0 && float64(idx[0])-period > -0.5 {
		idx = append([]int{int(math.Round(float64(idx[0]) - period))}, idx...)
	}
	out := make([]float64, 0, len(idx))
	for _, i := range idx {
		if i >= 0 {
			out = append(out, float64(i)/frameRate+float64(win)/2/SampleRate)
		}
	}
	return out
}

// Downbeats picks the 4/4 phase with the most low-band onset energy.
func Downbeats(beats []float64, low []float64) []float64 {
	if len(beats) < 4 {
		return beats
	}
	var energy [4]float64
	for i, t := range beats {
		f := int((t - float64(win)/2/SampleRate) * frameRate)
		for d := -2; d <= 2; d++ {
			if f+d >= 0 && f+d < len(low) {
				energy[i%4] += low[f+d]
			}
		}
	}
	phase := 0
	for p := 1; p < 4; p++ {
		if energy[p] > energy[phase] {
			phase = p
		}
	}
	var out []float64
	for i := phase; i < len(beats); i += 4 {
		out = append(out, beats[i])
	}
	return out
}

// Analyze runs the whole pipeline on decoded samples.
func Analyze(samples []float64, hint float64) Result {
	flux, low := Onsets(samples)
	// Kicks and snares define the beat better than hi-hats, so weight the low band up.
	mix := make([]float64, len(flux))
	for i := range flux {
		mix[i] = 1.0*flux[i] + low[i]
	}
	mix = normalize(mix)
	bpm, conf := Tempo(mix, hint)
	r := Result{BPM: math.Round(bpm*100) / 100, Duration: float64(len(samples)) / SampleRate, Confidence: math.Round(conf*100) / 100}
	if bpm <= 0 {
		return r
	}
	r.Beats = Track(mix, bpm)
	if fitted, period, ok := fitGrid(r.Beats); ok {
		r.Beats = fitted
		r.BPM = math.Round(60/period*100) / 100
		r.Steady = true
	}
	for i := range r.Beats {
		r.Beats[i] = math.Round(r.Beats[i]*1000) / 1000
	}
	r.Downbeats = Downbeats(r.Beats, low)
	return r
}

// Grid makes a beat grid from a known tempo, without audio.
func Grid(bpm, offset, duration float64) Result {
	r := Result{BPM: bpm, Duration: duration, Confidence: 1}
	period := 60 / bpm
	for i := 0; ; i++ {
		t := offset + float64(i)*period
		if t > duration+1e-9 {
			break
		}
		t = math.Round(t*1000) / 1000
		r.Beats = append(r.Beats, t)
		if i%4 == 0 {
			r.Downbeats = append(r.Downbeats, t)
		}
	}
	return r
}
