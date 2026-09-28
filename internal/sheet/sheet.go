// Package sheet tiles rendered frames into one labelled contact sheet.
package sheet

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"strings"
)

type Tile struct {
	Path  string
	Label string
}

type Options struct {
	Cols    int
	Gap     int
	Scale   int // pixel size of the label font
	BG      color.Color
	LabelBG color.Color
}

// Build reads the tiles and writes one PNG. It returns the sheet size.
func Build(tiles []Tile, out string, o Options) (int, int, error) {
	if len(tiles) == 0 {
		return 0, 0, fmt.Errorf("no frames to tile")
	}
	if o.Cols <= 0 {
		o.Cols = 4
		if len(tiles) <= 3 {
			o.Cols = len(tiles)
		} else if len(tiles) > 16 {
			o.Cols = 6
		}
	}
	if o.Gap == 0 {
		o.Gap = 6
	}
	if o.Scale == 0 {
		o.Scale = 2
	}
	if o.BG == nil {
		o.BG = color.RGBA{40, 40, 44, 255}
	}
	if o.LabelBG == nil {
		o.LabelBG = color.RGBA{0, 0, 0, 200}
	}
	imgs := make([]image.Image, len(tiles))
	tw, th := 0, 0
	for i, t := range tiles {
		f, err := os.Open(t.Path)
		if err != nil {
			return 0, 0, err
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			return 0, 0, fmt.Errorf("%s: %w", t.Path, err)
		}
		imgs[i] = img
		b := img.Bounds()
		tw, th = max(tw, b.Dx()), max(th, b.Dy())
	}
	rows := (len(tiles) + o.Cols - 1) / o.Cols
	// Labels sit in a strip under each tile, so they never cover the picture.
	lh := (glyphH+2)*o.Scale + o.Scale
	W := o.Cols*tw + (o.Cols+1)*o.Gap
	H := rows*(th+lh) + (rows+1)*o.Gap
	dst := image.NewRGBA(image.Rect(0, 0, W, H))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{o.BG}, image.Point{}, draw.Src)
	for i, img := range imgs {
		x := o.Gap + (i%o.Cols)*(tw+o.Gap)
		y := o.Gap + (i/o.Cols)*(th+lh+o.Gap)
		r := image.Rect(x, y, x+img.Bounds().Dx(), y+img.Bounds().Dy())
		draw.Draw(dst, r, img, img.Bounds().Min, draw.Over)
		if tiles[i].Label != "" {
			drawLabel(dst, x, y+th, tiles[i].Label, o.Scale, o.LabelBG)
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
	return W, H, nil
}

func drawLabel(dst *image.RGBA, x, y int, text string, s int, bg color.Color) {
	text = strings.ToLower(text)
	w := len(text)*(glyphW+1)*s + s*2
	h := (glyphH+2)*s + s
	draw.Draw(dst, image.Rect(x, y, x+w, y+h), &image.Uniform{bg}, image.Point{}, draw.Over)
	cx := x + s
	for _, r := range text {
		g, ok := font[r]
		if !ok {
			g = font['?']
		}
		for row := 0; row < glyphH; row++ {
			for col := 0; col < glyphW; col++ {
				if g[row][col] != '#' {
					continue
				}
				px, py := cx+col*s, y+s+row*s
				draw.Draw(dst, image.Rect(px, py, px+s, py+s), image.White, image.Point{}, draw.Src)
			}
		}
		cx += (glyphW + 1) * s
	}
}
