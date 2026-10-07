package imagegen

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rock3r/cav/internal/services"
	"github.com/rock3r/cav/internal/sheet"
)

// Vectorize traces a raster image into SVG with the chosen service: recraft (API),
// vtracer (colour, local) or potrace (one colour, local, through ffmpeg for the bitmap).
func Vectorize(ctx context.Context, ch *services.Choice, in string) (*Image, error) {
	switch ch.Service {
	case "recraft":
		data, err := os.ReadFile(in)
		if err != nil {
			return nil, err
		}
		return RecraftEdit(ctx, ch.Key, "vectorize", data)
	case "vtracer":
		tmp, err := os.MkdirTemp("", "cav-vec")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(tmp)
		out := filepath.Join(tmp, "out.svg")
		if b, err := exec.CommandContext(ctx, "vtracer", "--input", in, "--output", out).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("vtracer: %v: %s", err, strings.TrimSpace(string(b)))
		}
		data, err := os.ReadFile(out)
		return &Image{Data: data, MIME: "image/svg+xml", Provider: "vtracer"}, err
	case "potrace":
		tmp, err := os.MkdirTemp("", "cav-vec")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(tmp)
		pbm := filepath.Join(tmp, "in.pbm")
		if b, err := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-y", "-i", in, "-vf", "format=monob", pbm).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("ffmpeg (to a bitmap for potrace): %v: %s", err, strings.TrimSpace(string(b)))
		}
		out := filepath.Join(tmp, "out.svg")
		if b, err := exec.CommandContext(ctx, "potrace", "-s", pbm, "-o", out).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("potrace: %v: %s", err, strings.TrimSpace(string(b)))
		}
		data, err := os.ReadFile(out)
		return &Image{Data: data, MIME: "image/svg+xml", Provider: "potrace"}, err
	}
	return nil, fmt.Errorf("%s does not trace images", ch.Service)
}

// Card is a greybox storyboard frame: plain shapes and the shot's words, nothing to
// license. Colours come from the palette when it has them.
type Card struct {
	Width, Height int
	Title         string   // e.g. "s3"
	Lines         []string // what happens, wrapped to fit
	Footer        string   // e.g. "beats 8-14 · 4.0-7.0 s"
	Palette       []string // background, foreground, accent (hex)
}

func hexColor(s string, def color.RGBA) color.RGBA {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return def
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return def
	}
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}
}

func mix(a, b color.RGBA, t float64) color.RGBA {
	l := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t) }
	return color.RGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), 255}
}

// Greybox draws the card as a PNG.
func Greybox(c Card) ([]byte, error) {
	if c.Width <= 0 || c.Height <= 0 {
		c.Width, c.Height = 1280, 720
	}
	bg := color.RGBA{0x2a, 0x2e, 0x36, 255}
	fg := color.RGBA{0xc3, 0xca, 0xd8, 255}
	acc := color.RGBA{0x8f, 0xa4, 0xff, 255}
	if len(c.Palette) > 0 {
		bg = hexColor(c.Palette[0], bg)
	}
	if len(c.Palette) > 2 {
		acc = hexColor(c.Palette[2], acc)
	}
	img := image.NewRGBA(image.Rect(0, 0, c.Width, c.Height))
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)
	guide := mix(bg, fg, 0.12)
	// Thirds and the 90% safe area, as on a camera monitor.
	for i := 1; i <= 2; i++ {
		x, y := c.Width*i/3, c.Height*i/3
		draw.Draw(img, image.Rect(x, 0, x+1, c.Height), &image.Uniform{guide}, image.Point{}, draw.Src)
		draw.Draw(img, image.Rect(0, y, c.Width, y+1), &image.Uniform{guide}, image.Point{}, draw.Src)
	}
	mx, my := c.Width/20, c.Height/20
	safe := mix(bg, fg, 0.25)
	for _, r := range []image.Rectangle{
		image.Rect(mx, my, c.Width-mx, my+2), image.Rect(mx, c.Height-my-2, c.Width-mx, c.Height-my),
		image.Rect(mx, my, mx+2, c.Height-my), image.Rect(c.Width-mx-2, my, c.Width-mx, c.Height-my),
	} {
		draw.Draw(img, r, &image.Uniform{safe}, image.Point{}, draw.Src)
	}
	s := max(2, c.Height/160)  // body text pixel size
	big := max(4, c.Height/60) // title pixel size
	charW := 6 * s             // 5 px glyph + 1 px gap
	maxChars := (c.Width - 4*mx) / charW
	x, y := mx*2, my*2
	draw.Draw(img, image.Rect(x, y, x+big*2, y+9*big), &image.Uniform{acc}, image.Point{}, draw.Src)
	sheet.Label(img, x+big*3, y, c.Title, big, color.Transparent)
	y += 12 * big
	for _, line := range c.Lines {
		for _, l := range wrap(line, maxChars) {
			sheet.Label(img, x, y, l, s, color.Transparent)
			y += 11 * s
		}
	}
	if c.Footer != "" {
		footer := c.Footer
		if len(footer) > maxChars {
			footer = footer[:maxChars]
		}
		sheet.Label(img, x, c.Height-my*2-10*s, footer, s, color.Transparent)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func wrap(s string, n int) []string {
	if n < 8 {
		n = 8
	}
	var out []string
	cur := ""
	for _, w := range strings.Fields(s) {
		if cur != "" && len(cur)+1+len(w) > n {
			out = append(out, cur)
			cur = ""
		}
		if cur != "" {
			cur += " "
		}
		cur += w
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
