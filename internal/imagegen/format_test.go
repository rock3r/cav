package imagegen

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"os"
	"testing"
)

// testdata/recraft_cutout.webp is a real Recraft --alpha answer, scaled down to 64x64
// (2026-10-13): WebP with alpha, which Recraft sends for a "PNG".
func TestNormalizedTurnsWebPIntoPNGWithAlpha(t *testing.T) {
	data, err := os.ReadFile("testdata/recraft_cutout.webp")
	if err != nil {
		t.Fatal(err)
	}
	im, err := normalized(&Image{Data: data, MIME: "image/png"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if http.DetectContentType(im.Data) != "image/png" || im.MIME != "image/png" || im.Ext() != ".png" {
		t.Fatalf("got %s, MIME %s", http.DetectContentType(im.Data), im.MIME)
	}
	img, _, err := image.Decode(bytes.NewReader(im.Data))
	if err != nil {
		t.Fatal(err)
	}
	transparent := false
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y && !transparent; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a == 0 {
				transparent = true
				break
			}
		}
	}
	if !transparent {
		t.Fatal("the PNG lost the WebP's transparency")
	}
}

// Gemini answers JPEG; the MIME type follows the bytes, so the default name is .jpg.
func TestNormalizedTrustsTheBytes(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.White)
	var b bytes.Buffer
	jpeg.Encode(&b, img, nil)
	im, _ := normalized(&Image{Data: b.Bytes(), MIME: "image/png"}, nil)
	if im.MIME != "image/jpeg" || im.Ext() != ".jpg" {
		t.Fatalf("got %s", im.MIME)
	}
	conv, err := ToPNG(im)
	if err != nil || http.DetectContentType(conv.Data) != "image/png" {
		t.Fatalf("ToPNG: %v", err)
	}
}
