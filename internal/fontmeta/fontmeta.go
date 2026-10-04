// Package fontmeta reads ordered variable-font axes from OpenType fvar/name tables.
package fontmeta

import (
	"encoding/binary"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

type Axis struct {
	Tag     string  `json:"tag"`
	Name    string  `json:"name"`
	Min     float64 `json:"min"`
	Default float64 `json:"default"`
	Max     float64 `json:"max"`
}
type Font struct {
	Family string `json:"family"`
	Axes   []Axis `json:"axes"`
}

var order = binary.BigEndian

func Read(path string) []Font {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	header := make([]byte, 12)
	if _, err = f.ReadAt(header, 0); err != nil {
		return nil
	}
	offsets := []uint32{0}
	if string(header[:4]) == "ttcf" {
		n := order.Uint32(header[8:])
		if n == 0 || n > 64 {
			return nil
		}
		b := make([]byte, n*4)
		if _, err = f.ReadAt(b, 12); err != nil {
			return nil
		}
		offsets = []uint32{}
		for i := uint32(0); i < n; i++ {
			offsets = append(offsets, order.Uint32(b[i*4:]))
		}
	}
	fonts := []Font{}
	for _, offset := range offsets {
		if font, ok := readFont(f, int64(offset)); ok {
			fonts = append(fonts, font)
		}
	}
	return fonts
}
func table(f io.ReaderAt, offset int64, tag string) []byte {
	head := make([]byte, 12)
	if _, err := f.ReadAt(head, offset); err != nil {
		return nil
	}
	n := int(order.Uint16(head[4:]))
	if n > 256 {
		return nil
	}
	records := make([]byte, n*16)
	if _, err := f.ReadAt(records, offset+12); err != nil {
		return nil
	}
	for i := 0; i < n; i++ {
		r := records[i*16:]
		if string(r[:4]) == tag {
			length := order.Uint32(r[12:])
			if length > 1<<20 {
				return nil
			}
			b := make([]byte, length)
			if _, err := f.ReadAt(b, int64(order.Uint32(r[8:]))); err != nil {
				return nil
			}
			return b
		}
	}
	return nil
}
func names(b []byte) map[uint16]string {
	out := map[uint16]string{}
	if len(b) < 6 {
		return out
	}
	n, base := int(order.Uint16(b[2:])), int(order.Uint16(b[4:]))
	if 6+n*12 > len(b) {
		return out
	}
	for i := 0; i < n; i++ {
		r := b[6+i*12:]
		platform, lang, id := order.Uint16(r), order.Uint16(r[4:]), order.Uint16(r[6:])
		length, start := int(order.Uint16(r[8:])), base+int(order.Uint16(r[10:]))
		if start < 0 || start+length > len(b) {
			continue
		}
		raw := b[start : start+length]
		s := ""
		if platform == 0 || platform == 3 {
			if len(raw)%2 != 0 {
				continue
			}
			chars := make([]uint16, len(raw)/2)
			for j := range chars {
				chars[j] = order.Uint16(raw[j*2:])
			}
			s = string(utf16.Decode(chars))
		} else if platform == 1 {
			s = string(raw)
		}
		if s != "" && (out[id] == "" || lang == 0x409) {
			out[id] = s
		}
	}
	return out
}
func readFont(f io.ReaderAt, offset int64) (Font, bool) {
	b := table(f, offset, "fvar")
	if len(b) < 16 || order.Uint16(b) != 1 {
		return Font{}, false
	}
	start, n, size := int(order.Uint16(b[4:])), int(order.Uint16(b[8:])), int(order.Uint16(b[10:]))
	if n < 1 || n > 64 || size < 20 || start < 16 || start+n*size > len(b) {
		return Font{}, false
	}
	ns := names(table(f, offset, "name"))
	family := ns[16]
	if family == "" {
		family = ns[1]
	}
	if family == "" {
		return Font{}, false
	}
	axes := make([]Axis, 0, n)
	fixed := func(x []byte) float64 { return float64(int32(order.Uint32(x))) / 65536 }
	for i := 0; i < n; i++ {
		r := b[start+i*size:]
		a := Axis{Tag: string(r[:4]), Name: ns[order.Uint16(r[18:])], Min: fixed(r[4:]), Default: fixed(r[8:]), Max: fixed(r[12:])}
		if a.Min > a.Default || a.Default > a.Max || a.Name == "" {
			return Font{}, false
		}
		axes = append(axes, a)
	}
	return Font{Family: family, Axes: axes}, true
}

// Discovery is bounded and does not infer tags from display names or font families.
func Discover(roots []string) map[string][]Axis {
	result := map[string][]Axis{}
	ambiguous := map[string]bool{}
	scanned := 0
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			scanned++
			if scanned > 10000 {
				return fs.SkipAll
			}
			if err != nil || !d.Type().IsRegular() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".ttf" && ext != ".otf" && ext != ".ttc" {
				return nil
			}
			for _, font := range Read(path) {
				old, ok := result[font.Family]
				if ok && !sameAxes(old, font.Axes) {
					ambiguous[font.Family] = true
				}
				if !ambiguous[font.Family] {
					result[font.Family] = font.Axes
				} else {
					delete(result, font.Family)
				}
			}
			return nil
		})
	}
	if scanned > 10000 {
		return map[string][]Axis{}
	}
	return result
}
func sameAxes(a, b []Axis) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
