package beats

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"

	"github.com/rock3r/cav/internal/sheet"
)

// View chooses what a picture of a track shows.
type View struct {
	Start, End float64 // seconds; End 0 means the end of the track
	Width      int     // pixels; 0 means 1600
	FPS        float64 // frame numbers on the time axis
	Grid       Result
	Structure  *Structure
	// Visual is how much the picture changes at each video frame (0..1), from a render.
	Visual []float64
	// Hits are the moments where the picture changes most.
	Hits []Mark
	// Cuts are planned shot changes (from a storyboard), drawn across every lane: green
	// on a downbeat, red off it.
	Cuts []Mark
}

// Mark is a moment on the time axis; OK marks are drawn green, the others red.
type Mark struct {
	Time float64
	OK   bool
}

var (
	pBG     = color.RGBA{18, 18, 22, 255}
	pText   = color.RGBA{60, 60, 68, 255}
	pBar    = color.RGBA{255, 255, 255, 110}
	pBeat   = color.RGBA{255, 255, 255, 50}
	pCut    = color.RGBA{255, 214, 10, 255}
	pLoud   = color.RGBA{120, 200, 255, 255}
	pRise   = color.RGBA{90, 220, 120, 255}
	pFall   = color.RGBA{255, 90, 90, 255}
	pSilent = color.RGBA{255, 90, 90, 60}
	pVisual = color.RGBA{240, 240, 240, 255}
	pBand   = [3]color.RGBA{{255, 110, 80, 255}, {255, 214, 10, 255}, {90, 200, 255, 255}}
)

// heat maps 0..1 to a dark-blue, purple, orange, yellow ramp (like magma).
func heat(v float64) color.RGBA {
	stops := [][3]float64{{0, 0, 4}, {40, 15, 100}, {150, 40, 120}, {240, 100, 60}, {252, 220, 120}, {252, 253, 191}}
	v = math.Max(0, math.Min(1, v)) * float64(len(stops)-1)
	i := min(int(v), len(stops)-2)
	f := v - float64(i)
	var c [3]uint8
	for k := 0; k < 3; k++ {
		c[k] = uint8(stops[i][k] + (stops[i+1][k]-stops[i][k])*f)
	}
	return color.RGBA{c[0], c[1], c[2], 255}
}

func blend(dst *image.RGBA, x, y int, c color.RGBA) {
	if !(image.Point{x, y}.In(dst.Rect)) {
		return
	}
	o := dst.PixOffset(x, y)
	a := float64(c.A) / 255
	for k, v := range [3]uint8{c.R, c.G, c.B} {
		dst.Pix[o+k] = uint8(float64(dst.Pix[o+k])*(1-a) + float64(v)*a)
	}
	dst.Pix[o+3] = 255
}

func vline(dst *image.RGBA, x, y0, y1 int, c color.RGBA) {
	for y := y0; y < y1; y++ {
		blend(dst, x, y, c)
	}
}

// Picture draws the track as one image, all lanes on a shared time axis:
//   - the spectrogram (low notes at the bottom), with bar lines, beat ticks, bar numbers and
//     section boundaries in yellow;
//   - loudness, with silences shaded red, rises marked green and falls red;
//   - onsets in three lanes (red low, yellow mid, blue high), with the strongest accents dotted;
//   - with View.Visual, how much the rendered picture changes per frame, and its hits
//     (green on the beat grid, red off it);
//   - a time axis in seconds and frames.
func Picture(p *Profile, v View, out string) (int, int, error) {
	if len(p.Spec) == 0 {
		return 0, 0, fmt.Errorf("the track is too short to draw")
	}
	if v.End <= 0 || v.End > p.Duration {
		v.End = p.Duration
	}
	if v.Start < 0 || v.Start >= v.End {
		return 0, 0, fmt.Errorf("the time range is empty")
	}
	if v.Width <= 0 {
		v.Width = 1600
	}
	const left, right, labelH = 44, 12, 18
	plotW := v.Width - left - right
	specH, loudH, accH, visH, axisH := 260, 70, 3*20, 0, 30
	if v.Visual != nil {
		visH = 70
	}
	gap := 6
	top := labelH + gap
	specY := top
	loudY := specY + specH + gap
	accY := loudY + loudH + gap
	visY := accY + accH + gap
	axisY := visY + visH
	if visH > 0 {
		axisY += gap
	}
	H := axisY + axisH
	dst := image.NewRGBA(image.Rect(0, 0, v.Width, H))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{pBG}, image.Point{}, draw.Src)
	span := v.End - v.Start
	xOf := func(t float64) int { return left + int(math.Round((t-v.Start)/span*float64(plotW))) }
	tOf := func(x int) float64 { return v.Start + float64(x-left)/float64(plotW)*span }
	label := func(x, y int, s string, bg color.Color) { sheet.Label(dst, x, y, s, 2, bg) }

	// Spectrogram: each pixel column takes the loudest value of its profile columns, so a
	// short hit stays visible when a long track is squeezed into the width.
	for x := left; x < left+plotW; x++ {
		c0, c1 := p.Column(tOf(x)), p.Column(tOf(x+1))
		for y := 0; y < specH; y++ {
			band := (specH - 1 - y) * Bands / specH
			m := float32(-200)
			for c := c0; c <= max(c0, c1-1); c++ {
				m = max(m, p.Spec[c][band])
			}
			dst.SetRGBA(x, specY+y, heat((float64(m)+80)/80))
		}
	}
	for _, f := range []float64{100, 1000, 10000} {
		b := math.Log(f/bandLo) / math.Log(bandHi/bandLo) * Bands
		y := specY + specH - 1 - int(b*float64(specH)/Bands)
		name := map[float64]string{100: "100", 1000: "1k", 10000: "10k"}[f]
		label(2, y-8, name, pText)
	}

	// Loudness from -60 to 0 dBFS, as a filled curve.
	dbY := func(db float64) int {
		return loudY + loudH - 1 - int(math.Max(0, math.Min(1, (db+60)/60))*float64(loudH-1))
	}
	for x := left; x < left+plotW; x++ {
		c0, c1 := p.Column(tOf(x)), p.Column(tOf(x+1))
		m := -200.0
		for c := c0; c <= max(c0, c1-1); c++ {
			m = math.Max(m, p.Loud[c])
		}
		y := dbY(m)
		vline(dst, x, y, loudY+loudH, color.RGBA{pLoud.R, pLoud.G, pLoud.B, 90})
		blend(dst, x, y, pLoud)
	}
	label(2, loudY, "db", pText)

	// Beat grid and sections across the spectrogram (and the lanes under it).
	bottom := accY + accH
	if visH > 0 {
		bottom = visY + visH
	}
	isBar := map[float64]bool{}
	for _, d := range v.Grid.Downbeats {
		isBar[d] = true
	}
	lastLabel := -100
	for i, t := range v.Grid.Beats {
		if t < v.Start || t > v.End {
			continue
		}
		x := xOf(t)
		if !isBar[t] {
			vline(dst, x, specY, specY+8, pBeat)
			vline(dst, x, accY, bottom, pBeat)
			continue
		}
		vline(dst, x, specY, specY+specH, pBar)
		vline(dst, x, loudY, bottom, pBar)
		bar := 0
		for k, d := range v.Grid.Downbeats {
			if d == t {
				bar = k + 1
			}
		}
		s := fmt.Sprintf("%d", bar)
		if x-lastLabel > len(s)*12+10 && x+2+len(s)*12 < v.Width {
			label(x+2, 0, s, pBG)
			lastLabel = x
		}
		_ = i
	}
	label(2, 0, "bar", pText)
	if st := v.Structure; st != nil {
		for i, s := range st.Sections {
			if i == 0 || s.Start < v.Start || s.Start > v.End {
				continue
			}
			x := xOf(s.Start)
			for d := 0; d < 3; d++ {
				vline(dst, x+d-1, specY, bottom, pCut)
			}
			label(x+4, specY+specH-18, fmt.Sprintf("s%d", i+1), color.RGBA{120, 90, 0, 255})
		}
		for _, e := range st.Events {
			if e.Start > v.End || math.Max(e.Start, e.End) < v.Start {
				continue
			}
			switch e.Kind {
			case "silence":
				for x := xOf(e.Start); x <= xOf(e.End); x++ {
					vline(dst, x, loudY, loudY+loudH, pSilent)
				}
			case "rise":
				x := xOf(e.Start)
				for d := 0; d < 7; d++ {
					for k := -d / 2; k <= d/2; k++ {
						blend(dst, x+k, loudY+d, pRise)
					}
				}
			case "fall":
				x := xOf(e.Start)
				for d := 0; d < 7; d++ {
					for k := -(6 - d) / 2; k <= (6-d)/2; k++ {
						blend(dst, x+k, loudY+d, pFall)
					}
				}
			}
		}
	}
	// Onsets per band, one lane each: the kick and bass pattern, the snare/stab/voice pattern
	// and the hats. The strongest accents get a white dot.
	const laneH = 20
	for b := 0; b < 3; b++ {
		y0 := accY + b*laneH
		for x := left; x < left+plotW; x++ {
			c0, c1 := p.Column(tOf(x)), p.Column(tOf(x+1))
			m := 0.0
			for c := c0; c <= max(c0, c1-1); c++ {
				m = math.Max(m, p.BandFlux[b][c])
			}
			// Square root: the biggest hit of the track would otherwise flatten every other onset.
			if h := int(math.Sqrt(m) * (laneH - 2)); h > 0 {
				vline(dst, x, y0+laneH-1-h, y0+laneH-1, pBand[b])
			}
		}
		label(2, y0+3, []string{"low", "mid", "hi"}[b], pText)
	}
	if st := v.Structure; st != nil {
		names := map[string]int{"low": 0, "mid": 1, "high": 2}
		for _, a := range st.Accents {
			if a.Time < v.Start || a.Time > v.End {
				continue
			}
			x, y := xOf(a.Time), accY+names[a.Band]*laneH
			for dx := -1; dx <= 1; dx++ {
				for dy := 0; dy <= 2; dy++ {
					blend(dst, x+dx, y+dy, pVisual)
				}
			}
		}
	}

	// How much the rendered picture changes, per video frame, with the found hits.
	if visH > 0 {
		peak := 1e-9
		for _, c := range v.Visual {
			peak = math.Max(peak, c)
		}
		for x := left; x < left+plotW; x++ {
			f0, f1 := int(tOf(x)*v.FPS), int(tOf(x+1)*v.FPS)
			m := 0.0
			for f := max(0, f0); f <= max(f0, f1-1) && f < len(v.Visual); f++ {
				m = math.Max(m, v.Visual[f])
			}
			h := int(math.Sqrt(m/peak) * float64(visH-1))
			vline(dst, x, visY+visH-1-h, visY+visH, color.RGBA{pVisual.R, pVisual.G, pVisual.B, 140})
		}
		for _, m := range v.Hits {
			if m.Time >= v.Start && m.Time <= v.End {
				c := pFall
				if m.OK {
					c = pRise
				}
				x := xOf(m.Time)
				for k := -2; k <= 2; k++ {
					for d := 0; d < 6-2*abs(k); d++ {
						blend(dst, x+k, visY+d, c)
					}
				}
			}
		}
		label(2, visY, "pic", pText)
	}

	for _, m := range v.Cuts {
		if m.Time < v.Start || m.Time > v.End {
			continue
		}
		c := pFall
		if m.OK {
			c = pRise
		}
		x := xOf(m.Time)
		for _, dx := range []int{0, 1} {
			for y := specY; y < accY+accH; y++ {
				blend(dst, x+dx, y, color.RGBA{c.R, c.G, c.B, 200})
			}
		}
		for k := -4; k <= 4; k++ {
			for d := 0; d < 9-2*abs(k); d++ {
				blend(dst, x+k, specY+d, c)
			}
		}
	}

	// Time axis: a tick and a label every 1, 2, 5, 10, 15 or 30 seconds, whichever fits.
	step := 1.0
	for _, s := range []float64{1, 2, 5, 10, 15, 30, 60} {
		step = s
		if float64(plotW)*s/span >= 160 {
			break
		}
	}
	for t := math.Ceil(v.Start/step) * step; t <= v.End+1e-9; t += step {
		x := xOf(t)
		vline(dst, x, axisY, axisY+5, pVisual)
		s := fmt.Sprintf("%gs", t)
		if v.FPS > 0 {
			s += fmt.Sprintf(" f%d", int(math.Round(t*v.FPS)))
		}
		if x+len(s)*12 < v.Width {
			label(x+2, axisY+6, s, pBG)
		}
	}

	f, err := os.Create(out)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	if err := png.Encode(f, dst); err != nil {
		return 0, 0, err
	}
	return v.Width, H, nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
