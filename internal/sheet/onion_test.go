package sheet

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// writeFrame draws a still grey block and a white dot at dotX on a black frame.
func writeFrame(t *testing.T, path string, dotX int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 100, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 100; x++ {
			c := color.RGBA{0, 0, 0, 255}
			if x >= 70 && x < 90 && y >= 5 && y < 15 {
				c = color.RGBA{128, 128, 128, 255}
			}
			if x >= dotX && x < dotX+6 && y >= 25 && y < 31 {
				c = color.RGBA{255, 255, 255, 255}
			}
			img.Set(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestOnionDrawsMotionAndReportsSteps(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	dots := []int{5, 20, 40, 41}
	for i, x := range dots {
		p := filepath.Join(dir, string(rune('a'+i))+".png")
		writeFrame(t, p, x)
		paths = append(paths, p)
	}
	out := filepath.Join(dir, "onion.png")
	w, h, steps, err := Onion(paths, []int{0, 10, 20, 30}, out)
	if err != nil {
		t.Fatal(err)
	}
	if w != 100 || h <= 40 {
		t.Fatalf("size %dx%d", w, h)
	}
	f, _ := os.Open(out)
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	// The newest dot is drawn solid in its own colour.
	if r, g, b, _ := img.At(43, 27).RGBA(); r>>8 != 255 || g>>8 != 255 || b>>8 != 255 {
		t.Fatalf("last dot is %d,%d,%d; want white", r>>8, g>>8, b>>8)
	}
	// The oldest dot is a faint blue ghost: visible, but not white.
	r, g, b, _ := img.At(7, 27).RGBA()
	if b>>8 <= r>>8 || r>>8 == 255 || b>>8 < 40 {
		t.Fatalf("first dot is %d,%d,%d; want a faint blue ghost", r>>8, g>>8, b>>8)
	}
	// The still block is dimmed, not hidden.
	if r, _, _, _ := img.At(80, 10).RGBA(); r>>8 < 50 || r>>8 > 100 {
		t.Fatalf("still block red is %d; want a dimmed grey", r>>8)
	}
	if len(steps) != 3 {
		t.Fatalf("got %d steps", len(steps))
	}
	if steps[0].Changed == 0 || steps[0].Box != [4]int{5, 25, 26, 31} {
		t.Fatalf("first step %+v", steps[0])
	}
	// A one-pixel nudge changes far less than a jump.
	if steps[2].Changed >= steps[1].Changed {
		t.Fatalf("small move %.4f not below big move %.4f", steps[2].Changed, steps[1].Changed)
	}
}

func TestOnionReportsHolds(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.png"), filepath.Join(dir, "b.png")
	writeFrame(t, a, 10)
	writeFrame(t, b, 10)
	_, _, steps, err := Onion([]string{a, b}, []int{0, 5}, filepath.Join(dir, "o.png"))
	if err != nil {
		t.Fatal(err)
	}
	if steps[0].Changed != 0 || steps[0].Box != ([4]int{}) {
		t.Fatalf("hold reported as %+v", steps[0])
	}
}
