package sheet

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"sort"
)

// Step describes the change from one onion frame to the next.
type Step struct {
	From, To int
	// Changed is the share of pixels (0..1) that differ between the two frames.
	Changed float64
	// Box is the area that changed, in image pixels (x0, y0, x1, y1); empty when nothing did.
	Box [4]int
}

// A pixel counts as changed when one of its channels moves by more than this (0..255).
const onionThreshold = 24

var (
	onionEarly = [3]float64{70, 150, 255} // the oldest frame is tinted towards blue
	onionLate  = [3]float64{255, 140, 40} // frames near the newest are tinted towards orange
)

// Onion blends rendered frames into one image so that motion reads in a single picture.
// What stays the same in most frames is drawn once and dimmed. What moves is drawn once per
// frame, in order: older frames are fainter and tinted from blue towards orange, and the
// newest frame is drawn solid in its own colours, outlined in white. A legend strip under the image names each
// frame in its tint. frames and paths have the same length. It returns the image size and
// the change between neighbouring frames.
func Onion(paths []string, frames []int, out string) (int, int, []Step, error) {
	if len(paths) == 0 || len(paths) != len(frames) {
		return 0, 0, nil, fmt.Errorf("no frames to blend")
	}
	bg := color.RGBA{40, 40, 44, 255}
	imgs := make([]*image.RGBA, len(paths))
	for i, p := range paths {
		img, err := decodeOpaque(p, bg)
		if err != nil {
			return 0, 0, nil, err
		}
		if i > 0 && img.Bounds() != imgs[0].Bounds() {
			return 0, 0, nil, fmt.Errorf("%s has a different size from the first frame", p)
		}
		imgs[i] = img
	}
	b := imgs[0].Bounds()
	w, h := b.Dx(), b.Dy()

	// The still picture is what each pixel shows when nothing moving covers it. Where any frame
	// shows the flat background colour there, that is it: an object that settles and then holds
	// for most frames is still a moving object. Elsewhere (a gradient plate, say) it is the
	// median frame, ranked by brightness so it keeps real colours.
	flat := flatColour(imgs[0], imgs[len(imgs)-1])
	still := image.NewRGBA(b)
	px := make([]int, len(imgs))
pixels:
	for o := 0; o < len(still.Pix); o += 4 {
		// Where the first and last frames agree, that is the resting picture, even if
		// something passes over it in most of the frames between.
		if first, last := imgs[0].Pix[o:], imgs[len(imgs)-1].Pix[o:]; !differs(first, last) {
			copy(still.Pix[o:o+4], last[:4])
			continue
		}
		for i := range imgs {
			if !differs(imgs[i].Pix[o:], flat[:]) {
				copy(still.Pix[o:o+4], imgs[i].Pix[o:o+4])
				continue pixels
			}
			px[i] = i
		}
		sort.Slice(px, func(x, y int) bool { return luma(imgs[px[x]].Pix[o:]) < luma(imgs[px[y]].Pix[o:]) })
		copy(still.Pix[o:o+4], imgs[px[len(px)/2]].Pix[o:o+4])
	}

	labelH := (glyphH+2)*2 + 2
	gap := 6
	dst := image.NewRGBA(image.Rect(0, 0, w, h+labelH+2*gap))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)
	// Dim the still picture so the moving parts stand out.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			s := still.PixOffset(x+b.Min.X, y+b.Min.Y)
			d := dst.PixOffset(x, y)
			for c := 0; c < 3; c++ {
				dst.Pix[d+c] = uint8(0.45*float64(still.Pix[s+c]) + 0.55*float64(bgChannel(bg, c)))
			}
		}
	}
	n := len(imgs)
	tints := make([][3]float64, n)
	for i, img := range imgs {
		t := 1.0
		if n > 1 {
			t = float64(i) / float64(n-1)
		}
		alpha := 0.3 + 0.7*t
		tint := 0.0
		if i < n-1 {
			tint = 0.45
		}
		for c := 0; c < 3; c++ {
			tints[i][c] = onionEarly[c] + (onionLate[c]-onionEarly[c])*t
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				s := img.PixOffset(x+b.Min.X, y+b.Min.Y)
				if !differs(img.Pix[s:], still.Pix[s:]) {
					continue
				}
				d := dst.PixOffset(x, y)
				for c := 0; c < 3; c++ {
					v := float64(img.Pix[s+c])*(1-tint) + tints[i][c]*tint
					dst.Pix[d+c] = uint8(float64(dst.Pix[d+c])*(1-alpha) + v*alpha)
				}
			}
		}
	}
	// Outline the last frame's moving parts in white, so they are never mistaken for an early
	// ghost when the object itself is blue.
	last := imgs[n-1]
	moving := func(x, y int) bool {
		if x < 0 || y < 0 || x >= w || y >= h {
			return false
		}
		o := last.PixOffset(x+b.Min.X, y+b.Min.Y)
		return differs(last.Pix[o:], still.Pix[o:])
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if moving(x, y) {
				continue
			}
		ring:
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					if moving(x+dx, y+dy) {
						copy(dst.Pix[dst.PixOffset(x, y):], []uint8{255, 255, 255, 255})
						break ring
					}
				}
			}
		}
	}
	// Legend: one chip per frame in its tint, oldest first. The newest frame is drawn untinted,
	// so its chip is white on the background.
	x := gap
	for i, f := range frames {
		chip := color.RGBA{uint8(tints[i][0]), uint8(tints[i][1]), uint8(tints[i][2]), 255}
		if i == n-1 {
			chip = color.RGBA{90, 90, 96, 255}
		}
		text := fmt.Sprintf("f%d", f)
		if x+len(text)*(glyphW+1)*2+4 > w {
			break
		}
		drawLabel(dst, x, h+gap, text, 2, chip)
		x += len(text)*(glyphW+1)*2 + 4 + gap
	}

	steps := make([]Step, 0, n-1)
	for i := 1; i < n; i++ {
		st := Step{From: frames[i-1], To: frames[i], Box: [4]int{w, h, 0, 0}}
		changed := 0
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				o := imgs[i].PixOffset(x+b.Min.X, y+b.Min.Y)
				if !differs(imgs[i].Pix[o:], imgs[i-1].Pix[o:]) {
					continue
				}
				changed++
				st.Box = [4]int{min(st.Box[0], x), min(st.Box[1], y), max(st.Box[2], x+1), max(st.Box[3], y+1)}
			}
		}
		if changed == 0 {
			st.Box = [4]int{}
		}
		st.Changed = float64(changed) / float64(w*h)
		steps = append(steps, st)
	}

	f, err := os.Create(out)
	if err != nil {
		return 0, 0, nil, err
	}
	defer f.Close()
	if err := png.Encode(f, dst); err != nil {
		return 0, 0, nil, err
	}
	return dst.Bounds().Dx(), dst.Bounds().Dy(), steps, nil
}

// decodeOpaque reads a PNG and flattens any transparency over bg.
func decodeOpaque(path string, bg color.RGBA) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	out := image.NewRGBA(img.Bounds())
	draw.Draw(out, out.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)
	draw.Draw(out, out.Bounds(), img, img.Bounds().Min, draw.Over)
	return out, nil
}

// flatColour returns the most common colour of two frames (in coarse bins, then averaged
// within the winning bin). In most motion graphics that is the flat background.
func flatColour(a, b *image.RGBA) [3]uint8 {
	var count [4096]int
	var sum [4096][3]int
	for _, img := range []*image.RGBA{a, b} {
		for o := 0; o < len(img.Pix); o += 4 {
			p := img.Pix[o:]
			k := int(p[0]>>4)<<8 | int(p[1]>>4)<<4 | int(p[2]>>4)
			count[k]++
			for c := 0; c < 3; c++ {
				sum[k][c] += int(p[c])
			}
		}
	}
	best := 0
	for k := range count {
		if count[k] > count[best] {
			best = k
		}
	}
	n := max(1, count[best])
	return [3]uint8{uint8(sum[best][0] / n), uint8(sum[best][1] / n), uint8(sum[best][2] / n)}
}

func luma(p []uint8) int { return 299*int(p[0]) + 587*int(p[1]) + 114*int(p[2]) }

func differs(a, b []uint8) bool {
	for c := 0; c < 3; c++ {
		d := int(a[c]) - int(b[c])
		if d > onionThreshold || d < -onionThreshold {
			return true
		}
	}
	return false
}

func bgChannel(c color.RGBA, i int) uint8 { return [3]uint8{c.R, c.G, c.B}[i] }
