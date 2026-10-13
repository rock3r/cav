package imagegen

import (
	"bytes"
	"image"
	_ "image/jpeg"
	"image/png"
	"net/http"
	"strings"

	_ "golang.org/x/image/webp"
)

// normalized sets the image's MIME type from its bytes, because services do not always
// send what they say (Gemini answers JPEG, Recraft WebP). WebP becomes PNG: Cavalry and
// most tools expect PNG for a transparent image, and --alpha promises one.
func normalized(im *Image, err error) (*Image, error) {
	if err != nil || im == nil || strings.Contains(im.MIME, "svg") {
		return im, err
	}
	switch sniffed := http.DetectContentType(im.Data); sniffed {
	case "image/webp":
		return ToPNG(im)
	case "image/png", "image/jpeg":
		im.MIME = sniffed
	}
	return im, nil
}

// ToPNG re-encodes a raster image as PNG, keeping its alpha. A PNG comes back unchanged.
func ToPNG(im *Image) (*Image, error) {
	if http.DetectContentType(im.Data) == "image/png" {
		im.MIME = "image/png"
		return im, nil
	}
	img, _, err := image.Decode(bytes.NewReader(im.Data))
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, err
	}
	out := *im
	out.Data, out.MIME = b.Bytes(), "image/png"
	return &out, nil
}
